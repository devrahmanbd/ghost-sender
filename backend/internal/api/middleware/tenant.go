package middleware

import (
	"context"
	"net/http"
	"strings"

	"email-campaign-system/pkg/logger"
)

type tenantContextKey string

const TenantIDKey tenantContextKey = "tenantid"

type TenantConfig struct {
	Enabled       bool
	HeaderName    string
	DefaultTenant string
	Required      bool
}

type TenantMiddleware struct {
	log logger.Logger
	cfg TenantConfig
}

func NewTenantMiddleware(log logger.Logger, cfg TenantConfig) *TenantMiddleware {
	if cfg.HeaderName == "" {
		cfg.HeaderName = "X-Tenant-ID"
	}
	if cfg.DefaultTenant == "" {
		cfg.DefaultTenant = "default"
	}

	return &TenantMiddleware{
		log: log,
		cfg: cfg,
	}
}

func (m *TenantMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			ctx := context.WithValue(r.Context(), TenantIDKey, m.cfg.DefaultTenant)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		tenantID := m.extractTenantID(r)

		if tenantID == "" {
			if m.cfg.Required {
				m.log.Warn("missing tenant ID",
					logger.String("path", r.URL.Path),
					logger.String("method", r.Method),
				)
				http.Error(w, "Tenant ID required", http.StatusBadRequest)
				return
			}
			tenantID = m.cfg.DefaultTenant
		}

		ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *TenantMiddleware) extractTenantID(r *http.Request) string {
	tenantID := strings.TrimSpace(r.Header.Get(m.cfg.HeaderName))
	if tenantID != "" {
		return tenantID
	}

	tenantID = strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if tenantID != "" {
		return tenantID
	}

	host := r.Host
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		return parts[0]
	}

	return ""
}

func GetTenantID(ctx context.Context) string {
	if v, ok := ctx.Value(TenantIDKey).(string); ok {
		return v
	}
	return "default"
}
