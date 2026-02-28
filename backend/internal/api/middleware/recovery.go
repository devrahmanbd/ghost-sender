package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"

	"email-campaign-system/pkg/logger"
)

type RecoveryConfig struct {
	Enabled bool
}

type RecoveryMiddleware struct {
	log logger.Logger // FIX: was *logger.Logger (pointer to interface)
	cfg RecoveryConfig
}

func NewRecoveryMiddleware(log logger.Logger, cfg RecoveryConfig) *RecoveryMiddleware { // FIX: accept interface
	return &RecoveryMiddleware{log: log, cfg: cfg}
}

func (m *RecoveryMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		defer func() {
			if rec := recover(); rec != nil {
				reqID := GetRequestID(r.Context())

				m.log.Error("panic recovered",
					logger.String("request_id", reqID),
					logger.String("method", r.Method),
					logger.String("path", r.URL.Path),
					logger.String("query", r.URL.RawQuery),
					logger.String("remote_addr", r.RemoteAddr),
					logger.String("user_agent", r.UserAgent()),
					logger.Int64("duration_ms", time.Since(start).Milliseconds()),
					logger.Any("panic", rec),
					logger.String("stack", string(debug.Stack())),
				)

				writeInternalServerError(w, reqID)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func writeInternalServerError(w http.ResponseWriter, requestID string) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	if requestID != "" {
		h.Set("X-Request-ID", requestID)
	}
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      "Internal Server Error",
		"status":     http.StatusInternalServerError,
		"request_id": requestID,
	})
}
