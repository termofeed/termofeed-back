package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/config"
	"github.com/termofeed/termofeed-back/internal/ratelimit"
	"github.com/termofeed/termofeed-back/internal/room"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func LoggingAndMetricsMiddleware(logger *slog.Logger, metrics *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			path := r.URL.Path

			// Log
			logger.Info("http request",
				"method", r.Method,
				"path", path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)

			// Record metrics
			if metrics != nil {
				metrics.HttpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(wrapped.statusCode)).Inc()
				metrics.HttpRequestDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())
			}
		})
	}
}

func CORSMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && cfg.IsOriginAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Host-Token, X-Participant-Token")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(limiter *ratelimit.IPRateLimiter, trustedProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip health and metrics endpoints from rate limiting
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			ip := GetClientIP(r, trustedProxy)

			// General 120 req/min
			allowed, retryAfter := limiter.AllowRequest(ip)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(gen.Problem{
					Code:              "RATE_LIMIT_EXCEEDED",
					Message:           "Limite de requisições excedido.",
					RetryAfterSeconds: &retryAfter,
				})
				return
			}

			// Room creation 10 rooms/hour
			if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/rooms" || r.URL.Path == "/api/v1/rooms/") {
				cAllowed, cRetryAfter := limiter.AllowCreateRoom(ip)
				if !cAllowed {
					w.Header().Set("Retry-After", strconv.Itoa(cRetryAfter))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_ = json.NewEncoder(w).Encode(gen.Problem{
						Code:              "RATE_LIMIT_EXCEEDED",
						Message:           "Limite de criação de salas excedido.",
						RetryAfterSeconds: &cRetryAfter,
					})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetClientIP(r *http.Request, trustedProxy bool) string {
	if trustedProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				clientIP := strings.TrimSpace(parts[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}
		if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			return strings.TrimSpace(xrip)
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func RespondProblem(w http.ResponseWriter, status int, code, message string, retryAfter *int, details []room.FieldIssue) {
	w.Header().Set("Content-Type", "application/json")
	if retryAfter != nil {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", *retryAfter))
	}
	w.WriteHeader(status)

	prob := gen.Problem{
		Code:              code,
		Message:           message,
		RetryAfterSeconds: retryAfter,
	}

	if len(details) > 0 {
		var d []struct {
			Field *string `json:"field,omitempty"`
			Issue *string `json:"issue,omitempty"`
		}
		for _, item := range details {
			f := item.Field
			i := item.Issue
			d = append(d, struct {
				Field *string `json:"field,omitempty"`
				Issue *string `json:"issue,omitempty"`
			}{Field: &f, Issue: &i})
		}
		prob.Details = &d
	}

	_ = json.NewEncoder(w).Encode(prob)
}

func RespondError(w http.ResponseWriter, err error) {
	if rErr, ok := err.(*room.Error); ok {
		RespondProblem(w, rErr.HTTPStatus, rErr.Code, rErr.Message, rErr.RetryAfterSeconds, rErr.Details)
		return
	}
	RespondProblem(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Erro interno do servidor.", nil, nil)
}
