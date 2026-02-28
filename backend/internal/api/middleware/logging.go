package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"email-campaign-system/pkg/logger"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

type LoggingConfig struct {
	Enabled            bool
	LogRequestHeaders  bool
	LogResponseHeaders bool
	LogBodyBytes       bool
	RequestIDHeader    string
}

type LoggingMiddleware struct {
	log logger.Logger // Changed from *logger.Logger
	cfg LoggingConfig
}

func NewLoggingMiddleware(log logger.Logger, cfg LoggingConfig) *LoggingMiddleware { // Changed from *logger.Logger
	if cfg.RequestIDHeader == "" {
		cfg.RequestIDHeader = "X-Request-ID"
	}
	return &LoggingMiddleware{log: log, cfg: cfg}
}

func (m *LoggingMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		reqID := m.getOrCreateRequestID(r)
		r = r.WithContext(context.WithValue(r.Context(), RequestIDKey, reqID))
		w.Header().Set(m.cfg.RequestIDHeader, reqID)

		start := time.Now()
		lrw := newLoggingResponseWriter(w)

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		ip := clientIP(r)

		// Build logger fields using proper logger field functions
		fields := []logger.Field{
			logger.String("request_id", reqID),
			logger.String("remote_ip", ip),
			logger.String("method", r.Method),
			logger.String("path", r.URL.Path),
			logger.String("query", r.URL.RawQuery),
			logger.Int("status", lrw.status),
			logger.Int64("bytes", lrw.bytes),
			logger.Int64("duration_ms", duration.Milliseconds()),
			logger.String("user_agent", r.UserAgent()),
			logger.String("referer", r.Referer()),
		}

		if m.cfg.LogRequestHeaders {
			fields = append(fields, logger.Any("req_headers", sanitizeHeaders(r.Header)))
		}
		if m.cfg.LogResponseHeaders {
			fields = append(fields, logger.Any("resp_headers", sanitizeHeaders(lrw.Header())))
		}

		if lrw.status >= 500 {
			m.log.Error("HTTP request", fields...)
			return
		}
		if lrw.status >= 400 {
			m.log.Warn("HTTP request", fields...)
			return
		}
		m.log.Info("HTTP request", fields...)
	})
}

func (m *LoggingMiddleware) getOrCreateRequestID(r *http.Request) string {
	h := r.Header.Get(m.cfg.RequestIDHeader)
	if isValidRequestID(h) {
		return h
	}
	return newRequestID()
}

func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (w *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *loggingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		w.bytes += n
		return n, err
	}
	return 0, http.ErrNotSupported
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func sanitizeHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		ck := http.CanonicalHeaderKey(k)
		if ck == "Authorization" || ck == "Cookie" || ck == "Set-Cookie" || ck == "X-Api-Key" {
			out[ck] = []string{"[redacted]"}
			continue
		}
		out[ck] = v
	}
	return out
}

func isValidRequestID(s string) bool {
	if len(s) < 8 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
