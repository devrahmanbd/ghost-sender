package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"email-campaign-system/pkg/logger"
)

type RateLimitMiddleware struct {
	limiter  *rate.Limiter
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	logger   logger.Logger // Changed from *logger.Logger
	enabled  bool
	perIP    bool
	rps      float64
	burst    int
}

type RateLimitConfig struct {
	Enabled bool
	RPS     float64
	Burst   int
	PerIP   bool
}

func NewRateLimitMiddleware(log logger.Logger, config RateLimitConfig) *RateLimitMiddleware { // Changed from *logger.Logger
	return &RateLimitMiddleware{
		limiter:  rate.NewLimiter(rate.Limit(config.RPS), config.Burst),
		limiters: make(map[string]*rate.Limiter),
		logger:   log,
		enabled:  config.Enabled,
		perIP:    config.PerIP,
		rps:      config.RPS,
		burst:    config.Burst,
	}
}

func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}

		var limiter *rate.Limiter

		if m.perIP {
			ip := m.getClientIP(r)
			limiter = m.getLimiterForIP(ip)
		} else {
			limiter = m.limiter
		}

		if !limiter.Allow() {
			m.logger.Warn("Rate limit exceeded",
				logger.String("ip", m.getClientIP(r)),
				logger.String("path", r.URL.Path),
				logger.String("method", r.Method),
			)

			m.respondRateLimitError(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitMiddleware) getLimiterForIP(ip string) *rate.Limiter {
	m.mu.Lock()
	defer m.mu.Unlock()

	limiter, exists := m.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(m.rps), m.burst)
		m.limiters[ip] = limiter
	}

	return limiter
}

func (m *RateLimitMiddleware) getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return ip
	}

	ip = r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}

	return r.RemoteAddr
}

func (m *RateLimitMiddleware) respondRateLimitError(w http.ResponseWriter, r *http.Request) {
	retryAfter := m.calculateRetryAfter()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", retryAfter)
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.2f", m.rps))
	w.Header().Set("X-RateLimit-Burst", strconv.Itoa(m.burst))
	w.WriteHeader(http.StatusTooManyRequests)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":       "Rate limit exceeded",
		"status":      http.StatusTooManyRequests,
		"retry_after": retryAfter,
		"message":     "Too many requests. Please try again later.",
	})
}

func (m *RateLimitMiddleware) calculateRetryAfter() string {
	waitTime := time.Duration(1.0/m.rps) * time.Second
	return strconv.Itoa(int(waitTime.Seconds()))
}

func (m *RateLimitMiddleware) CleanupStaleEntries() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		for ip, limiter := range m.limiters {
			if limiter.Tokens() == float64(m.burst) {
				delete(m.limiters, ip)
			}
		}
		m.mu.Unlock()
	}
}

func (m *RateLimitMiddleware) Enable() {
	m.enabled = true
	m.logger.Info("Rate limiting enabled")
}

func (m *RateLimitMiddleware) Disable() {
	m.enabled = false
	m.logger.Info("Rate limiting disabled")
}

func (m *RateLimitMiddleware) UpdateLimits(rps float64, burst int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rps = rps
	m.burst = burst
	m.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	m.limiters = make(map[string]*rate.Limiter)

	m.logger.Info("Rate limits updated",
		logger.Float64("rps", rps),
		logger.Int("burst", burst),
	)
}

func (m *RateLimitMiddleware) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"enabled":    m.enabled,
		"per_ip":     m.perIP,
		"rps":        m.rps,
		"burst":      m.burst,
		"active_ips": len(m.limiters),
	}
}
