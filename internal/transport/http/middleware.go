package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"

	"test-task/internal/domain"
	"test-task/internal/metrics"
	"test-task/internal/service"
)

type ctxKey int

const userIDKey ctxKey = iota

func userIDFromCtx(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// Auth проверяет Bearer-токен и кладёт userID в контекст.
func Auth(tokens *service.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSON(w, http.StatusUnauthorized, errorBody{Error: "missing or malformed Authorization header"})
				return
			}
			userID, err := tokens.Parse(parts[1])
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, errorBody{Error: "invalid or expired token"})
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusRecorder фиксирует статус ответа для метрик.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			path := routePattern(r)
			m.Duration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
			m.RequestsTotal.WithLabelValues(r.Method, path, statusClass(rec.status)).Inc()
			if rec.status >= http.StatusInternalServerError {
				m.ErrorsTotal.WithLabelValues(r.Method, path).Inc()
			}
		})
	}
}

// routePattern возвращает шаблон маршрута (без конкретных id), чтобы не плодить
// кардинальность меток Prometheus.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// userLimiters хранит token-bucket лимитеры на пользователя.
type userLimiters struct {
	mu       sync.Mutex
	limiters map[int64]*rate.Limiter
	rps      rate.Limit
	burst    int
}

func newUserLimiters(perMinute int) *userLimiters {
	if perMinute <= 0 {
		perMinute = 100
	}
	return &userLimiters{
		limiters: make(map[int64]*rate.Limiter),
		rps:      rate.Limit(float64(perMinute) / 60.0),
		burst:    perMinute,
	}
}

func (u *userLimiters) get(userID int64) *rate.Limiter {
	u.mu.Lock()
	defer u.mu.Unlock()
	l, ok := u.limiters[userID]
	if !ok {
		l = rate.NewLimiter(u.rps, u.burst)
		u.limiters[userID] = l
	}
	return l
}

// RateLimit ограничивает число запросов на пользователя (по умолчанию 100/мин).
// Должен стоять после Auth, чтобы знать userID.
func RateLimit(perMinute int) func(http.Handler) http.Handler {
	limiters := newUserLimiters(perMinute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := userIDFromCtx(r.Context())
			if !ok {
				writeError(w, nil, domain.ErrUnauthorized)
				return
			}
			if !limiters.get(userID).Allow() {
				w.Header().Set("Retry-After", "60")
				writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
