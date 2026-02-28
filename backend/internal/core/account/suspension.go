package account

import (
	"email-campaign-system/pkg/logger"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SuspensionReason string

const (
	ReasonConsecutiveFailures SuspensionReason = "consecutive_failures"
	ReasonSpamDetected        SuspensionReason = "spam_detected"
	ReasonRateLimitExceeded   SuspensionReason = "rate_limit_exceeded"
	ReasonAuthenticationFail  SuspensionReason = "authentication_failed"
	ReasonQuotaExceeded       SuspensionReason = "quota_exceeded"
	ReasonProviderError       SuspensionReason = "provider_error"
	ReasonManualSuspension    SuspensionReason = "manual_suspension"
	ReasonHealthCheckFailed   SuspensionReason = "health_check_failed"
	ReasonBounceRate          SuspensionReason = "bounce_rate_exceeded"
	ReasonComplaintRate       SuspensionReason = "complaint_rate_exceeded"
)

type SuspensionType string

const (
	SuspensionTypeTemporary SuspensionType = "temporary"
	SuspensionTypePermanent SuspensionType = "permanent"
	SuspensionTypeAutomatic SuspensionType = "automatic"
)

var (
	ErrAlreadySuspended = errors.New("account is already suspended")
	ErrNotSuspended     = errors.New("account is not suspended")
)

type SuspensionManager struct {
	manager             *AccountManager
	log                 logger.Logger // ← CHANGED: Removed pointer
	config              *SuspensionConfig
	suspensionHistory   map[string][]*SuspensionRecord
	historyMu           sync.RWMutex
	suspensionRules     map[SuspensionReason]*SuspensionRule
	rulesMu             sync.RWMutex
	notificationEnabled bool
}

type SuspensionConfig struct {
	FailureThreshold       int
	ConsecutiveThreshold   int
	SpamScoreThreshold     float64
	BounceRateThreshold    float64
	ComplaintRateThreshold float64
	HealthScoreThreshold   float64
	AutoRecoveryEnabled    bool
	RecoveryCheckInterval  time.Duration
	MaxSuspensionDuration  time.Duration
	DefaultSuspensionTime  time.Duration
	EnableNotifications    bool
}

type SuspensionRule struct {
	Reason       SuspensionReason
	Threshold    int
	Duration     time.Duration
	Automatic    bool
	Recoverable  bool
	RecoveryTime time.Duration
	Severity     int
}

type SuspensionRecord struct {
	AccountID    string
	Email        string
	Reason       SuspensionReason
	Type         SuspensionType
	SuspendedAt  time.Time
	ResumeAt     time.Time
	ResumedAt    *time.Time
	Details      string
	ErrorMessage string
	AutoResumed  bool
	Severity     int
	Metadata     map[string]interface{}
}

func NewSuspensionManager(manager *AccountManager, log logger.Logger, threshold int) *SuspensionManager {

	var logValue logger.Logger
	if log != nil {
		logValue = log
	}

	sm := &SuspensionManager{
		manager:           manager,
		log:               logValue, // ← CHANGED
		suspensionHistory: make(map[string][]*SuspensionRecord),
		suspensionRules:   make(map[SuspensionReason]*SuspensionRule),
		config:            DefaultSuspensionConfig(),
	}

	if threshold > 0 {
		sm.config.FailureThreshold = threshold
	}

	sm.initializeDefaultRules()

	if sm.config.AutoRecoveryEnabled {
		go sm.autoRecoveryLoop()
	}

	return sm
}

func DefaultSuspensionConfig() *SuspensionConfig {
	return &SuspensionConfig{
		FailureThreshold:       5,
		ConsecutiveThreshold:   3,
		SpamScoreThreshold:     70.0,
		BounceRateThreshold:    10.0,
		ComplaintRateThreshold: 0.5,
		HealthScoreThreshold:   20.0,
		AutoRecoveryEnabled:    true,
		RecoveryCheckInterval:  5 * time.Minute,
		MaxSuspensionDuration:  24 * time.Hour,
		DefaultSuspensionTime:  1 * time.Hour,
		EnableNotifications:    true,
	}
}

func (sm *SuspensionManager) initializeDefaultRules() {
	rules := []*SuspensionRule{
		{
			Reason:       ReasonConsecutiveFailures,
			Threshold:    5,
			Duration:     30 * time.Minute,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 30 * time.Minute,
			Severity:     3,
		},
		{
			Reason:       ReasonSpamDetected,
			Threshold:    1,
			Duration:     2 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 2 * time.Hour,
			Severity:     5,
		},
		{
			Reason:       ReasonRateLimitExceeded,
			Threshold:    3,
			Duration:     15 * time.Minute,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 15 * time.Minute,
			Severity:     2,
		},
		{
			Reason:       ReasonAuthenticationFail,
			Threshold:    3,
			Duration:     1 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 1 * time.Hour,
			Severity:     4,
		},
		{
			Reason:       ReasonQuotaExceeded,
			Threshold:    1,
			Duration:     24 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 24 * time.Hour,
			Severity:     3,
		},
		{
			Reason:       ReasonProviderError,
			Threshold:    5,
			Duration:     30 * time.Minute,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 30 * time.Minute,
			Severity:     3,
		},
		{
			Reason:       ReasonHealthCheckFailed,
			Threshold:    5,
			Duration:     1 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 1 * time.Hour,
			Severity:     4,
		},
		{
			Reason:       ReasonBounceRate,
			Threshold:    1,
			Duration:     4 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 4 * time.Hour,
			Severity:     5,
		},
		{
			Reason:       ReasonComplaintRate,
			Threshold:    1,
			Duration:     6 * time.Hour,
			Automatic:    true,
			Recoverable:  true,
			RecoveryTime: 6 * time.Hour,
			Severity:     5,
		},
		{
			Reason:       ReasonManualSuspension,
			Threshold:    1,
			Duration:     0,
			Automatic:    false,
			Recoverable:  true,
			RecoveryTime: 0,
			Severity:     10,
		},
	}

	sm.rulesMu.Lock()
	defer sm.rulesMu.Unlock()

	for _, rule := range rules {
		sm.suspensionRules[rule.Reason] = rule
	}
}

func (sm *SuspensionManager) CheckAndSuspend(acc *ManagedAccount, errorMsg string) bool {
	if acc.Suspension.IsSuspended {
		return false
	}

	reason := sm.determineReason(acc, errorMsg)
	if reason == "" {
		return false
	}

	shouldSuspend := sm.shouldSuspendAccount(acc, reason)
	if !shouldSuspend {
		return false
	}

	return sm.SuspendAccount(acc.Account.ID, reason, errorMsg, SuspensionTypeAutomatic)
}

func (sm *SuspensionManager) determineReason(acc *ManagedAccount, errorMsg string) SuspensionReason {
	errorLower := strings.ToLower(errorMsg)

	if strings.Contains(errorLower, "spam") || strings.Contains(errorLower, "blocked") {
		return ReasonSpamDetected
	}

	if strings.Contains(errorLower, "rate limit") || strings.Contains(errorLower, "too many") {
		return ReasonRateLimitExceeded
	}

	if strings.Contains(errorLower, "auth") || strings.Contains(errorLower, "credential") ||
		strings.Contains(errorLower, "unauthorized") {
		return ReasonAuthenticationFail
	}

	if strings.Contains(errorLower, "quota") || strings.Contains(errorLower, "limit exceeded") {
		return ReasonQuotaExceeded
	}

	if strings.Contains(errorLower, "bounce") {
		return ReasonBounceRate
	}

	if strings.Contains(errorLower, "complaint") || strings.Contains(errorLower, "abuse") {
		return ReasonComplaintRate
	}

	acc.Health.mu.RLock()
	consecutiveFails := acc.Health.ConsecutiveFails
	healthScore := acc.Health.Score
	acc.Health.mu.RUnlock()

	if consecutiveFails >= sm.config.ConsecutiveThreshold {
		return ReasonConsecutiveFailures
	}

	if healthScore < sm.config.HealthScoreThreshold {
		return ReasonHealthCheckFailed
	}

	if errorMsg != "" {
		return ReasonProviderError
	}

	return ""
}

func (sm *SuspensionManager) shouldSuspendAccount(acc *ManagedAccount, reason SuspensionReason) bool {
	sm.rulesMu.RLock()
	rule, exists := sm.suspensionRules[reason]
	sm.rulesMu.RUnlock()

	if !exists || !rule.Automatic {
		return false
	}

	switch reason {
	case ReasonConsecutiveFailures:
		acc.Health.mu.RLock()
		fails := acc.Health.ConsecutiveFails
		acc.Health.mu.RUnlock()
		return fails >= rule.Threshold

	case ReasonHealthCheckFailed:
		acc.Health.mu.RLock()
		score := acc.Health.Score
		acc.Health.mu.RUnlock()
		return score < sm.config.HealthScoreThreshold

	case ReasonSpamDetected, ReasonAuthenticationFail, ReasonQuotaExceeded,
		ReasonBounceRate, ReasonComplaintRate:
		return true

	case ReasonRateLimitExceeded, ReasonProviderError:
		recentSuspensions := sm.getRecentSuspensions(acc.Account.ID, reason, 1*time.Hour)
		return len(recentSuspensions) < rule.Threshold

	default:
		return false
	}
}

func (sm *SuspensionManager) SuspendAccount(accountID string, reason SuspensionReason, details string, suspType SuspensionType) bool {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		sm.log.Error("failed to get account for suspension", // ← FIXED
			logger.String("account_id", accountID),
			logger.String("error", err.Error()),
		)
		return false
	}

	acc.Suspension.mu.Lock()
	if acc.Suspension.IsSuspended {
		acc.Suspension.mu.Unlock()
		return false
	}

	sm.rulesMu.RLock()
	rule, exists := sm.suspensionRules[reason]
	sm.rulesMu.RUnlock()

	duration := sm.config.DefaultSuspensionTime
	if exists && rule.Duration > 0 {
		duration = rule.Duration
	}

	severity := 3
	if exists {
		severity = rule.Severity
	}

	now := time.Now()
	resumeAt := now.Add(duration)

	if suspType == SuspensionTypePermanent {
		resumeAt = now.Add(100 * 365 * 24 * time.Hour)
	}

	acc.Suspension.IsSuspended = true
	acc.Suspension.Reason = string(reason)
	acc.Suspension.SuspendedAt = now
	acc.Suspension.ResumeAt = resumeAt

	acc.Suspension.mu.Unlock()

	record := &SuspensionRecord{
		AccountID:    accountID,
		Email:        acc.Account.Email,
		Reason:       reason,
		Type:         suspType,
		SuspendedAt:  now,
		ResumeAt:     resumeAt,
		Details:      details,
		ErrorMessage: details,
		Severity:     severity,
		Metadata:     make(map[string]interface{}),
	}

	sm.recordSuspension(record)

	sm.log.Warn("account suspended", // ← FIXED
		logger.String("account_id", accountID),
		logger.String("email", acc.Account.Email),
		logger.String("reason", string(reason)),
		logger.String("type", string(suspType)),
		logger.Duration("duration", duration),
		logger.Time("resume_at", resumeAt),
	)

	if sm.config.EnableNotifications {
		sm.sendSuspensionNotification(acc, record)
	}

	return true
}

