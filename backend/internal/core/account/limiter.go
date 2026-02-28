package account

import (
    "errors"
    "sync"
    "time"

    "email-campaign-system/pkg/logger"
)

var (
    ErrDailyLimitReached    = errors.New("daily limit reached")
    ErrRotationLimitReached = errors.New("rotation limit reached")
    ErrHourlyLimitReached   = errors.New("hourly limit reached")
    ErrNoLimitSet           = errors.New("no limit configured")
)

type AccountLimiter struct {
    manager       *AccountManager
    log           logger.Logger
    config        *LimiterConfig
    resetScheduler *resetScheduler
    mu            sync.RWMutex
}

type LimiterConfig struct {
    EnableDailyLimit    bool
    EnableRotationLimit bool
    EnableHourlyLimit   bool
    DefaultDailyLimit   int
    DefaultRotationLimit int
    DefaultHourlyLimit  int
    AutoResetEnabled    bool
    ResetTime           string
    WarningThreshold    float64
}

type LimitStatus struct {
    AccountID        string
    DailyLimit       int
    DailySent        int
    DailyRemaining   int
    DailyPercentage  float64
    RotationLimit    int
    RotationSent     int
    RotationRemaining int
    RotationPercentage float64
    HourlyLimit      int
    HourlySent       int
    HourlyRemaining  int
    LastReset        time.Time
    NextReset        time.Time
    IsLimited        bool
}

type resetScheduler struct {
    limiter   *AccountLimiter
    stopChan  chan struct{}
    running   bool
    mu        sync.RWMutex
}

func NewAccountLimiter(manager *AccountManager, log logger.Logger) *AccountLimiter {
    limiter := &AccountLimiter{
        manager: manager,
        log:     log,
        config:  DefaultLimiterConfig(),
    }

    limiter.resetScheduler = &resetScheduler{
        limiter:  limiter,
        stopChan: make(chan struct{}),
    }

    if limiter.config.AutoResetEnabled {
        go limiter.resetScheduler.start()
    }

    return limiter
}

func DefaultLimiterConfig() *LimiterConfig {
    return &LimiterConfig{
        EnableDailyLimit:     true,
        EnableRotationLimit:  true,
        EnableHourlyLimit:    false,
        DefaultDailyLimit:    500,
        DefaultRotationLimit: 100,
        DefaultHourlyLimit:   50,
        AutoResetEnabled:     true,
        ResetTime:            "00:00",
        WarningThreshold:     0.8,
    }
}

func (al *AccountLimiter) CheckLimit(accountID string) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.RLock()
    defer acc.Limits.mu.RUnlock()

    if al.config.EnableDailyLimit && acc.Limits.DailyLimit > 0 {
        if acc.Limits.DailySent >= acc.Limits.DailyLimit {
            return ErrDailyLimitReached
        }
    }

    if al.config.EnableRotationLimit && acc.Limits.RotationLimit > 0 {
        if acc.Limits.RotationSent >= acc.Limits.RotationLimit {
            return ErrRotationLimitReached
        }
    }

    return nil
}

func (al *AccountLimiter) IncrementUsage(accountID string) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.Lock()
    defer acc.Limits.mu.Unlock()

    if al.config.EnableDailyLimit {
        acc.Limits.DailySent++
    }

    if al.config.EnableRotationLimit {
        acc.Limits.RotationSent++
    }

    acc.mu.Lock()
    acc.Account.Stats.TotalSent++
    acc.mu.Unlock()

    al.checkWarningThreshold(acc)

    return nil
}

func (al *AccountLimiter) checkWarningThreshold(acc *ManagedAccount) {
    if acc.Limits.DailyLimit > 0 {
        percentage := float64(acc.Limits.DailySent) / float64(acc.Limits.DailyLimit)
        if percentage >= al.config.WarningThreshold && percentage < 1.0 {
            al.log.Warn("account approaching daily limit",
                logger.String("account_id", acc.Account.ID),
                logger.String("email", acc.Account.Email),
                logger.Int("sent", acc.Limits.DailySent),
                logger.Int("limit", acc.Limits.DailyLimit),
                logger.Float64("percentage", percentage*100),
            )
        }
    }
}

func (al *AccountLimiter) ResetDaily(accountID string) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.Lock()
    defer acc.Limits.mu.Unlock()

    acc.Limits.DailySent = 0
    acc.Limits.LastReset = time.Now()

    al.log.Info("daily limit reset",
        logger.String("account_id", accountID),
        logger.String("email", acc.Account.Email),
    )

    return nil
}

