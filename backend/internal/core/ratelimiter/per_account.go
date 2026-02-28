package ratelimiter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type PerAccountLimiter struct {
	accounts map[string]*accountLimit
	mu       sync.RWMutex
	config   *PerAccountConfig
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type PerAccountConfig struct {
	DefaultDailyLimit     int
	DefaultRotationLimit  int
	DefaultCooldownPeriod time.Duration
	EnableDailyReset      bool
	DailyResetTime        string
	EnableCooldown        bool
	MaxConcurrentSends    int
	TrackingWindow        time.Duration
	AutoCleanup           bool
	CleanupInterval       time.Duration
	InactiveThreshold     time.Duration
}

type accountLimit struct {
	accountID         string
	dailyLimit        int
	rotationLimit     int
	dailySent         int
	rotationSent      int
	cooldownUntil     time.Time
	lastSendTime      time.Time
	lastDailyReset    time.Time
	lastRotationReset time.Time
	concurrentSends   int
	totalSent         int64
	totalBlocked      int64
	isActive          bool
	mu                sync.Mutex
}

type AccountLimitInfo struct {
	AccountID          string
	DailyLimit         int
	DailyRemaining     int
	DailySent          int
	RotationLimit      int
	RotationRemaining  int
	RotationSent       int
	CooldownUntil      time.Time
	IsInCooldown       bool
	LastSendTime       time.Time
	LastDailyReset     time.Time
	LastRotationReset  time.Time
	ConcurrentSends    int
	TotalSent          int64
	TotalBlocked       int64
	IsActive           bool
	CanSend            bool
	BlockReason        string
}

func NewPerAccountLimiter(config *PerAccountConfig) *PerAccountLimiter {
	if config == nil {
		config = DefaultPerAccountConfig()
	}

	pal := &PerAccountLimiter{
		accounts: make(map[string]*accountLimit),
		config:   config,
		stopCh:   make(chan struct{}),
	}

	if config.EnableDailyReset {
		pal.startDailyResetScheduler()
	}

	if config.AutoCleanup {
		pal.startCleanup()
	}

	return pal
}

func DefaultPerAccountConfig() *PerAccountConfig {
	return &PerAccountConfig{
		DefaultDailyLimit:     500,
		DefaultRotationLimit:  100,
		DefaultCooldownPeriod: 5 * time.Minute,
		EnableDailyReset:      true,
		DailyResetTime:        "00:00",
		EnableCooldown:        true,
		MaxConcurrentSends:    5,
		TrackingWindow:        24 * time.Hour,
		AutoCleanup:           true,
		CleanupInterval:       1 * time.Hour,
		InactiveThreshold:     24 * time.Hour,
	}
}

func (pal *PerAccountLimiter) Allow(accountID string) (bool, error) {
	return pal.AllowN(accountID, 1)
}

func (pal *PerAccountLimiter) AllowN(accountID string, n int) (bool, error) {
	if n <= 0 {
		return false, fmt.Errorf("n must be positive")
	}

	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	now := time.Now()

	if pal.config.EnableDailyReset {
		if now.Sub(acct.lastDailyReset) >= 24*time.Hour {
			pal.resetDaily(acct)
		}
	}

	if pal.config.EnableCooldown && now.Before(acct.cooldownUntil) {
		acct.totalBlocked++
		return false, nil
	}

	if acct.dailySent+n > acct.dailyLimit {
		acct.totalBlocked++
		return false, nil
	}

	if acct.rotationSent+n > acct.rotationLimit {
		acct.totalBlocked++
		return false, nil
	}

	if acct.concurrentSends >= pal.config.MaxConcurrentSends {
		acct.totalBlocked++
		return false, nil
	}

	acct.dailySent += n
	acct.rotationSent += n
	acct.totalSent += int64(n)
	acct.lastSendTime = now
	acct.isActive = true

	return true, nil
}

func (pal *PerAccountLimiter) Wait(ctx context.Context, accountID string) error {
	return pal.WaitN(ctx, accountID, 1)
}

func (pal *PerAccountLimiter) WaitN(ctx context.Context, accountID string, n int) error {
	if n <= 0 {
		return fmt.Errorf("n must be positive")
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		allowed, err := pal.AllowN(accountID, n)
		if err != nil {
			return err
		}

		if allowed {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

func (pal *PerAccountLimiter) Reserve(accountID string) (*Reservation, error) {
	return pal.ReserveN(accountID, 1)
}

func (pal *PerAccountLimiter) ReserveN(accountID string, n int) (*Reservation, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be positive")
	}

	allowed, err := pal.AllowN(accountID, n)
	if err != nil {
		return nil, err
	}

	reservation := &Reservation{
		OK:        allowed,
		tokens:    n,
		key:       accountID,
		TimeToAct: time.Now(),
		delay:     0, // For simplicity, not calculating delay here, but it could be implemented based on when the next token will be available
	}

	return reservation, nil
}


func (pal *PerAccountLimiter) IncrementConcurrent(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	if acct.concurrentSends >= pal.config.MaxConcurrentSends {
		return fmt.Errorf("max concurrent sends reached")
	}

	acct.concurrentSends++
	return nil
}

func (pal *PerAccountLimiter) DecrementConcurrent(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	if acct.concurrentSends > 0 {
		acct.concurrentSends--
	}

	return nil
}

func (pal *PerAccountLimiter) ResetDaily(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	pal.resetDaily(acct)
	return nil
}

func (pal *PerAccountLimiter) ResetRotation(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.rotationSent = 0
	acct.lastRotationReset = time.Now()
	return nil
}

func (pal *PerAccountLimiter) SetCooldown(accountID string, duration time.Duration) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.cooldownUntil = time.Now().Add(duration)
	return nil
}

func (pal *PerAccountLimiter) ClearCooldown(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.cooldownUntil = time.Time{}
	return nil
}

func (pal *PerAccountLimiter) SetDailyLimit(accountID string, limit int) error {
	if limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.dailyLimit = limit
	return nil
}

func (pal *PerAccountLimiter) SetRotationLimit(accountID string, limit int) error {
	if limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.rotationLimit = limit
	return nil
}

func (pal *PerAccountLimiter) GetAccountInfo(accountID string) (*AccountLimitInfo, error) {
	pal.mu.RLock()
	acct, exists := pal.accounts[accountID]
	pal.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("account not found")
	}

	acct.mu.Lock()
	defer acct.mu.Unlock()

	now := time.Now()
	isInCooldown := now.Before(acct.cooldownUntil)

	dailyRemaining := acct.dailyLimit - acct.dailySent
	if dailyRemaining < 0 {
		dailyRemaining = 0
	}

	rotationRemaining := acct.rotationLimit - acct.rotationSent
	if rotationRemaining < 0 {
		rotationRemaining = 0
	}

	canSend := true
	blockReason := ""

	if isInCooldown {
		canSend = false
		blockReason = "account in cooldown"
	} else if dailyRemaining == 0 {
		canSend = false
		blockReason = "daily limit reached"
	} else if rotationRemaining == 0 {
		canSend = false
		blockReason = "rotation limit reached"
	} else if acct.concurrentSends >= pal.config.MaxConcurrentSends {
		canSend = false
		blockReason = "max concurrent sends reached"
	}

	return &AccountLimitInfo{
		AccountID:          accountID,
		DailyLimit:         acct.dailyLimit,
		DailyRemaining:     dailyRemaining,
		DailySent:          acct.dailySent,
		RotationLimit:      acct.rotationLimit,
		RotationRemaining:  rotationRemaining,
		RotationSent:       acct.rotationSent,
		CooldownUntil:      acct.cooldownUntil,
		IsInCooldown:       isInCooldown,
		LastSendTime:       acct.lastSendTime,
		LastDailyReset:     acct.lastDailyReset,
		LastRotationReset:  acct.lastRotationReset,
		ConcurrentSends:    acct.concurrentSends,
		TotalSent:          acct.totalSent,
		TotalBlocked:       acct.totalBlocked,
		IsActive:           acct.isActive,
		CanSend:            canSend,
		BlockReason:        blockReason,
	}, nil
}

func (pal *PerAccountLimiter) GetAllAccounts() ([]*AccountLimitInfo, error) {
	pal.mu.RLock()
	accountIDs := make([]string, 0, len(pal.accounts))
	for id := range pal.accounts {
		accountIDs = append(accountIDs, id)
	}
	pal.mu.RUnlock()

	infos := make([]*AccountLimitInfo, 0, len(accountIDs))
	for _, id := range accountIDs {
		info, err := pal.GetAccountInfo(id)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}

	return infos, nil
}

func (pal *PerAccountLimiter) RemoveAccount(accountID string) error {
	pal.mu.Lock()
	defer pal.mu.Unlock()

	delete(pal.accounts, accountID)
	return nil
}

func (pal *PerAccountLimiter) Reset(accountID string) error {
	acct := pal.getOrCreateAccount(accountID)
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.dailySent = 0
	acct.rotationSent = 0
	acct.cooldownUntil = time.Time{}
	acct.lastDailyReset = time.Now()
	acct.lastRotationReset = time.Now()
	acct.concurrentSends = 0

	return nil
}

func (pal *PerAccountLimiter) Close() error {
	close(pal.stopCh)
	pal.wg.Wait()
	return nil
}

func (pal *PerAccountLimiter) getOrCreateAccount(accountID string) *accountLimit {
	pal.mu.RLock()
	acct, exists := pal.accounts[accountID]
	pal.mu.RUnlock()

	if exists {
		return acct
	}

	pal.mu.Lock()
	defer pal.mu.Unlock()

	acct, exists = pal.accounts[accountID]
	if exists {
		return acct
	}

	now := time.Now()
	acct = &accountLimit{
		accountID:         accountID,
		dailyLimit:        pal.config.DefaultDailyLimit,
		rotationLimit:     pal.config.DefaultRotationLimit,
		lastDailyReset:    now,
		lastRotationReset: now,
		isActive:          true,
	}

	pal.accounts[accountID] = acct
	return acct
}

func (pal *PerAccountLimiter) resetDaily(acct *accountLimit) {
	acct.dailySent = 0
	acct.lastDailyReset = time.Now()
}

func (pal *PerAccountLimiter) startDailyResetScheduler() {
	pal.wg.Add(1)
	go func() {
		defer pal.wg.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-pal.stopCh:
				return
			case <-ticker.C:
				pal.performDailyReset()
			}
		}
	}()
}

