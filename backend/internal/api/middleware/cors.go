package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAgeSeconds    int
}

type CORSMiddleware struct {
	cfg CORSConfig
}

func NewCORSMiddleware(cfg CORSConfig) *CORSMiddleware {
	if len(cfg.AllowedMethods) == 0 {
		cfg.AllowedMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		}
	}
	if len(cfg.AllowedHeaders) == 0 {
		cfg.AllowedHeaders = []string{
			"Accept",
			"Accept-Language",
			"Authorization",
			"Content-Language",
			"Content-Type",
			"Origin",
			"X-Requested-With",
		}
	}
	if cfg.MaxAgeSeconds == 0 {
		cfg.MaxAgeSeconds = 600
	}
	return &CORSMiddleware{cfg: cfg}
}

func (m *CORSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		allowedOrigin, ok := m.resolveAllowedOrigin(origin)
		if !ok {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", allowedOrigin)
		h.Add("Vary", "Origin")

		if m.cfg.AllowCredentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if len(m.cfg.ExposedHeaders) > 0 {
			h.Set("Access-Control-Expose-Headers", strings.Join(m.cfg.ExposedHeaders, ", "))
		}

		if r.Method == http.MethodOptions {
			reqMethod := r.Header.Get("Access-Control-Request-Method")
			if reqMethod == "" {
				reqMethod = http.MethodGet
			}

			reqHeaders := r.Header.Get("Access-Control-Request-Headers")

			h.Set("Access-Control-Allow-Methods", strings.Join(m.cfg.AllowedMethods, ", "))
			h.Set("Access-Control-Allow-Headers", m.allowedHeadersValue(reqHeaders))
			h.Set("Access-Control-Max-Age", strconv.Itoa(m.cfg.MaxAgeSeconds))

			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *CORSMiddleware) resolveAllowedOrigin(origin string) (string, bool) {
	if len(m.cfg.AllowedOrigins) == 0 {
		return "", false
	}

	for _, o := range m.cfg.AllowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}

		if o == "*" {
			if m.cfg.AllowCredentials {
				return origin, true
			}
			return "*", true
		}

		if strings.EqualFold(o, origin) {
			return origin, true
		}
	}

	return "", false
}

func (m *CORSMiddleware) allowedHeadersValue(requested string) string {
	allowedSet := make(map[string]struct{}, len(m.cfg.AllowedHeaders))
	for _, h := range m.cfg.AllowedHeaders {
		h = http.CanonicalHeaderKey(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		allowedSet[h] = struct{}{}
	}

	if requested == "" {
		out := make([]string, 0, len(allowedSet))
		for h := range allowedSet {
			out = append(out, h)
		}
		return strings.Join(out, ", ")
	}

	reqParts := strings.Split(requested, ",")
	out := make([]string, 0, len(reqParts))
	for _, p := range reqParts {
		h := http.CanonicalHeaderKey(strings.TrimSpace(p))
		if h == "" {
			continue
		}
		if _, ok := allowedSet[h]; ok {
			out = append(out, h)
		}
	}

	if len(out) == 0 {
		out = make([]string, 0, len(allowedSet))
		for h := range allowedSet {
			out = append(out, h)
		}
	}

	return strings.Join(out, ", ")
}