func (sm *SuspensionManager) ResumeAccount(accountID string) error {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return err
	}

	acc.Suspension.mu.Lock()
	defer acc.Suspension.mu.Unlock()

	if !acc.Suspension.IsSuspended {
		return ErrNotSuspended
	}

	acc.Suspension.IsSuspended = false
	acc.Suspension.Reason = ""
	now := time.Now()

	sm.historyMu.Lock()
	if history, exists := sm.suspensionHistory[accountID]; exists {
		if len(history) > 0 {
			lastRecord := history[len(history)-1]
			if lastRecord.ResumedAt == nil {
				lastRecord.ResumedAt = &now
			}
		}
	}
	sm.historyMu.Unlock()

	acc.Health.mu.Lock()
	acc.Health.ConsecutiveFails = 0
	acc.Health.ConsecutiveSuccess = 0
	acc.Health.mu.Unlock()

	sm.log.Info("account resumed", // ← FIXED
		logger.String("account_id", accountID),
		logger.String("email", acc.Account.Email),
	)

	return nil
}

func (sm *SuspensionManager) AutoResumeCheck(accountID string) bool {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return false
	}

	acc.Suspension.mu.RLock()
	isSuspended := acc.Suspension.IsSuspended
	resumeAt := acc.Suspension.ResumeAt
	acc.Suspension.mu.RUnlock()

	if !isSuspended {
		return false
	}

	if time.Now().After(resumeAt) {
		err := sm.ResumeAccount(accountID)
		if err == nil {
			sm.log.Info("account auto-resumed", // ← FIXED
				logger.String("account_id", accountID),
				logger.String("email", acc.Account.Email),
			)
			return true
		}
	}

	return false
}