func (pal *PerAccountLimiter) performDailyReset() {
	pal.mu.RLock()
	accountIDs := make([]string, 0, len(pal.accounts))
	for id := range pal.accounts {
		accountIDs = append(accountIDs, id)
	}
	pal.mu.RUnlock()

	now := time.Now()
	for _, id := range accountIDs {
		acct := pal.getOrCreateAccount(id)
		acct.mu.Lock()

		if now.Sub(acct.lastDailyReset) >= 24*time.Hour {
			pal.resetDaily(acct)
		}

		acct.mu.Unlock()
	}
}

func (pal *PerAccountLimiter) startCleanup() {
	pal.wg.Add(1)
	go func() {
		defer pal.wg.Done()
		ticker := time.NewTicker(pal.config.CleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-pal.stopCh:
				return
			case <-ticker.C:
				pal.performCleanup()
			}
		}
	}()
}

func (pal *PerAccountLimiter) performCleanup() {
	pal.mu.Lock()
	defer pal.mu.Unlock()

	now := time.Now()
	for id, acct := range pal.accounts {
		acct.mu.Lock()
		inactive := now.Sub(acct.lastSendTime) > pal.config.InactiveThreshold
		acct.mu.Unlock()

		if inactive {
			delete(pal.accounts, id)
		}
	}
}

