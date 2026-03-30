package middleware

import (
        "context"
        "crypto/hmac"
        "crypto/sha256"
        "crypto/subtle"
        "encoding/base64"
        "encoding/json"
        "net/http"
        "strings"
        "time"
        "unicode/utf8"

        "email-campaign-system/pkg/logger"
)

type authContextKey string

const AuthTokenKey authContextKey = "auth_token"
const AuthUserKey authContextKey = "auth_user"

type AuthConfig struct {
        Enabled       bool
        HeaderName    string
        Token         string
        AllowedTokens []string
        BypassPaths   []string
        BypassMethods []string
        RequireHTTPS  bool
        Realm         string
        JWTSecret     string
}

type AuthMiddleware struct {
        log logger.Logger
        cfg AuthConfig
}

func NewAuthMiddleware(log logger.Logger, cfg AuthConfig) *AuthMiddleware {
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

                if m.cfg.JWTSecret != "" {
                        username, err := m.validateJWT(token)
                        if err == nil {
                                ctx := context.WithValue(r.Context(), AuthTokenKey, token)
                                ctx = context.WithValue(ctx, AuthUserKey, username)
                                next.ServeHTTP(w, r.WithContext(ctx))
                                return
                        }
                }

                if m.isTokenAllowed(token) {
                        ctx := context.WithValue(r.Context(), AuthTokenKey, token)
                        next.ServeHTTP(w, r.WithContext(ctx))
                        return
                }

                m.log.Warn("unauthorized request",
                        logger.String("request_id", GetRequestID(r.Context())),
                        logger.String("remote_addr", r.RemoteAddr),
                        logger.String("method", r.Method),
                        logger.String("path", r.URL.Path),
                )
                m.writeUnauthorized(w, r, "invalid_token", "Invalid or expired token")
        })
}

func GetAuthToken(ctx context.Context) string {
        if v, ok := ctx.Value(AuthTokenKey).(string); ok {
                return v
        }
        return ""
}

func GetAuthUser(ctx context.Context) string {
        if v, ok := ctx.Value(AuthUserKey).(string); ok {
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
        if h != "" {
                if strings.EqualFold(m.cfg.HeaderName, "Authorization") {
                        if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
                                return "", false
                        }
                        token := strings.TrimSpace(h[len("Bearer "):])
                        return normalizeToken(token)
                }
                return normalizeToken(h)
        }

        if strings.HasPrefix(r.URL.Path, "/ws/") || strings.HasPrefix(r.URL.Path, "/ws") {
                if t := r.URL.Query().Get("token"); t != "" {
                        return normalizeToken(t)
                }
        }

        return "", false
}

func normalizeToken(token string) (string, bool) {
        token = strings.TrimSpace(token)
        if token == "" {
                return "", false
        }
        if !utf8.ValidString(token) {
                return "", false
        }
        if len(token) < 8 || len(token) > 2048 {
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

func (m *AuthMiddleware) validateJWT(tokenString string) (string, error) {
        parts := strings.Split(tokenString, ".")
        if len(parts) != 3 {
                return "", http.ErrNoCookie
        }

        signingInput := parts[0] + "." + parts[1]
        mac := hmac.New(sha256.New, []byte(m.cfg.JWTSecret))
        mac.Write([]byte(signingInput))
        expectedSig := mac.Sum(nil)

        actualSig, err := jwtBase64Decode(parts[2])
        if err != nil {
                return "", err
        }

        if !hmac.Equal(expectedSig, actualSig) {
                return "", http.ErrNoCookie
        }

        payloadBytes, err := jwtBase64Decode(parts[1])
        if err != nil {
                return "", err
        }

        var claims struct {
                Sub string `json:"sub"`
                Exp int64  `json:"exp"`
        }
        if err := json.Unmarshal(payloadBytes, &claims); err != nil {
                return "", err
        }

        if time.Now().Unix() > claims.Exp {
                return "", http.ErrNoCookie
        }

        return claims.Sub, nil
}

func jwtBase64Decode(s string) ([]byte, error) {
        switch len(s) % 4 {
        case 2:
                s += "=="
        case 3:
                s += "="
        }
        return base64.URLEncoding.DecodeString(s)
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