func (sm *SuspensionManager) autoRecoveryLoop() {
	ticker := time.NewTicker(sm.config.RecoveryCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		sm.checkAllForAutoResume()
	}
}

func (sm *SuspensionManager) checkAllForAutoResume() {
	accounts := sm.manager.ListAccounts(nil)

	for _, acc := range accounts {
		acc.Suspension.mu.RLock()
		isSuspended := acc.Suspension.IsSuspended
		acc.Suspension.mu.RUnlock()

		if isSuspended {
			sm.AutoResumeCheck(acc.Account.ID)
		}
	}
}

func (sm *SuspensionManager) recordSuspension(record *SuspensionRecord) {
	sm.historyMu.Lock()
	defer sm.historyMu.Unlock()

	history, exists := sm.suspensionHistory[record.AccountID]
	if !exists {
		history = make([]*SuspensionRecord, 0)
	}

	history = append(history, record)

	if len(history) > 100 {
		history = history[len(history)-100:]
	}

	sm.suspensionHistory[record.AccountID] = history
}

func (sm *SuspensionManager) GetSuspensionHistory(accountID string) []*SuspensionRecord {
	sm.historyMu.RLock()
	defer sm.historyMu.RUnlock()

	history, exists := sm.suspensionHistory[accountID]
	if !exists {
		return []*SuspensionRecord{}
	}

	result := make([]*SuspensionRecord, len(history))
	copy(result, history)

	return result
}