func (pal *PerAccountLimiter) GetConfig() *PerAccountConfig {
	return pal.config
}

func (pal *PerAccountLimiter) UpdateConfig(config *PerAccountConfig) {
	pal.config = config
}

func (pal *PerAccountLimiter) CanSend(accountID string) (bool, string) {
	info, err := pal.GetAccountInfo(accountID)
	if err != nil {
		return false, "account not found"
	}

	return info.CanSend, info.BlockReason
}

func (pal *PerAccountLimiter) GetLimit(key string) (*LimitInfo, error) {
	info, err := pal.GetAccountInfo(key)
	if err != nil {
		return nil, err
	}

	return &LimitInfo{
		Key:       key,
		Rate:      info.DailyLimit,
		Per:       24 * time.Hour,
		Burst:     info.RotationLimit,
		Available: info.DailyRemaining,
		ResetAt:   info.LastDailyReset.Add(24 * time.Hour),
	}, nil
}

func (pal *PerAccountLimiter) SetLimit(key string, limit RateLimit) error {
	return pal.SetDailyLimit(key, limit.Rate)
}
func (pal *PerAccountLimiter) GetStats(key string) (*Stats, error) {
	info, err := pal.GetAccountInfo(key)
	if err != nil {
		return nil, err
	}

	return &Stats{
		Key:             key,
		TotalRequests:   info.TotalSent + info.TotalBlocked,
		AllowedRequests: info.TotalSent,
		DeniedRequests:  info.TotalBlocked,
		LastRequestAt:   info.LastSendTime, 
		WindowStart:     info.LastDailyReset,
		WindowEnd:       info.LastDailyReset.Add(24 * time.Hour),
		CurrentRate:     0,  
		AverageRate:     0,  
		PeakRate:        0,
	}, nil
}

