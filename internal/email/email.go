// Package email — мок внешнего email-сервиса, обёрнутый в circuit breaker.
// Используется для отправки приглашений в команду.
package email

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"

	"github.com/sony/gobreaker"

	"test-task/internal/config"
)

var ErrServiceUnavailable = errors.New("email service unavailable")

// Sender — интерфейс отправки писем (для подмены в тестах).
type Sender interface {
	SendInvite(ctx context.Context, to, teamName string) error
}

// MockClient имитирует внешний сервис: может «падать» с заданной вероятностью.
type MockClient struct {
	failRate float64
	log      *slog.Logger
}

func NewMockClient(failRate float64, log *slog.Logger) *MockClient {
	return &MockClient{failRate: failRate, log: log}
}

func (m *MockClient) send(to, teamName string) error {
	if rand.Float64() < m.failRate {
		return ErrServiceUnavailable
	}
	m.log.Info("invite email sent", "to", to, "team", teamName)
	return nil
}

// Service оборачивает Sender в circuit breaker (sony/gobreaker).
type Service struct {
	client *MockClient
	cb     *gobreaker.CircuitBreaker
}

func NewService(cfg config.EmailConfig, log *slog.Logger) *Service {
	threshold := uint32(cfg.FailureThreshold)
	if threshold == 0 {
		threshold = 3
	}
	settings := gobreaker.Settings{
		Name:        "email-service",
		Timeout:     cfg.OpenTimeout, // как долго breaker остаётся open
		MaxRequests: 1,               // пробных запросов в half-open
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= threshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed", "name", name, "from", from.String(), "to", to.String())
		},
	}
	return &Service{
		client: NewMockClient(cfg.FailRate, log),
		cb:     gobreaker.NewCircuitBreaker(settings),
	}
}

// SendInvite отправляет приглашение через breaker. Если цепь разомкнута,
// сразу возвращает ErrServiceUnavailable, не дёргая внешний сервис.
func (s *Service) SendInvite(ctx context.Context, to, teamName string) error {
	_, err := s.cb.Execute(func() (any, error) {
		return nil, s.client.send(to, teamName)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return ErrServiceUnavailable
		}
		return err
	}
	return nil
}