func (al *AccountLimiter) ResetRotation(accountID string) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.Lock()
    defer acc.Limits.mu.Unlock()

    acc.Limits.RotationSent = 0

    al.log.Debug("rotation limit reset",
        logger.String("account_id", accountID),
        logger.String("email", acc.Account.Email),
    )

    return nil
}

func (al *AccountLimiter) ResetAllDaily() {
    accounts := al.manager.ListAccounts(nil)

    for _, acc := range accounts {
        al.ResetDaily(acc.Account.ID)
    }

    al.log.Info("all accounts daily limits reset",
        logger.Int("count", len(accounts)),
    )
}

func (al *AccountLimiter) GetLimitStatus(accountID string) (*LimitStatus, error) {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return nil, err
    }

    acc.Limits.mu.RLock()
    defer acc.Limits.mu.RUnlock()

    status := &LimitStatus{
        AccountID:     accountID,
        DailyLimit:    acc.Limits.DailyLimit,
        DailySent:     acc.Limits.DailySent,
        RotationLimit: acc.Limits.RotationLimit,
        RotationSent:  acc.Limits.RotationSent,
        LastReset:     acc.Limits.LastReset,
    }

    if status.DailyLimit > 0 {
        status.DailyRemaining = status.DailyLimit - status.DailySent
        if status.DailyRemaining < 0 {
            status.DailyRemaining = 0
        }
        status.DailyPercentage = float64(status.DailySent) / float64(status.DailyLimit) * 100.0
        status.IsLimited = status.DailySent >= status.DailyLimit
    }

    if status.RotationLimit > 0 {
        status.RotationRemaining = status.RotationLimit - status.RotationSent
        if status.RotationRemaining < 0 {
            status.RotationRemaining = 0
        }
        status.RotationPercentage = float64(status.RotationSent) / float64(status.RotationLimit) * 100.0
        if status.RotationSent >= status.RotationLimit {
            status.IsLimited = true
        }
    }

    status.NextReset = al.calculateNextReset()

    return status, nil
}

func (al *AccountLimiter) calculateNextReset() time.Time {
    now := time.Now()
    next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
    return next
}

func (al *AccountLimiter) SetDailyLimit(accountID string, limit int) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.Lock()
    defer acc.Limits.mu.Unlock()

    acc.Limits.DailyLimit = limit

    al.log.Info("daily limit updated",
        logger.String("account_id", accountID),
        logger.Int("limit", limit),
    )

    return nil
}

func (al *AccountLimiter) SetRotationLimit(accountID string, limit int) error {
    acc, err := al.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Limits.mu.Lock()
    defer acc.Limits.mu.Unlock()

    acc.Limits.RotationLimit = limit

    al.log.Info("rotation limit updated",
        logger.String("account_id", accountID),
        logger.Int("limit", limit),
    )

    return nil
}

func (al *AccountLimiter) GetUsageStats() map[string]interface{} {
    accounts := al.manager.ListAccounts(nil)

    totalDaily := 0
    totalRotation := 0
    limitedAccounts := 0

    for _, acc := range accounts {
        acc.Limits.mu.RLock()
        totalDaily += acc.Limits.DailySent
        totalRotation += acc.Limits.RotationSent
        
        if acc.Limits.DailyLimit > 0 && acc.Limits.DailySent >= acc.Limits.DailyLimit {
            limitedAccounts++
        }
        acc.Limits.mu.RUnlock()
    }

    return map[string]interface{}{
        "total_accounts":    len(accounts),
        "total_daily_sent":  totalDaily,
        "total_rotation_sent": totalRotation,
        "limited_accounts":  limitedAccounts,
    }
}

func (al *AccountLimiter) IsLimitReached(accountID string) bool {
    status, err := al.GetLimitStatus(accountID)
    if err != nil {
        return false
    }

    return status.IsLimited
}

func (al *AccountLimiter) GetRemainingCapacity(accountID string) int {
    status, err := al.GetLimitStatus(accountID)
    if err != nil {
        return 0
    }

    if status.DailyLimit == 0 {
        return 0
    }

    remaining := status.DailyRemaining
    if status.RotationLimit > 0 && status.RotationRemaining < remaining {
        remaining = status.RotationRemaining
    }

    return remaining
}

func (al *AccountLimiter) UpdateConfig(config *LimiterConfig) {
    if config == nil {
        return
    }

    al.mu.Lock()
    al.config = config
    al.mu.Unlock()

    al.log.Info("limiter config updated",
        logger.Int("daily_limit", config.DefaultDailyLimit),
        logger.Int("rotation_limit", config.DefaultRotationLimit),
    )
}

