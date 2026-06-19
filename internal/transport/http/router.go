package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"test-task/internal/metrics"
	"test-task/internal/service"
)

type RouterDeps struct {
	Handler         *Handler
	Tokens          *service.TokenManager
	Metrics         *metrics.Metrics
	Registry        *prometheus.Registry
	RateLimitPerMin int
}

func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(Metrics(d.Metrics))

	// Служебные эндпоинты без аутентификации.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Handle("/metrics", promhttp.HandlerFor(d.Registry, promhttp.HandlerOpts{}))

	r.Route("/api/v1", func(r chi.Router) {
		// Публичные маршруты.
		r.Post("/register", d.Handler.Register)
		r.Post("/login", d.Handler.Login)

		// Защищённые маршруты: аутентификация + rate limiting на пользователя.
		r.Group(func(r chi.Router) {
			r.Use(Auth(d.Tokens))
			r.Use(RateLimit(d.RateLimitPerMin))

			r.Post("/teams", d.Handler.CreateTeam)
			r.Get("/teams", d.Handler.ListTeams)
			r.Post("/teams/{id}/invite", d.Handler.InviteToTeam)

			r.Post("/tasks", d.Handler.CreateTask)
			r.Get("/tasks", d.Handler.ListTasks)
			r.Put("/tasks/{id}", d.Handler.UpdateTask)
			r.Get("/tasks/{id}/history", d.Handler.TaskHistory)

			r.Get("/analytics/team-stats", d.Handler.TeamStats)
			r.Get("/analytics/top-creators", d.Handler.TopCreators)
			r.Get("/analytics/integrity-issues", d.Handler.IntegrityIssues)
		})
	})

	return r
}
