package models

import (
	"errors"
	"fmt"
	"net/mail"
	"time"
)

type Provider string

const (
	ProviderGmail          Provider = "gmail"
	ProviderYahoo          Provider = "yahoo"
	ProviderOutlook        Provider = "outlook"
	ProviderHotmail        Provider = "hotmail"
	ProviderICloud         Provider = "icloud"
	ProviderWorkspace      Provider = "workspace"
	ProviderSMTP           Provider = "smtp"
	ProviderCustom         Provider = "custom"
)

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusFailed    AccountStatus = "failed"
	AccountStatusCooldown  AccountStatus = "cooldown"
	AccountStatusDisabled  AccountStatus = "disabled"
)


type AuthenticationType string

const (
	AuthTypeOAuth2      AuthenticationType = "oauth2"
	AuthTypeAppPassword AuthenticationType = "app_password"
	AuthTypeSMTP        AuthenticationType = "smtp"
	AuthTypeAPIKey      AuthenticationType = "api_key"
)

type Account struct {
	ID                  string             `json:"id" db:"id"`
	TenantID            string             `json:"tenant_id" db:"tenant_id"`
	Name                string             `json:"name" db:"name"`
	Email               string             `json:"email" db:"email"`
	Provider            Provider           `json:"provider" db:"provider"`
	AuthType            AuthenticationType `json:"auth_type" db:"auth_type"`
	Status              AccountStatus      `json:"status" db:"status"`
	IsDefault           bool               `json:"is_default" db:"is_default"`
	Priority            int                `json:"priority" db:"priority"`
	Weight              float64            `json:"weight" db:"weight"`
	Tags                []string           `json:"tags" db:"tags"`
	Credentials         *AccountCredentials `json:"credentials,omitempty"`
	SMTPConfig          *SMTPConfig        `json:"smtp_config,omitempty"`
	OAuth2Config        *OAuth2Config      `json:"oauth2_config,omitempty"`
	HealthMetrics       AccountHealth      `json:"health_metrics"`
	Limits              AccountLimits      `json:"limits"`
	Stats               AccountStats       `json:"stats"`
	SuspensionInfo      *SuspensionInfo    `json:"suspension_info,omitempty"`
	RotationInfo        AccountRotation    `json:"rotation_info"`
	LastUsedAt          *time.Time         `json:"last_used_at" db:"last_used_at"`
	LastTestedAt        *time.Time         `json:"last_tested_at" db:"last_tested_at"`
	LastHealthCheckAt   *time.Time         `json:"last_health_check_at" db:"last_health_check_at"`
	LastErrorAt         *time.Time         `json:"last_error_at" db:"last_error_at"`
	LastError           string             `json:"last_error" db:"last_error"`
	CooldownUntil       *time.Time         `json:"cooldown_until" db:"cooldown_until"`
	CreatedAt           time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at" db:"updated_at"`
	CreatedBy           string             `json:"created_by" db:"created_by"`
	UpdatedBy           string             `json:"updated_by" db:"updated_by"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type AccountCredentials struct {
	Password         string `json:"password,omitempty"`
	AppPassword      string `json:"app_password,omitempty"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`
	TokenExpiry      *time.Time `json:"token_expiry,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	ClientSecret     string `json:"client_secret,omitempty"`
	EncryptionKey    string `json:"encryption_key,omitempty"`
	IsEncrypted      bool   `json:"is_encrypted"`
}

type SMTPConfig struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	UseTLS             bool   `json:"use_tls"`
	UseSSL             bool   `json:"use_ssl"`
	UseSTARTTLS        bool   `json:"use_starttls"`
	SkipVerify         bool   `json:"skip_verify"`
	ConnectionTimeout  int    `json:"connection_timeout"`
	SendTimeout        int    `json:"send_timeout"`
	KeepAlive          bool   `json:"keep_alive"`
	MaxConnections     int    `json:"max_connections"`
	MaxIdleConnections int    `json:"max_idle_connections"`
	LocalName          string `json:"local_name,omitempty"`
}

type OAuth2Config struct {
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes"`
	GrantType    string   `json:"grant_type"`
	ResponseType string   `json:"response_type"`
	State        string   `json:"state,omitempty"`
}

