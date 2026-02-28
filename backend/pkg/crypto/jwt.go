package crypto

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DefaultTokenExpiration = 24 * time.Hour
	DefaultRefreshExpiration = 7 * 24 * time.Hour
	DefaultIssuer = "email-campaign-system"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
	ErrInvalidSigningMethod = errors.New("invalid signing method")
	ErrMissingClaims = errors.New("missing required claims")
	ErrInvalidAudience = errors.New("invalid audience")
	ErrInvalidIssuer = errors.New("invalid issuer")
	ErrTokenRevoked = errors.New("token has been revoked")
)

type SigningMethod string

const (
	SigningMethodHS256 SigningMethod = "HS256"
	SigningMethodHS384 SigningMethod = "HS384"
	SigningMethodHS512 SigningMethod = "HS512"
	SigningMethodRS256 SigningMethod = "RS256"
	SigningMethodRS384 SigningMethod = "RS384"
	SigningMethodRS512 SigningMethod = "RS512"
	SigningMethodES256 SigningMethod = "ES256"
	SigningMethodES384 SigningMethod = "ES384"
	SigningMethodES512 SigningMethod = "ES512"
)

type Claims struct {
	UserID      string   `json:"user_id,omitempty"`
	Email       string   `json:"email,omitempty"`
	TenantID    string   `json:"tenant_id,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	TokenType   string   `json:"token_type,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type JWTManager struct {
	secretKey       []byte
	privateKey      *rsa.PrivateKey
	publicKey       *rsa.PublicKey
	signingMethod   jwt.SigningMethod
	issuer          string
	audience        []string
	tokenExpiration time.Duration
	refreshExpiration time.Duration
}

type JWTOption func(*JWTManager)

func WithSecretKey(key []byte) JWTOption {
	return func(m *JWTManager) {
		m.secretKey = key
	}
}

func WithRSAKeys(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) JWTOption {
	return func(m *JWTManager) {
		m.privateKey = privateKey
		m.publicKey = publicKey
	}
}

func WithSigningMethod(method SigningMethod) JWTOption {
	return func(m *JWTManager) {
		switch method {
		case SigningMethodHS256:
			m.signingMethod = jwt.SigningMethodHS256
		case SigningMethodHS384:
			m.signingMethod = jwt.SigningMethodHS384
		case SigningMethodHS512:
			m.signingMethod = jwt.SigningMethodHS512
		case SigningMethodRS256:
			m.signingMethod = jwt.SigningMethodRS256
		case SigningMethodRS384:
			m.signingMethod = jwt.SigningMethodRS384
		case SigningMethodRS512:
			m.signingMethod = jwt.SigningMethodRS512
		case SigningMethodES256:
			m.signingMethod = jwt.SigningMethodES256
		case SigningMethodES384:
			m.signingMethod = jwt.SigningMethodES384
		case SigningMethodES512:
			m.signingMethod = jwt.SigningMethodES512
		default:
			m.signingMethod = jwt.SigningMethodHS256
		}
	}
}

func WithIssuer(issuer string) JWTOption {
	return func(m *JWTManager) {
		m.issuer = issuer
	}
}

func WithAudience(audience []string) JWTOption {
	return func(m *JWTManager) {
		m.audience = audience
	}
}

func WithTokenExpiration(duration time.Duration) JWTOption {
	return func(m *JWTManager) {
		m.tokenExpiration = duration
	}
}

func WithRefreshExpiration(duration time.Duration) JWTOption {
	return func(m *JWTManager) {
		m.refreshExpiration = duration
	}
}

func NewJWTManager(opts ...JWTOption) *JWTManager {
	manager := &JWTManager{
		signingMethod:     jwt.SigningMethodHS256,
		issuer:            DefaultIssuer,
		tokenExpiration:   DefaultTokenExpiration,
		refreshExpiration: DefaultRefreshExpiration,
	}

	for _, opt := range opts {
		opt(manager)
	}

	return manager
}

func (m *JWTManager) GenerateToken(claims *Claims) (string, error) {
	now := time.Now()
	
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(now.Add(m.tokenExpiration))
	}
	
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(now)
	}
	
	if claims.NotBefore == nil {
		claims.NotBefore = jwt.NewNumericDate(now)
	}
	
	if claims.Issuer == "" {
		claims.Issuer = m.issuer
	}
	
	if len(claims.Audience) == 0 && len(m.audience) > 0 {
		claims.Audience = m.audience
	}

	token := jwt.NewWithClaims(m.signingMethod, claims)

	var signedToken string
	var err error

	if m.privateKey != nil {
		signedToken, err = token.SignedString(m.privateKey)
	} else {
		signedToken, err = token.SignedString(m.secretKey)
	}

	if err != nil {
		return "", err
	}

	return signedToken, nil
}

func (m *JWTManager) GenerateAccessToken(userID, email, tenantID string, roles, permissions []string) (string, error) {
	claims := &Claims{
		UserID:      userID,
		Email:       email,
		TenantID:    tenantID,
		Roles:       roles,
		Permissions: permissions,
		TokenType:   "access",
	}

	return m.GenerateToken(claims)
}

func (m *JWTManager) GenerateRefreshToken(userID, sessionID string) (string, error) {
	now := time.Now()
	
	claims := &Claims{
		UserID:    userID,
		TokenType: "refresh",
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    m.issuer,
			Audience:  m.audience,
		},
	}

	return m.GenerateToken(claims)
}

