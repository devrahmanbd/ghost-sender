package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"email-campaign-system/pkg/logger"
)

type authContextKey string

const AuthTokenKey authContextKey = "auth_token"

type AuthConfig struct {
	Enabled       bool
	HeaderName    string
	Token         string
	AllowedTokens []string
	BypassPaths   []string
	BypassMethods []string
	RequireHTTPS  bool
	Realm         string
}

type AuthMiddleware struct {
	log logger.Logger // Changed from *logger.Logger
	cfg AuthConfig
}

func NewAuthMiddleware(log logger.Logger, cfg AuthConfig) *AuthMiddleware { // Changed from *logger.Logger
	if cfg.HeaderName == "" {
		cfg.HeaderName = "Authorization"
	}
	if cfg.Realm == "" {
		cfg.Realm = "api"
	}
	return &AuthMiddleware{log: log, cfg: cfg}
}

func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		if m.isBypassed(r) {
			next.ServeHTTP(w, r)
			return
		}

		if m.cfg.RequireHTTPS && r.TLS == nil {
			m.writeUnauthorized(w, r, "invalid_request", "TLS required")
			return
		}

		token, ok := m.extractToken(r)
		if !ok {
			m.writeUnauthorized(w, r, "invalid_token", "Missing or invalid token")
			return
		}

		if !m.isTokenAllowed(token) {
			m.log.Warn("unauthorized request",
				logger.String("request_id", GetRequestID(r.Context())),
				logger.String("remote_addr", r.RemoteAddr),
				logger.String("method", r.Method),
				logger.String("path", r.URL.Path),
			)
			m.writeUnauthorized(w, r, "invalid_token", "Invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), AuthTokenKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetAuthToken(ctx context.Context) string {
	if v, ok := ctx.Value(AuthTokenKey).(string); ok {
		return v
	}
	return ""
}

func (m *AuthMiddleware) isBypassed(r *http.Request) bool {
	method := r.Method
	for _, bm := range m.cfg.BypassMethods {
		if bm == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(bm), method) {
			return true
		}
	}

	path := r.URL.Path
	for _, p := range m.cfg.BypassPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == path {
			return true
		}
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

func (m *AuthMiddleware) extractToken(r *http.Request) (string, bool) {
	h := strings.TrimSpace(r.Header.Get(m.cfg.HeaderName))
	if h == "" {
		return "", false
	}

	if strings.EqualFold(m.cfg.HeaderName, "Authorization") {
		if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return "", false
		}
		token := strings.TrimSpace(h[len("Bearer "):])
		return normalizeToken(token)
	}

	return normalizeToken(h)
}

func normalizeToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	if !utf8.ValidString(token) {
		return "", false
	}
	if len(token) < 8 || len(token) > 512 {
		return "", false
	}
	return token, true
}

func (m *AuthMiddleware) isTokenAllowed(token string) bool {
	if m.cfg.Token != "" {
		if secureEqual(token, m.cfg.Token) {
			return true
		}
	}
	for _, t := range m.cfg.AllowedTokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if secureEqual(token, t) {
			return true
		}
	}
	return false
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (m *AuthMiddleware) writeUnauthorized(w http.ResponseWriter, r *http.Request, errCode string, desc string) {
	if errCode == "" {
		errCode = "invalid_token"
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("WWW-Authenticate", `Bearer realm="`+m.cfg.Realm+`", error="`+errCode+`", error_description="`+escapeQuotes(desc)+`"`)
	if rid := GetRequestID(r.Context()); rid != "" {
		h.Set("X-Request-ID", rid)
	}

	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      "Unauthorized",
		"status":     http.StatusUnauthorized,
		"code":       errCode,
		"message":    desc,
		"request_id": GetRequestID(r.Context()),
	})
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `'`)
}