type AccountHealth struct {
	HealthScore           float64   `json:"health_score"`
	IsHealthy             bool      `json:"is_healthy"`
	ConsecutiveSuccesses  int       `json:"consecutive_successes"`
	ConsecutiveFailures   int       `json:"consecutive_failures"`
	TotalSuccesses        int64     `json:"total_successes"`
	TotalFailures         int64     `json:"total_failures"`
	SuccessRate           float64   `json:"success_rate"`
	FailureRate           float64   `json:"failure_rate"`
	AverageResponseTimeMs float64   `json:"average_response_time_ms"`
	LastResponseTimeMs    float64   `json:"last_response_time_ms"`
	SpamComplaintCount    int64     `json:"spam_complaint_count"`
	BounceCount           int64     `json:"bounce_count"`
	HardBounceCount       int64     `json:"hard_bounce_count"`
	SoftBounceCount       int64     `json:"soft_bounce_count"`
	SpamScore             float64   `json:"spam_score"`
	ReputationScore       float64   `json:"reputation_score"`
	LastCalculatedAt      time.Time `json:"last_calculated_at"`
}

type AccountLimits struct {
	DailyLimit           int   `json:"daily_limit"`
	HourlyLimit          int   `json:"hourly_limit"`
	PerMinuteLimit       int   `json:"per_minute_limit"`
	RotationLimit        int   `json:"rotation_limit"`
	MaxConsecutiveSends  int   `json:"max_consecutive_sends"`
	CooldownPeriodSec    int   `json:"cooldown_period_sec"`
	SentToday            int64 `json:"sent_today"`
	SentThisHour         int64 `json:"sent_this_hour"`
	SentThisMinute       int64 `json:"sent_this_minute"`
	SentSinceRotation    int64 `json:"sent_since_rotation"`
	RemainingDaily       int64 `json:"remaining_daily"`
	RemainingHourly      int64 `json:"remaining_hourly"`
	RemainingMinute      int64 `json:"remaining_minute"`
	LimitResetAt         *time.Time `json:"limit_reset_at,omitempty"`
	DailyResetAt         *time.Time `json:"daily_reset_at,omitempty"`
}

