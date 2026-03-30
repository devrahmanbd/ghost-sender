package handlers

import (
        "crypto/hmac"
        "crypto/sha256"
        "encoding/base64"
        "encoding/json"
        "fmt"
        "net/http"
        "strings"
        "time"

        "email-campaign-system/internal/config"
        "email-campaign-system/pkg/logger"
)

// concurrencyLimiter is the minimal interface AuthHandler needs to update the
// campaign concurrency limit at runtime.  *campaign.Manager satisfies it.
type concurrencyLimiter interface {
        SetMaxConcurrent(n int)
}

type AuthHandler struct {
        cfg             *config.AppConfig
        logger          logger.Logger
        campaignManager concurrencyLimiter
}

func NewAuthHandler(cfg *config.AppConfig, log logger.Logger) *AuthHandler {
        return &AuthHandler{cfg: cfg, logger: log}
}

// WithCampaignManagerLimiter wires the campaign manager so UpdateLicense can
// propagate the new concurrent-campaign limit to the manager immediately.
func (h *AuthHandler) WithCampaignManagerLimiter(mgr concurrencyLimiter) {
        h.campaignManager = mgr
}

type LoginRequest struct {
        Username string `json:"username"`
        Password string `json:"password"`
}

type LoginResponse struct {
        Token                  string `json:"token"`
        ExpiresAt              int64  `json:"expires_at"`
        Username               string `json:"username"`
        MaxConcurrentCampaigns int    `json:"max_concurrent_campaigns"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
        var req LoginRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
                return
        }

        if req.Username == "" || req.Password == "" {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(map[string]string{"error": "Username and password are required"})
                return
        }

        adminUser := h.cfg.Security.AdminUsername
        adminPass := h.cfg.Security.AdminPassword
        if adminUser == "" {
                adminUser = "admin"
        }
        if adminPass == "" {
                adminPass = "admin"
        }

        if req.Username != adminUser || req.Password != adminPass {
                h.logger.Warn("failed login attempt",
                        logger.String("username", req.Username),
                        logger.String("remote_addr", r.RemoteAddr),
                )
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                json.NewEncoder(w).Encode(map[string]string{"error": "Invalid username or password"})
                return
        }

        expiration := h.cfg.Security.JWTExpiration
        if expiration == 0 {
                expiration = 24 * time.Hour
        }
        expiresAt := time.Now().Add(expiration)

        token, err := h.generateJWT(req.Username, expiresAt)
        if err != nil {
                h.logger.Error("failed to generate token", logger.Error(err))
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusInternalServerError)
                json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate token"})
                return
        }

        h.logger.Info("user logged in",
                logger.String("username", req.Username),
                logger.String("remote_addr", r.RemoteAddr),
        )

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(LoginResponse{
                Token:                  token,
                ExpiresAt:              expiresAt.Unix(),
                Username:               req.Username,
                MaxConcurrentCampaigns: h.cfg.Security.MaxConcurrentCampaigns,
        })
}

func (h *AuthHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
                "username":                 h.cfg.Security.AdminUsername,
                "max_concurrent_campaigns": h.cfg.Security.MaxConcurrentCampaigns,
        })
}

func (h *AuthHandler) UpdateLicense(w http.ResponseWriter, r *http.Request) {
        // Require both X-Api-Key and X-Encryption-Key headers regardless of
        // whether those features are individually enabled in config.
        apiKey := r.Header.Get("X-Api-Key")
        encKey := r.Header.Get("X-Encryption-Key")

        if apiKey == "" || encKey == "" {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                json.NewEncoder(w).Encode(map[string]string{"error": "X-Api-Key and X-Encryption-Key headers are required"})
                return
        }
        if apiKey != h.cfg.Security.APIKey {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                json.NewEncoder(w).Encode(map[string]string{"error": "Invalid X-Api-Key"})
                return
        }
        if encKey != h.cfg.Security.EncryptionKey {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                json.NewEncoder(w).Encode(map[string]string{"error": "Invalid X-Encryption-Key"})
                return
        }

        var req struct {
                MaxConcurrentCampaigns int `json:"max_concurrent_campaigns"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
                return
        }

        // -1 = block all, 0 = unlimited, >0 = cap at N.
        // Values below -1 are clamped to -1.
        if req.MaxConcurrentCampaigns < -1 {
                req.MaxConcurrentCampaigns = -1
        }

        // Update the live config — the campaign handler reads from this pointer
        // on every StartCampaign call, so the new limit takes effect immediately.
        h.cfg.Security.MaxConcurrentCampaigns = req.MaxConcurrentCampaigns

        // Also propagate to the campaign manager so the manager-level guard is
        // consistent with the handler-level guard.
        if h.campaignManager != nil {
                h.campaignManager.SetMaxConcurrent(req.MaxConcurrentCampaigns)
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
                "max_concurrent_campaigns": req.MaxConcurrentCampaigns,
                "message":                  "License updated",
        })
}

func (h *AuthHandler) generateJWT(username string, expiresAt time.Time) (string, error) {
        header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

        payload := fmt.Sprintf(`{"sub":"%s","exp":%d,"iat":%d}`,
                username, expiresAt.Unix(), time.Now().Unix())
        payloadEncoded := base64URLEncode([]byte(payload))

        signingInput := header + "." + payloadEncoded
        signature := h.sign([]byte(signingInput))
        signatureEncoded := base64URLEncode(signature)

        return signingInput + "." + signatureEncoded, nil
}

func ValidateJWT(tokenString string, secret string) (string, error) {
        parts := strings.Split(tokenString, ".")
        if len(parts) != 3 {
                return "", fmt.Errorf("invalid token format")
        }

        signingInput := parts[0] + "." + parts[1]
        mac := hmac.New(sha256.New, []byte(secret))
        mac.Write([]byte(signingInput))
        expectedSig := mac.Sum(nil)

        actualSig, err := base64URLDecode(parts[2])
        if err != nil {
                return "", fmt.Errorf("invalid signature encoding")
        }

        if !hmac.Equal(expectedSig, actualSig) {
                return "", fmt.Errorf("invalid signature")
        }

        payloadBytes, err := base64URLDecode(parts[1])
        if err != nil {
                return "", fmt.Errorf("invalid payload encoding")
        }

        var claims struct {
                Sub string `json:"sub"`
                Exp int64  `json:"exp"`
        }
        if err := json.Unmarshal(payloadBytes, &claims); err != nil {
                return "", fmt.Errorf("invalid payload")
        }

        if time.Now().Unix() > claims.Exp {
                return "", fmt.Errorf("token expired")
        }

        return claims.Sub, nil
}

func (h *AuthHandler) sign(data []byte) []byte {
        mac := hmac.New(sha256.New, []byte(h.cfg.Security.JWTSecret))
        mac.Write(data)
        return mac.Sum(nil)
}

func base64URLEncode(data []byte) string {
        return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func base64URLDecode(s string) ([]byte, error) {
        switch len(s) % 4 {
        case 2:
                s += "=="
        case 3:
                s += "="
        }
        return base64.URLEncoding.DecodeString(s)
}