func (sm *SuspensionManager) getRecentSuspensions(accountID string, reason SuspensionReason, duration time.Duration) []*SuspensionRecord {
	sm.historyMu.RLock()
	defer sm.historyMu.RUnlock()

	history, exists := sm.suspensionHistory[accountID]
	if !exists {
		return []*SuspensionRecord{}
	}

	cutoff := time.Now().Add(-duration)
	recent := make([]*SuspensionRecord, 0)

	for _, record := range history {
		if record.Reason == reason && record.SuspendedAt.After(cutoff) {
			recent = append(recent, record)
		}
	}

	return recent
}

func (sm *SuspensionManager) GetSuspendedAccounts() []*ManagedAccount {
	accounts := sm.manager.ListAccounts(nil)
	suspended := make([]*ManagedAccount, 0)

	for _, acc := range accounts {
		acc.Suspension.mu.RLock()
		isSuspended := acc.Suspension.IsSuspended
		acc.Suspension.mu.RUnlock()

		if isSuspended {
			suspended = append(suspended, acc)
		}
	}

	return suspended
}

func (sm *SuspensionManager) GetSuspensionStats() map[string]interface{} {
	suspendedAccounts := sm.GetSuspendedAccounts()

	reasonCounts := make(map[SuspensionReason]int)
	totalSuspensions := 0

	sm.historyMu.RLock()
	for _, history := range sm.suspensionHistory {
		totalSuspensions += len(history)
		for _, record := range history {
			reasonCounts[record.Reason]++
		}
	}
	sm.historyMu.RUnlock()

	return map[string]interface{}{
		"currently_suspended": len(suspendedAccounts),
		"total_suspensions":   totalSuspensions,
		"by_reason":           reasonCounts,
	}
}

func (sm *SuspensionManager) IsSuspended(accountID string) bool {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return false
	}

	acc.Suspension.mu.RLock()
	defer acc.Suspension.mu.RUnlock()

	return acc.Suspension.IsSuspended
}

func (sm *SuspensionManager) GetSuspensionReason(accountID string) string {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return ""
	}

	acc.Suspension.mu.RLock()
	defer acc.Suspension.mu.RUnlock()

	return acc.Suspension.Reason
}

func (sm *SuspensionManager) GetResumeTime(accountID string) time.Time {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return time.Time{}
	}

	acc.Suspension.mu.RLock()
	defer acc.Suspension.mu.RUnlock()

	return acc.Suspension.ResumeAt
}

func (sm *SuspensionManager) ExtendSuspension(accountID string, duration time.Duration) error {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return err
	}

	acc.Suspension.mu.Lock()
	defer acc.Suspension.mu.Unlock()

	if !acc.Suspension.IsSuspended {
		return ErrNotSuspended
	}

	acc.Suspension.ResumeAt = acc.Suspension.ResumeAt.Add(duration)

	sm.log.Info("suspension extended", // ← FIXED
		logger.String("account_id", accountID),
		logger.Duration("extension", duration),
		logger.Time("new_resume_at", acc.Suspension.ResumeAt),
	)

	return nil
}

