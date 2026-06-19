package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"test-task/internal/cache"
	"test-task/internal/config"
	"test-task/internal/database"
	"test-task/internal/email"
	"test-task/internal/metrics"
	"test-task/internal/repository"
	"test-task/internal/service"
	httpapi "test-task/internal/transport/http"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// MySQL + миграции
	db, err := database.New(cfg.MySQL)
	if err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	log.Info("mysql connected and migrated")

	// Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rdb.Close()
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancelPing()
		return fmt.Errorf("connect redis: %w", err)
	}
	cancelPing()
	log.Info("redis connected")

	// Слои
	userRepo := repository.NewUserRepo(db)
	teamRepo := repository.NewTeamRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	analyticsRepo := repository.NewAnalyticsRepo(db)

	taskCache := cache.NewRedis(rdb, cfg.Redis.TaskListTTL)
	mailer := email.NewService(cfg.Email, log)
	tokens := service.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.TokenTTL)

	authSvc := service.NewAuthService(userRepo, tokens)
	teamSvc := service.NewTeamService(teamRepo, userRepo, mailer)
	taskSvc := service.NewTaskService(taskRepo, teamRepo, taskCache, log)

	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	handler := httpapi.NewHandler(authSvc, teamSvc, taskSvc, analyticsRepo, log)
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Handler:         handler,
		Tokens:          tokens,
		Metrics:         m,
		Registry:        registry,
		RateLimitPerMin: cfg.RateLimit.RequestsPerMinute,
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	// Запуск + graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	log.Info("server stopped gracefully")
	return nil
}