func (m *JWTManager) GenerateTokenPair(userID, email, tenantID, sessionID string, roles, permissions []string) (*TokenPair, error) {
	accessToken, err := m.GenerateAccessToken(userID, email, tenantID, roles, permissions)
	if err != nil {
		return nil, err
	}

	refreshToken, err := m.GenerateRefreshToken(userID, sessionID)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(m.tokenExpiration)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(m.tokenExpiration.Seconds()),
		ExpiresAt:    expiresAt,
	}, nil
}

func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != m.signingMethod {
			return nil, ErrInvalidSigningMethod
		}

		if m.publicKey != nil {
			return m.publicKey, nil
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (m *JWTManager) ParseToken(tokenString string) (*Claims, error) {
	return m.ValidateToken(tokenString)
}

func (m *JWTManager) RefreshAccessToken(refreshToken string) (string, error) {
	claims, err := m.ValidateToken(refreshToken)
	if err != nil {
		return "", err
	}

	if claims.TokenType != "refresh" {
		return "", errors.New("invalid token type")
	}

	newClaims := &Claims{
		UserID:    claims.UserID,
		TokenType: "access",
	}

	return m.GenerateToken(newClaims)
}

func (m *JWTManager) VerifyTokenType(tokenString, expectedType string) (*Claims, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != expectedType {
		return nil, fmt.Errorf("expected token type %s, got %s", expectedType, claims.TokenType)
	}

	return claims, nil
}

func (m *JWTManager) GetClaims(tokenString string) (*Claims, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (m *JWTManager) IsExpired(tokenString string) bool {
	claims, err := m.GetClaims(tokenString)
	if err != nil {
		return true
	}

	if claims.ExpiresAt == nil {
		return false
	}

	return claims.ExpiresAt.Time.Before(time.Now())
}

func (m *JWTManager) GetExpirationTime(tokenString string) (time.Time, error) {
	claims, err := m.GetClaims(tokenString)
	if err != nil {
		return time.Time{}, err
	}

	if claims.ExpiresAt == nil {
		return time.Time{}, errors.New("token has no expiration")
	}

	return claims.ExpiresAt.Time, nil
}

func (m *JWTManager) GetRemainingTime(tokenString string) (time.Duration, error) {
	expiresAt, err := m.GetExpirationTime(tokenString)
	if err != nil {
		return 0, err
	}

	remaining := time.Until(expiresAt)
	if remaining < 0 {
		return 0, ErrExpiredToken
	}

	return remaining, nil
}

func GenerateToken(claims *Claims, secretKey []byte) (string, error) {
	manager := NewJWTManager(WithSecretKey(secretKey))
	return manager.GenerateToken(claims)
}

func ValidateToken(tokenString string, secretKey []byte) (*Claims, error) {
	manager := NewJWTManager(WithSecretKey(secretKey))
	return manager.ValidateToken(tokenString)
}

func ParseTokenWithoutValidation(tokenString string) (*Claims, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

type TokenValidator struct {
	validateIssuer   bool
	validateAudience bool
	expectedIssuer   string
	expectedAudience []string
	clockSkew        time.Duration
}

type ValidatorOption func(*TokenValidator)

func WithIssuerValidation(issuer string) ValidatorOption {
	return func(v *TokenValidator) {
		v.validateIssuer = true
		v.expectedIssuer = issuer
	}
}

func WithAudienceValidation(audience []string) ValidatorOption {
	return func(v *TokenValidator) {
		v.validateAudience = true
		v.expectedAudience = audience
	}
}

func WithClockSkew(skew time.Duration) ValidatorOption {
	return func(v *TokenValidator) {
		v.clockSkew = skew
	}
}

func NewTokenValidator(opts ...ValidatorOption) *TokenValidator {
	validator := &TokenValidator{
		clockSkew: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(validator)
	}

	return validator
}

func (v *TokenValidator) Validate(claims *Claims) error {
	now := time.Now()

	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Add(v.clockSkew).Before(now) {
		return ErrExpiredToken
	}

	if claims.NotBefore != nil && claims.NotBefore.Time.After(now.Add(v.clockSkew)) {
		return errors.New("token not valid yet")
	}

	if v.validateIssuer && claims.Issuer != v.expectedIssuer {
		return ErrInvalidIssuer
	}

	if v.validateAudience && len(v.expectedAudience) > 0 {
		valid := false
		for _, aud := range v.expectedAudience {
			for _, claimAud := range claims.Audience {
				if aud == claimAud {
					valid = true
					break
				}
			}
			if valid {
				break
			}
		}
		if !valid {
			return ErrInvalidAudience
		}
	}

	return nil
}

func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is empty")
	}

	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) {
		return "", errors.New("invalid authorization header format")
	}

	if authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", errors.New("authorization header must start with 'Bearer '")
	}

	return authHeader[len(bearerPrefix):], nil
}

func CreateSimpleToken(userID string, secretKey []byte, expiration time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	manager := NewJWTManager(WithSecretKey(secretKey))
	return manager.GenerateToken(claims)
}

func ValidateSimpleToken(tokenString string, secretKey []byte) (string, error) {
	claims, err := ValidateToken(tokenString, secretKey)
	if err != nil {
		return "", err
	}

	return claims.UserID, nil
}
