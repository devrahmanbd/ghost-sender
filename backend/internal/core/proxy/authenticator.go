package proxy

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
)

var (
	ErrAuthenticationFailed   = errors.New("proxy authentication failed")
	ErrInvalidCredentials     = errors.New("invalid proxy credentials")
	ErrUnsupportedAuthScheme  = errors.New("unsupported authentication scheme")
	ErrMissingAuthCredentials = errors.New("missing authentication credentials")
)

type AuthScheme string

const (
	AuthSchemeBasic  AuthScheme = "Basic"
	AuthSchemeDigest AuthScheme = "Digest"
	AuthSchemeNTLM   AuthScheme = "NTLM"
)

type ProxyAuthenticator struct {
	mu              sync.RWMutex
	credentials     map[string]*models.Proxy
	authCache       map[string]*AuthToken
	cacheTTL        time.Duration
	supportedSchemes []AuthScheme
}

type AuthToken struct {
	Token      string
	Scheme     AuthScheme
	ExpiresAt  time.Time
	CreatedAt  time.Time
	ProxyID    string
}

type AuthConfig struct {
	CacheTTL         time.Duration
	EnableCaching    bool
	SupportedSchemes []AuthScheme
	MaxRetries       int
	RetryDelay       time.Duration
}

type DigestChallenge struct {
	Realm     string
	Qop       string
	Nonce     string
	Opaque    string
	Algorithm string
}

func NewProxyAuthenticator(config *AuthConfig) *ProxyAuthenticator {
	if config == nil {
		config = DefaultAuthConfig()
	}

	return &ProxyAuthenticator{
		credentials:      make(map[string]*models.Proxy),
		authCache:        make(map[string]*AuthToken),
		cacheTTL:         config.CacheTTL,
		supportedSchemes: config.SupportedSchemes,
	}
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		CacheTTL:      30 * time.Minute,
		EnableCaching: true,
		SupportedSchemes: []AuthScheme{
			AuthSchemeBasic,
			AuthSchemeDigest,
		},
		MaxRetries: 3,
		RetryDelay: 2 * time.Second,
	}
}

func (pa *ProxyAuthenticator) RegisterProxy(p *models.Proxy) error {
	if p == nil {
		return errors.New("proxy is nil")
	}

	if p.ID == "" {
		return errors.New("proxy ID is required")
	}

	if p.Username == "" && p.Password != "" {
		return ErrInvalidCredentials
	}

	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.credentials[p.ID] = p
	return nil
}

func (pa *ProxyAuthenticator) UnregisterProxy(proxyID string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	delete(pa.credentials, proxyID)
	delete(pa.authCache, proxyID)
}

func (pa *ProxyAuthenticator) GetAuthHeader(p *models.Proxy) (string, error) {
	if p.Username == "" {
		return "", nil
	}

	pa.mu.RLock()
	cached, exists := pa.authCache[p.ID]
	pa.mu.RUnlock()

	if exists && !cached.IsExpired() {
		return cached.Token, nil
	}

	token, err := pa.generateBasicAuth(p.Username, p.Password)
	if err != nil {
		return "", err
	}

	pa.cacheAuthToken(p.ID, token, AuthSchemeBasic)

	return token, nil
}

func (pa *ProxyAuthenticator) generateBasicAuth(username, password string) (string, error) {
	if username == "" {
		return "", ErrMissingAuthCredentials
	}

	auth := username + ":" + password
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	return "Basic " + encoded, nil
}

func (pa *ProxyAuthenticator) GenerateDigestAuth(username, password, method, uri string, challenge *DigestChallenge) (string, error) {
	if challenge == nil {
		return "", errors.New("digest challenge is nil")
	}

	ha1 := md5Hash(username + ":" + challenge.Realm + ":" + password)
	ha2 := md5Hash(method + ":" + uri)

	var response string
	if challenge.Qop == "auth" || challenge.Qop == "auth-int" {
		nc := "00000001"
		cnonce := generateCNonce()
		response = md5Hash(ha1 + ":" + challenge.Nonce + ":" + nc + ":" + cnonce + ":" + challenge.Qop + ":" + ha2)

		return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=%s, nc=%s, cnonce="%s", response="%s", opaque="%s"`,
			username, challenge.Realm, challenge.Nonce, uri, challenge.Qop, nc, cnonce, response, challenge.Opaque), nil
	}

	response = md5Hash(ha1 + ":" + challenge.Nonce + ":" + ha2)
	return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", opaque="%s"`,
		username, challenge.Realm, challenge.Nonce, uri, response, challenge.Opaque), nil
}