func (rs *resetScheduler) start() {
    rs.mu.Lock()
    if rs.running {
        rs.mu.Unlock()
        return
    }
    rs.running = true
    rs.mu.Unlock()

    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            rs.checkAndReset()
        case <-rs.stopChan:
            return
        }
    }
}

func (rs *resetScheduler) checkAndReset() {
    now := time.Now()
    
    if now.Hour() == 0 && now.Minute() < 5 {
        rs.limiter.ResetAllDaily()
    }
}

func (rs *resetScheduler) stop() {
    rs.mu.Lock()
    if !rs.running {
        rs.mu.Unlock()
        return
    }
    rs.running = false
    rs.mu.Unlock()

    close(rs.stopChan)
}

func (al *AccountLimiter) Stop() {
    if al.resetScheduler != nil {
        al.resetScheduler.stop()
    }
}

func (al *AccountLimiter) GetAccountsNearLimit(threshold float64) []*ManagedAccount {
    accounts := al.manager.ListAccounts(nil)
    nearLimit := make([]*ManagedAccount, 0)

    for _, acc := range accounts {
        acc.Limits.mu.RLock()
        dailyLimit := acc.Limits.DailyLimit
        dailySent := acc.Limits.DailySent
        acc.Limits.mu.RUnlock()

        if dailyLimit > 0 {
            percentage := float64(dailySent) / float64(dailyLimit)
            if percentage >= threshold && percentage < 1.0 {
                nearLimit = append(nearLimit, acc)
            }
        }
    }

    return nearLimit
}

func (al *AccountLimiter) GetLimitedAccounts() []*ManagedAccount {
    accounts := al.manager.ListAccounts(nil)
    limited := make([]*ManagedAccount, 0)

    for _, acc := range accounts {
        if al.IsLimitReached(acc.Account.ID) {
            limited = append(limited, acc)
        }
    }

    return limited
}

func (al *AccountLimiter) SetBulkDailyLimit(limit int) {
    accounts := al.manager.ListAccounts(nil)

    for _, acc := range accounts {
        al.SetDailyLimit(acc.Account.ID, limit)
    }

    al.log.Info("bulk daily limit set",
        logger.Int("limit", limit),
        logger.Int("accounts", len(accounts)),
    )
}

func (al *AccountLimiter) SetBulkRotationLimit(limit int) {
    accounts := al.manager.ListAccounts(nil)

    for _, acc := range accounts {
        al.SetRotationLimit(acc.Account.ID, limit)
    }

    al.log.Info("bulk rotation limit set",
        logger.Int("limit", limit),
        logger.Int("accounts", len(accounts)),
    )
}

func (al *AccountLimiter) GetAverageDailyUsage() float64 {
    accounts := al.manager.ListAccounts(nil)
    if len(accounts) == 0 {
        return 0.0
    }

    totalSent := 0
    for _, acc := range accounts {
        acc.Limits.mu.RLock()
        totalSent += acc.Limits.DailySent
        acc.Limits.mu.RUnlock()
    }

    return float64(totalSent) / float64(len(accounts))
}

func (al *AccountLimiter) GetTotalCapacity() int {
    accounts := al.manager.ListAccounts(nil)
    totalCapacity := 0

    for _, acc := range accounts {
        capacity := al.GetRemainingCapacity(acc.Account.ID)
        totalCapacity += capacity
    }

    return totalCapacity
}

func (al *AccountLimiter) PredictLimitReach(accountID string, emailsToSend int) (bool, int) {
    status, err := al.GetLimitStatus(accountID)
    if err != nil {
        return false, 0
    }

    if status.DailyLimit == 0 {
        return false, emailsToSend
    }

    remaining := status.DailyRemaining
    if status.RotationLimit > 0 && status.RotationRemaining < remaining {
        remaining = status.RotationRemaining
    }

    if emailsToSend <= remaining {
        return false, emailsToSend
    }

    return true, remaining
}

func (al *AccountLimiter) GetConfig() *LimiterConfig {
    al.mu.RLock()
    defer al.mu.RUnlock()

    return al.config
}

func NewAccountHealth() *AccountHealth {
    return &AccountHealth{
        Score:   100.0,
        Status:  HealthStatusHealthy,
        Details: make(map[string]interface{}),
    }
}

func NewAccountLimits(dailyLimit, rotationLimit int) *AccountLimits {
    return &AccountLimits{
        DailyLimit:    dailyLimit,
        RotationLimit: rotationLimit,
        DailySent:     0,
        RotationSent:  0,
        LastReset:     time.Now(),
    }
}

func NewSuspensionState() *SuspensionState {
    return &SuspensionState{
        IsSuspended: false,
    }
}