func (sm *SuspensionManager) UpdateRule(reason SuspensionReason, rule *SuspensionRule) {
	sm.rulesMu.Lock()
	defer sm.rulesMu.Unlock()

	rule.Reason = reason
	sm.suspensionRules[reason] = rule

	sm.log.Info("suspension rule updated", // ← FIXED
		logger.String("reason", string(reason)),
		logger.Int("threshold", rule.Threshold),
		logger.Duration("duration", rule.Duration),
	)
}

func (sm *SuspensionManager) GetRule(reason SuspensionReason) *SuspensionRule {
	sm.rulesMu.RLock()
	defer sm.rulesMu.RUnlock()

	return sm.suspensionRules[reason]
}

func (sm *SuspensionManager) sendSuspensionNotification(acc *ManagedAccount, record *SuspensionRecord) {
	// Notification system not implemented yet
	// TODO: Implement notification system

	message := fmt.Sprintf(
		"⚠️ Account Suspended\n\n"+
			"Email: %s\n"+
			"Reason: %s\n"+
			"Type: %s\n"+
			"Suspended At: %s\n"+
			"Resume At: %s\n"+
			"Details: %s",
		record.Email,
		record.Reason,
		record.Type,
		record.SuspendedAt.Format(time.RFC3339),
		record.ResumeAt.Format(time.RFC3339),
		record.Details,
	)

	sm.log.Debug("suspension notification prepared", // ← FIXED
		logger.String("account_id", acc.Account.ID),
	)
	_ = message
}

func (sm *SuspensionManager) ClearHistory(accountID string) {
	sm.historyMu.Lock()
	defer sm.historyMu.Unlock()

	delete(sm.suspensionHistory, accountID)

	sm.log.Info("suspension history cleared", // ← FIXED
		logger.String("account_id", accountID),
	)
}

func (sm *SuspensionManager) GetSuspensionCount(accountID string) int {
	sm.historyMu.RLock()
	defer sm.historyMu.RUnlock()

	history, exists := sm.suspensionHistory[accountID]
	if !exists {
		return 0
	}

	return len(history)
}

func (sm *SuspensionManager) HasRecentSuspension(accountID string, duration time.Duration) bool {
	recent := sm.getRecentSuspensions(accountID, "", duration)
	return len(recent) > 0
}

func (sm *SuspensionManager) UpdateConfig(config *SuspensionConfig) {
	if config == nil {
		return
	}

	sm.config = config

	sm.log.Info("suspension config updated", // ← FIXED
		logger.Int("failure_threshold", config.FailureThreshold),
		logger.Bool("auto_recovery", config.AutoRecoveryEnabled),
	)
}

func (sm *SuspensionManager) ManualSuspend(accountID string, reason string, duration time.Duration) error {
	details := fmt.Sprintf("Manual suspension: %s", reason)

	suspType := SuspensionTypeTemporary
	if duration == 0 {
		suspType = SuspensionTypePermanent
	}

	success := sm.SuspendAccount(accountID, ReasonManualSuspension, details, suspType)
	if !success {
		return ErrAlreadySuspended
	}

	if duration > 0 {
		acc, err := sm.manager.GetAccount(accountID)
		if err != nil {
			return err
		}

		acc.Suspension.mu.Lock()
		acc.Suspension.ResumeAt = time.Now().Add(duration)
		acc.Suspension.mu.Unlock()
	}

	return nil
}

func (sm *SuspensionManager) GetTimeUntilResume(accountID string) time.Duration {
	acc, err := sm.manager.GetAccount(accountID)
	if err != nil {
		return 0
	}

	acc.Suspension.mu.RLock()
	defer acc.Suspension.mu.RUnlock()

	if !acc.Suspension.IsSuspended {
		return 0
	}

	remaining := time.Until(acc.Suspension.ResumeAt)
	if remaining < 0 {
		return 0
	}

	return remaining
}

func (ss *SuspensionState) GetReason() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.Reason
}

func (ss *SuspensionState) GetSuspendedAt() time.Time {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.SuspendedAt
}

func (ss *SuspensionState) GetResumeAt() time.Time {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.ResumeAt
}