type AccountStats struct {
	TotalSent        int64     `json:"total_sent"`
	TotalDelivered   int64     `json:"total_delivered"`
	TotalFailed      int64     `json:"total_failed"`
	TotalBounced     int64     `json:"total_bounced"`
	TotalComplaints  int64     `json:"total_complaints"`
	TotalOpens       int64     `json:"total_opens"`
	TotalClicks      int64     `json:"total_clicks"`
	DeliveryRate     float64   `json:"delivery_rate"`
	OpenRate         float64   `json:"open_rate"`
	ClickRate        float64   `json:"click_rate"`
	BounceRate       float64   `json:"bounce_rate"`
	ComplaintRate    float64   `json:"complaint_rate"`
	LastSentAt       *time.Time `json:"last_sent_at,omitempty"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
}

type SuspensionInfo struct {
	SuspendedAt     time.Time `json:"suspended_at"`
	SuspendedBy     string    `json:"suspended_by"`
	SuspensionReason string   `json:"suspension_reason"`
	IsAutoSuspended bool      `json:"is_auto_suspended"`
	ResumeAt        *time.Time `json:"resume_at,omitempty"`
	CanResume       bool      `json:"can_resume"`
	AttemptCount    int       `json:"attempt_count"`
}

type AccountRotation struct {
	TimesUsed           int64      `json:"times_used"`
	TimesRotated        int64      `json:"times_rotated"`
	LastRotatedAt       *time.Time `json:"last_rotated_at,omitempty"`
	CurrentRotationIndex int       `json:"current_rotation_index"`
	RotationWeight      float64    `json:"rotation_weight"`
	PreferredTimeSlots  []string   `json:"preferred_time_slots,omitempty"`
}

func NewAccount(email string, provider Provider, tenantID, createdBy string) (*Account, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}

	now := time.Now()
	dailyReset := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())

	return &Account{
		ID:          generateAccountID(),
		TenantID:    tenantID,
		Email:       email,
		Provider:    provider,
		Status:      AccountStatusDisabled,
		Priority:    100,
		Weight:      1.0,
		Tags:        []string{},
		HealthMetrics: AccountHealth{
			HealthScore:     100.0,
			IsHealthy:       true,
			LastCalculatedAt: now,
		},
		Limits: AccountLimits{
			DailyLimit:      getDefaultDailyLimit(provider),
			HourlyLimit:     getDefaultHourlyLimit(provider),
			PerMinuteLimit:  getDefaultPerMinuteLimit(provider),
			RotationLimit:   100,
			CooldownPeriodSec: 300,
			DailyResetAt:    &dailyReset,
		},
		Stats: AccountStats{
			LastUpdatedAt: now,
		},
		RotationInfo: AccountRotation{
			RotationWeight: 1.0,
		},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: createdBy,
		UpdatedBy: createdBy,
		Metadata:  make(map[string]interface{}),
	}, nil
}

func (a *Account) Validate() error {
	if a.Email == "" {
		return errors.New("email is required")
	}
	if err := validateEmail(a.Email); err != nil {
		return err
	}
	if a.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if !a.Provider.IsValid() {
		return fmt.Errorf("invalid provider: %s", a.Provider)
	}
	if !a.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", a.Status)
	}
	if a.AuthType != "" && !a.AuthType.IsValid() {
		return fmt.Errorf("invalid auth type: %s", a.AuthType)
	}
	if a.Priority < 0 || a.Priority > 1000 {
		return errors.New("priority must be between 0 and 1000")
	}
	if a.Weight < 0 || a.Weight > 100 {
		return errors.New("weight must be between 0 and 100")
	}
	return nil
}

func (a *Account) IsActive() bool {
	return a.Status == AccountStatusActive
}

func (a *Account) IsSuspended() bool {
	return a.Status == AccountStatusSuspended
}

func (a *Account) IsInCooldown() bool {
	if a.CooldownUntil == nil {
		return false
	}
	return time.Now().Before(*a.CooldownUntil)
}

func (a *Account) CanSend() bool {
	if !a.IsActive() {
		return false
	}
	if a.IsInCooldown() {
		return false
	}
	if a.Limits.RemainingDaily <= 0 {
		return false
	}
	if a.Limits.RemainingHourly <= 0 {
		return false
	}
	if a.Limits.RemainingMinute <= 0 {
		return false
	}
	if !a.HealthMetrics.IsHealthy {
		return false
	}
	return true
}

func (a *Account) Activate() error {
	if a.Status == AccountStatusActive {
		return errors.New("account is already active")
	}
	a.Status = AccountStatusActive
	a.UpdatedAt = time.Now()
	return nil
}

func (a *Account) Suspend(reason string, autoSuspended bool, suspendedBy string) error {
	if a.IsSuspended() {
		return errors.New("account is already suspended")
	}
	
	now := time.Now()
	a.Status = AccountStatusSuspended
	a.SuspensionInfo = &SuspensionInfo{
		SuspendedAt:      now,
		SuspendedBy:      suspendedBy,
		SuspensionReason: reason,
		IsAutoSuspended:  autoSuspended,
		CanResume:        true,
		AttemptCount:     0,
	}
	a.UpdatedAt = now
	return nil
}

func (a *Account) Resume() error {
	if !a.IsSuspended() {
		return errors.New("account is not suspended")
	}
	if a.SuspensionInfo != nil && !a.SuspensionInfo.CanResume {
		return errors.New("account cannot be resumed")
	}
	
	a.Status = AccountStatusActive
	a.SuspensionInfo = nil
	a.UpdatedAt = time.Now()
	return nil
}

func (a *Account) SetCooldown(durationSec int) {
	cooldownUntil := time.Now().Add(time.Duration(durationSec) * time.Second)
	a.CooldownUntil = &cooldownUntil
	a.Status = AccountStatusCooldown
	a.UpdatedAt = time.Now()
}

func (a *Account) ClearCooldown() {
	a.CooldownUntil = nil
	if a.Status == AccountStatusCooldown {
		a.Status = AccountStatusActive
	}
	a.UpdatedAt = time.Now()
}

func (a *Account) IncrementSent() {
	a.Limits.SentToday++
	a.Limits.SentThisHour++
	a.Limits.SentThisMinute++
	a.Limits.SentSinceRotation++
	a.Stats.TotalSent++
	
	a.Limits.RemainingDaily = int64(a.Limits.DailyLimit) - a.Limits.SentToday
	a.Limits.RemainingHourly = int64(a.Limits.HourlyLimit) - a.Limits.SentThisHour
	a.Limits.RemainingMinute = int64(a.Limits.PerMinuteLimit) - a.Limits.SentThisMinute
	
	now := time.Now()
	a.LastUsedAt = &now
	a.Stats.LastSentAt = &now
	a.Stats.LastUpdatedAt = now
	a.UpdatedAt = now
}

func (a *Account) IncrementSuccess() {
	a.HealthMetrics.ConsecutiveSuccesses++
	a.HealthMetrics.ConsecutiveFailures = 0
	a.HealthMetrics.TotalSuccesses++
	a.Stats.TotalDelivered++
	a.calculateHealthScore()
	a.UpdatedAt = time.Now()
}

func (a *Account) IncrementFailure(errorMsg string) {
	a.HealthMetrics.ConsecutiveFailures++
	a.HealthMetrics.ConsecutiveSuccesses = 0
	a.HealthMetrics.TotalFailures++
	a.Stats.TotalFailed++
	
	now := time.Now()
	a.LastErrorAt = &now
	a.LastError = errorMsg
	
	a.calculateHealthScore()
	a.UpdatedAt = now
}

func (a *Account) calculateHealthScore() {
	total := a.HealthMetrics.TotalSuccesses + a.HealthMetrics.TotalFailures
	if total == 0 {
		a.HealthMetrics.HealthScore = 100.0
		a.HealthMetrics.IsHealthy = true
		return
	}
	
	successRate := float64(a.HealthMetrics.TotalSuccesses) / float64(total)
	a.HealthMetrics.SuccessRate = successRate
	a.HealthMetrics.FailureRate = 1.0 - successRate
	
	baseScore := successRate * 100.0
	
	consecutiveFailurePenalty := float64(a.HealthMetrics.ConsecutiveFailures) * 5.0
	spamPenalty := float64(a.HealthMetrics.SpamComplaintCount) * 10.0
	bouncePenalty := float64(a.HealthMetrics.HardBounceCount) * 5.0
	
	score := baseScore - consecutiveFailurePenalty - spamPenalty - bouncePenalty
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	
	a.HealthMetrics.HealthScore = score
	a.HealthMetrics.IsHealthy = score >= 70.0
	a.HealthMetrics.LastCalculatedAt = time.Now()
}

func (a *Account) RecordBounce(isHard bool) {
	a.Stats.TotalBounced++
	a.HealthMetrics.BounceCount++
	
	if isHard {
		a.HealthMetrics.HardBounceCount++
	} else {
		a.HealthMetrics.SoftBounceCount++
	}
	
	a.calculateHealthScore()
	a.calculateRates()
	a.UpdatedAt = time.Now()
}

func (a *Account) RecordComplaint() {
	a.Stats.TotalComplaints++
	a.HealthMetrics.SpamComplaintCount++
	a.calculateHealthScore()
	a.calculateRates()
	a.UpdatedAt = time.Now()
}

func (a *Account) calculateRates() {
	if a.Stats.TotalSent > 0 {
		a.Stats.DeliveryRate = float64(a.Stats.TotalDelivered) / float64(a.Stats.TotalSent)
		a.Stats.BounceRate = float64(a.Stats.TotalBounced) / float64(a.Stats.TotalSent)
		a.Stats.ComplaintRate = float64(a.Stats.TotalComplaints) / float64(a.Stats.TotalSent)
	}
	
	if a.Stats.TotalDelivered > 0 {
		a.Stats.OpenRate = float64(a.Stats.TotalOpens) / float64(a.Stats.TotalDelivered)
		a.Stats.ClickRate = float64(a.Stats.TotalClicks) / float64(a.Stats.TotalDelivered)
	}
	
	a.Stats.LastUpdatedAt = time.Now()
}

func (a *Account) ResetDailyLimits() {
	a.Limits.SentToday = 0
	a.Limits.RemainingDaily = int64(a.Limits.DailyLimit)
	now := time.Now()
	dailyReset := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	a.Limits.DailyResetAt = &dailyReset
	a.UpdatedAt = now
}

func (a *Account) ResetHourlyLimits() {
	a.Limits.SentThisHour = 0
	a.Limits.RemainingHourly = int64(a.Limits.HourlyLimit)
	a.UpdatedAt = time.Now()
}

func (a *Account) ResetMinuteLimits() {
	a.Limits.SentThisMinute = 0
	a.Limits.RemainingMinute = int64(a.Limits.PerMinuteLimit)
	a.UpdatedAt = time.Now()
}

func (a *Account) ResetRotationCounter() {
	a.Limits.SentSinceRotation = 0
	now := time.Now()
	a.RotationInfo.LastRotatedAt = &now
	a.RotationInfo.TimesRotated++
	a.UpdatedAt = now
}

func (a *Account) NeedsRotation() bool {
	if a.Limits.RotationLimit <= 0 {
		return false
	}
	return a.Limits.SentSinceRotation >= int64(a.Limits.RotationLimit)
}

func (a *Account) TestConnection() error {
	if a.Credentials == nil {
		return errors.New("credentials not configured")
	}
	
	now := time.Now()
	a.LastTestedAt = &now
	a.UpdatedAt = now
	
	return nil
}

func (p Provider) IsValid() bool {
	switch p {
	case ProviderGmail, ProviderYahoo, ProviderOutlook, ProviderHotmail,
		ProviderICloud, ProviderWorkspace, ProviderSMTP, ProviderCustom:
		return true
	}
	return false
}
func (s AccountStatus) IsValid() bool {
	switch s {
	case AccountStatusActive, AccountStatusSuspended,
		AccountStatusFailed, AccountStatusCooldown, AccountStatusDisabled:
		return true
	}
	return false
}


func (a AuthenticationType) IsValid() bool {
	switch a {
	case AuthTypeOAuth2, AuthTypeAppPassword, AuthTypeSMTP, AuthTypeAPIKey:
		return true
	}
	return false
}

func validateEmail(email string) error {
	_, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email address: %w", err)
	}
	return nil
}

func getDefaultDailyLimit(provider Provider) int {
	switch provider {
	case ProviderGmail:
		return 500
	case ProviderWorkspace:
		return 2000
	case ProviderYahoo:
		return 500
	case ProviderOutlook, ProviderHotmail:
		return 300
	case ProviderICloud:
		return 200
	default:
		return 500
	}
}

func getDefaultHourlyLimit(provider Provider) int {
	return getDefaultDailyLimit(provider) / 12
}

func getDefaultPerMinuteLimit(provider Provider) int {
	return getDefaultHourlyLimit(provider) / 60
}

func generateAccountID() string {
	return fmt.Sprintf("acc_%d", time.Now().UnixNano())
}

func (a *Account) GetSMTPAddress() string {
	if a.SMTPConfig == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", a.SMTPConfig.Host, a.SMTPConfig.Port)
}

func (a *Account) RequiresOAuth2() bool {
	return a.AuthType == AuthTypeOAuth2
}

func (a *Account) HasValidToken() bool {
	if a.Credentials == nil || a.Credentials.AccessToken == "" {
		return false
	}
	if a.Credentials.TokenExpiry == nil {
		return true
	}
	return time.Now().Before(*a.Credentials.TokenExpiry)
}

func (a *Account) TokenNeedsRefresh() bool {
	if a.Credentials == nil || a.Credentials.TokenExpiry == nil {
		return false
	}
	refreshThreshold := a.Credentials.TokenExpiry.Add(-5 * time.Minute)
	return time.Now().After(refreshThreshold)
}

func (a *Account) Clone() *Account {
	clone := *a
	clone.ID = generateAccountID()
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.Stats = AccountStats{LastUpdatedAt: now}
	clone.HealthMetrics.LastCalculatedAt = now
	return &clone
}