func (pa *ProxyAuthenticator) ParseAuthChallenge(authHeader string) (*DigestChallenge, error) {
	if !strings.HasPrefix(authHeader, "Digest ") {
		return nil, ErrUnsupportedAuthScheme
	}

	challenge := &DigestChallenge{}
	parts := strings.Split(authHeader[7:], ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.Trim(strings.TrimSpace(kv[1]), `"`)

		switch key {
		case "realm":
			challenge.Realm = value
		case "qop":
			challenge.Qop = value
		case "nonce":
			challenge.Nonce = value
		case "opaque":
			challenge.Opaque = value
		case "algorithm":
			challenge.Algorithm = value
		}
	}

	return challenge, nil
}

func (pa *ProxyAuthenticator) ValidateCredentials(p *models.Proxy) error {
	if p.Username == "" && p.Password == "" {
		return nil
	}

	if p.Username == "" {
		return errors.New("username is required when password is set")
	}

	if p.Password == "" {
		return errors.New("password is required when username is set")
	}

	if len(p.Username) > 255 {
		return errors.New("username too long")
	}

	if len(p.Password) > 255 {
		return errors.New("password too long")
	}

	return nil
}

func (pa *ProxyAuthenticator) cacheAuthToken(proxyID, token string, scheme AuthScheme) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.authCache[proxyID] = &AuthToken{
		Token:     token,
		Scheme:    scheme,
		ExpiresAt: time.Now().Add(pa.cacheTTL),
		CreatedAt: time.Now(),
		ProxyID:   proxyID,
	}
}

func (pa *ProxyAuthenticator) ClearCache() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.authCache = make(map[string]*AuthToken)
}

func (pa *ProxyAuthenticator) ClearExpiredTokens() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	now := time.Now()
	for id, token := range pa.authCache {
		if token.ExpiresAt.Before(now) {
			delete(pa.authCache, id)
		}
	}
}

func (pa *ProxyAuthenticator) GetCachedToken(proxyID string) (*AuthToken, bool) {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	token, exists := pa.authCache[proxyID]
	if !exists || token.IsExpired() {
		return nil, false
	}

	return token, true
}

func (pa *ProxyAuthenticator) IsSchemeSupported(scheme AuthScheme) bool {
	for _, supported := range pa.supportedSchemes {
		if supported == scheme {
			return true
		}
	}
	return false
}

func (pa *ProxyAuthenticator) AddAuthToRequest(req *http.Request, p *models.Proxy) error {
	if p.Username == "" {
		return nil
	}

	authHeader, err := pa.GetAuthHeader(p)
	if err != nil {
		return err
	}

	if authHeader != "" {
		req.Header.Set("Proxy-Authorization", authHeader)
	}

	return nil
}

func (pa *ProxyAuthenticator) TestAuthentication(p *models.Proxy) error {
	if p.Username == "" && p.Password == "" {
		return nil
	}

	if err := pa.ValidateCredentials(p); err != nil {
		return err
	}

	_, err := pa.GetAuthHeader(p)
	return err
}

func (at *AuthToken) IsExpired() bool {
	return time.Now().After(at.ExpiresAt)
}

func (at *AuthToken) TimeUntilExpiry() time.Duration {
	return time.Until(at.ExpiresAt)
}

func (at *AuthToken) Refresh(ttl time.Duration) {
	at.ExpiresAt = time.Now().Add(ttl)
}

func md5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return fmt.Sprintf("%x", hash)
}

func generateCNonce() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func EncodeBasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func DecodeBasicAuth(encoded string) (username, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid basic auth format")
	}

	return parts[0], parts[1], nil
}

func ExtractAuthScheme(authHeader string) AuthScheme {
	if strings.HasPrefix(authHeader, "Basic ") {
		return AuthSchemeBasic
	}
	if strings.HasPrefix(authHeader, "Digest ") {
		return AuthSchemeDigest
	}
	if strings.HasPrefix(authHeader, "NTLM ") {
		return AuthSchemeNTLM
	}
	return ""
}

