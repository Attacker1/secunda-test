package email

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"test-task/internal/config"
)

func newSvc(failRate float64) *Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(config.EmailConfig{
		FailureThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
		FailRate:         failRate,
	}, log)
}

func TestService_SendSuccess(t *testing.T) {
	svc := newSvc(0.0) // мок никогда не падает
	if err := svc.SendInvite(context.Background(), "a@b.com", "team"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestService_CircuitOpensAfterFailures(t *testing.T) {
	svc := newSvc(1.0) // мок всегда падает
	ctx := context.Background()

	// Первые вызовы доходят до сервиса и падают, накапливая ошибки.
	_ = svc.SendInvite(ctx, "a@b.com", "team")
	_ = svc.SendInvite(ctx, "a@b.com", "team")

	// Порог достигнут — цепь разомкнута, дальше fail-fast.
	if err := svc.SendInvite(ctx, "a@b.com", "team"); err != ErrServiceUnavailable {
		t.Fatalf("expected ErrServiceUnavailable when circuit open, got %v", err)
	}
}

func TestService_CircuitRecovers(t *testing.T) {
	svc := newSvc(1.0)
	ctx := context.Background()
	_ = svc.SendInvite(ctx, "a@b.com", "team")
	_ = svc.SendInvite(ctx, "a@b.com", "team")

	// Чиним мок и ждём перехода в half-open.
	svc.client.failRate = 0.0
	time.Sleep(80 * time.Millisecond)

	if err := svc.SendInvite(ctx, "a@b.com", "team"); err != nil {
		t.Fatalf("expected recovery after timeout, got %v", err)
	}
}
