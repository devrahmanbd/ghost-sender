package account

import (
    "context"
    "sync"
    "time"

    "email-campaign-system/pkg/logger"
)

type HealthStatus string

const (
    HealthStatusHealthy   HealthStatus = "healthy"
    HealthStatusDegraded  HealthStatus = "degraded"
    HealthStatusUnhealthy HealthStatus = "unhealthy"
    HealthStatusUnknown   HealthStatus = "unknown"
)

type HealthMonitor struct {
    manager       *AccountManager
    log           logger.Logger
    interval      time.Duration
    stopChan      chan struct{}
    running       bool
    mu            sync.RWMutex
    healthHistory map[string][]*HealthCheckResult
    historyMu     sync.RWMutex
    config        *HealthMonitorConfig
}

type HealthMonitorConfig struct {
    CheckInterval        time.Duration
    ConnectionTimeout    time.Duration
    MaxHistorySize       int
    FailureThreshold     int
    DegradedThreshold    float64
    UnhealthyThreshold   float64
    RecoveryThreshold    float64
    ConsecutiveChecks    int
}

type AccountHealth struct {
    Score              float64
    Status             HealthStatus
    LastCheckTime      time.Time
    ConsecutiveSuccess int
    ConsecutiveFails   int
    ResponseTime       time.Duration
    ErrorRate          float64
    Details            map[string]interface{}
    mu                 sync.RWMutex
}

type HealthCheckResult struct {
    AccountID      string
    Timestamp      time.Time
    Score          float64
    Status         HealthStatus
    Success        bool
    ResponseTime   time.Duration
    ErrorMessage   string
    CheckType      string
    Metrics        *HealthMetrics
}

type HealthMetrics struct {
    ConnectionSuccess bool
    AuthSuccess       bool
    ResponseTime      time.Duration
    SuccessRate       float64
    ErrorCount        int
    LastError         string
    ProviderHealth    float64
    UsageHealth       float64
}

type AccountLimits struct {
    DailyLimit    int
    RotationLimit int
    DailySent     int
    RotationSent  int
    LastReset     time.Time
    mu            sync.RWMutex
}

func NewHealthMonitor(manager *AccountManager, log logger.Logger, interval time.Duration) *HealthMonitor {
    if interval == 0 {
        interval = 5 * time.Minute
    }

    return &HealthMonitor{
        manager:       manager,
        log:           log,
        interval:      interval,
        stopChan:      make(chan struct{}),
        healthHistory: make(map[string][]*HealthCheckResult),
        config:        DefaultHealthMonitorConfig(),
    }
}

func DefaultHealthMonitorConfig() *HealthMonitorConfig {
    return &HealthMonitorConfig{
        CheckInterval:      5 * time.Minute,
        ConnectionTimeout:  30 * time.Second,
        MaxHistorySize:     100,
        FailureThreshold:   5,
        DegradedThreshold:  50.0,
        UnhealthyThreshold: 30.0,
        RecoveryThreshold:  70.0,
        ConsecutiveChecks:  3,
    }
}

func (hm *HealthMonitor) Start() {
    hm.mu.Lock()
    if hm.running {
        hm.mu.Unlock()
        return
    }
    hm.running = true
    hm.mu.Unlock()

    hm.log.Info("health monitor started",
        logger.Duration("interval", hm.interval),
    )

    go hm.monitorLoop()
}

func (hm *HealthMonitor) Stop() {
    hm.mu.Lock()
    if !hm.running {
        hm.mu.Unlock()
        return
    }
    hm.running = false
    hm.mu.Unlock()

    close(hm.stopChan)
    hm.log.Info("health monitor stopped")
}

func (hm *HealthMonitor) monitorLoop() {
    ticker := time.NewTicker(hm.interval)
    defer ticker.Stop()

    hm.performHealthCheck()

    for {
        select {
        case <-ticker.C:
            hm.performHealthCheck()
        case <-hm.stopChan:
            return
        }
    }
}

func (hm *HealthMonitor) performHealthCheck() {
    hm.log.Debug("performing health check on all accounts")

    accounts := hm.manager.ListAccounts(nil)
    var wg sync.WaitGroup

    for _, acc := range accounts {
        wg.Add(1)
        go func(account *ManagedAccount) {
            defer wg.Done()
            hm.checkAccountHealth(account)
        }(acc)
    }

    wg.Wait()

    hm.log.Debug("health check completed",
        logger.Int("accounts_checked", len(accounts)),
    )
}

func (hm *HealthMonitor) checkAccountHealth(acc *ManagedAccount) {
    startTime := time.Now()
    
    result := &HealthCheckResult{
        AccountID: acc.Account.ID,
        Timestamp: startTime,
        CheckType: "periodic",
        Metrics:   &HealthMetrics{},
    }

    ctx, cancel := context.WithTimeout(context.Background(), hm.config.ConnectionTimeout)
    defer cancel()

    err := acc.Provider.TestConnection(ctx)
    responseTime := time.Since(startTime)

    result.ResponseTime = responseTime
    result.Metrics.ResponseTime = responseTime

    if err != nil {
        result.Success = false
        result.ErrorMessage = err.Error()
        result.Metrics.ConnectionSuccess = false
        hm.handleCheckFailure(acc, result)
    } else {
        result.Success = true
        result.Metrics.ConnectionSuccess = true
        result.Metrics.AuthSuccess = true
        hm.handleCheckSuccess(acc, result)
    }

    metrics := hm.calculateHealthMetrics(acc)
    result.Metrics.SuccessRate = metrics.SuccessRate
    result.Metrics.ErrorCount = metrics.ErrorCount
    result.Metrics.ProviderHealth = metrics.ProviderHealth
    result.Metrics.UsageHealth = metrics.UsageHealth

    score := hm.calculateHealthScore(acc, metrics)
    result.Score = score
    result.Status = hm.determineHealthStatus(score)

    hm.updateAccountHealth(acc, result)
    hm.recordHealthCheck(result)
}

func (hm *HealthMonitor) handleCheckSuccess(acc *ManagedAccount, result *HealthCheckResult) {
    acc.Health.mu.Lock()
    defer acc.Health.mu.Unlock()

    acc.Health.ConsecutiveSuccess++
    acc.Health.ConsecutiveFails = 0

    if acc.Health.ConsecutiveSuccess >= hm.config.ConsecutiveChecks {
        if acc.Health.Status == HealthStatusUnhealthy && acc.Health.Score >= hm.config.RecoveryThreshold {
            acc.Health.Status = HealthStatusDegraded
            hm.log.Info("account health recovering",
                logger.String("account_id", acc.Account.ID),
                logger.String("email", acc.Account.Email),
                logger.String("status", string(HealthStatusDegraded)),
            )
        }
    }
}

func (hm *HealthMonitor) handleCheckFailure(acc *ManagedAccount, result *HealthCheckResult) {
    acc.Health.mu.Lock()
    defer acc.Health.mu.Unlock()

    acc.Health.ConsecutiveFails++
    acc.Health.ConsecutiveSuccess = 0

    hm.log.Warn("account health check failed",
        logger.String("account_id", acc.Account.ID),
        logger.String("email", acc.Account.Email),
        logger.Int("consecutive_fails", acc.Health.ConsecutiveFails),
        logger.String("error", result.ErrorMessage),
    )

    if acc.Health.ConsecutiveFails >= hm.config.FailureThreshold {
        if hm.manager.suspension != nil {
            hm.manager.suspension.CheckAndSuspend(acc, result.ErrorMessage)
        }
    }
}

func (hm *HealthMonitor) calculateHealthMetrics(acc *ManagedAccount) *HealthMetrics {
    metrics := &HealthMetrics{}

    acc.mu.RLock()
    totalSent := acc.Account.Stats.TotalSent
    totalFailed := acc.Account.Stats.TotalFailed
    acc.mu.RUnlock()

    total := totalSent + totalFailed
    if total > 0 {
        metrics.SuccessRate = float64(totalSent) / float64(total) * 100.0
        metrics.ErrorCount = int(totalFailed)
    } else {
        metrics.SuccessRate = 100.0
    }

    if acc.Provider != nil {
        providerHealth := acc.Provider.GetHealth()
        if providerHealth != nil {
            metrics.ProviderHealth = 100.0 - providerHealth.ErrorRate
        } else {
            metrics.ProviderHealth = 100.0
        }
    } else {
        metrics.ProviderHealth = 0.0
    }

    acc.Limits.mu.RLock()
    if acc.Limits.DailyLimit > 0 {
        usagePercent := float64(acc.Limits.DailySent) / float64(acc.Limits.DailyLimit)
        metrics.UsageHealth = (1.0 - usagePercent) * 100.0
    } else {
        metrics.UsageHealth = 100.0
    }
    acc.Limits.mu.RUnlock()

    return metrics
}

func (hm *HealthMonitor) calculateHealthScore(acc *ManagedAccount, metrics *HealthMetrics) float64 {
    score := 100.0

    successRateWeight := 0.35
    score = metrics.SuccessRate * successRateWeight

    providerHealthWeight := 0.25
    score += metrics.ProviderHealth * providerHealthWeight

    usageHealthWeight := 0.15
    score += metrics.UsageHealth * usageHealthWeight

    consecutiveFailsPenalty := 0.15
    acc.Health.mu.RLock()
    consecutiveFails := acc.Health.ConsecutiveFails
    acc.Health.mu.RUnlock()

    if consecutiveFails > 0 {
        failPenalty := float64(consecutiveFails) * 10.0
        if failPenalty > 100.0 {
            failPenalty = 100.0
        }
        score -= failPenalty * consecutiveFailsPenalty
    }

    responseTimeWeight := 0.10
    if metrics.ResponseTime > 0 {
        responseScore := 100.0
        if metrics.ResponseTime > 10*time.Second {
            responseScore = 0.0
        } else if metrics.ResponseTime > 5*time.Second {
            responseScore = 50.0
        } else if metrics.ResponseTime > 2*time.Second {
            responseScore = 75.0
        }
        score += responseScore * responseTimeWeight
    }

    if acc.Suspension.IsSuspended {
        score *= 0.1
    }

    if score < 0 {
        score = 0
    }
    if score > 100 {
        score = 100
    }

    return score
}

func (hm *HealthMonitor) determineHealthStatus(score float64) HealthStatus {
    if score >= hm.config.DegradedThreshold {
        return HealthStatusHealthy
    } else if score >= hm.config.UnhealthyThreshold {
        return HealthStatusDegraded
    }
    return HealthStatusUnhealthy
}

func (hm *HealthMonitor) updateAccountHealth(acc *ManagedAccount, result *HealthCheckResult) {
    acc.Health.mu.Lock()
    defer acc.Health.mu.Unlock()

    acc.Health.Score = result.Score
    acc.Health.Status = result.Status
    acc.Health.LastCheckTime = result.Timestamp
    acc.Health.ResponseTime = result.ResponseTime

    if result.Metrics != nil {
        acc.Health.ErrorRate = 100.0 - result.Metrics.SuccessRate
    }

    if acc.Health.Details == nil {
        acc.Health.Details = make(map[string]interface{})
    }

    acc.Health.Details["last_check"] = result.Timestamp
    acc.Health.Details["response_time"] = result.ResponseTime
    acc.Health.Details["success"] = result.Success
    if result.ErrorMessage != "" {
        acc.Health.Details["last_error"] = result.ErrorMessage
    }
}

func (hm *HealthMonitor) recordHealthCheck(result *HealthCheckResult) {
    hm.historyMu.Lock()
    defer hm.historyMu.Unlock()

    history, exists := hm.healthHistory[result.AccountID]
    if !exists {
        history = make([]*HealthCheckResult, 0)
    }

    history = append(history, result)

    if len(history) > hm.config.MaxHistorySize {
        history = history[1:]
    }

    hm.healthHistory[result.AccountID] = history
}

func (hm *HealthMonitor) GetAccountHealthHistory(accountID string, limit int) []*HealthCheckResult {
    hm.historyMu.RLock()
    defer hm.historyMu.RUnlock()

    history, exists := hm.healthHistory[accountID]
    if !exists {
        return []*HealthCheckResult{}
    }

    if limit <= 0 || limit > len(history) {
        limit = len(history)
    }

    start := len(history) - limit
    if start < 0 {
        start = 0
    }

    result := make([]*HealthCheckResult, limit)
    copy(result, history[start:])

    return result
}

func (hm *HealthMonitor) GetHealthSummary() map[string]interface{} {
    accounts := hm.manager.ListAccounts(nil)

    healthy := 0
    degraded := 0
    unhealthy := 0
    totalScore := 0.0

    for _, acc := range accounts {
        acc.Health.mu.RLock()
        status := acc.Health.Status
        score := acc.Health.Score
        acc.Health.mu.RUnlock()

        totalScore += score

        switch status {
        case HealthStatusHealthy:
            healthy++
        case HealthStatusDegraded:
            degraded++
        case HealthStatusUnhealthy:
            unhealthy++
        }
    }

    avgScore := 0.0
    if len(accounts) > 0 {
        avgScore = totalScore / float64(len(accounts))
    }

    return map[string]interface{}{
        "total_accounts":   len(accounts),
        "healthy":          healthy,
        "degraded":         degraded,
        "unhealthy":        unhealthy,
        "average_score":    avgScore,
        "last_check_time": time.Now(),
    }
}

func (hm *HealthMonitor) CheckAccountNow(accountID string) (*HealthCheckResult, error) {
    acc, err := hm.manager.GetAccount(accountID)
    if err != nil {
        return nil, err
    }

    hm.checkAccountHealth(acc)

    hm.historyMu.RLock()
    defer hm.historyMu.RUnlock()

    history := hm.healthHistory[accountID]
    if len(history) == 0 {
        return nil, nil
    }

    return history[len(history)-1], nil
}

func (hm *HealthMonitor) GetUnhealthyAccounts() []*ManagedAccount {
    accounts := hm.manager.ListAccounts(nil)
    var unhealthy []*ManagedAccount

    for _, acc := range accounts {
        acc.Health.mu.RLock()
        status := acc.Health.Status
        acc.Health.mu.RUnlock()

        if status == HealthStatusUnhealthy {
            unhealthy = append(unhealthy, acc)
        }
    }

    return unhealthy
}

func (hm *HealthMonitor) GetDegradedAccounts() []*ManagedAccount {
    accounts := hm.manager.ListAccounts(nil)
    var degraded []*ManagedAccount

    for _, acc := range accounts {
        acc.Health.mu.RLock()
        status := acc.Health.Status
        acc.Health.mu.RUnlock()

        if status == HealthStatusDegraded {
            degraded = append(degraded, acc)
        }
    }

    return degraded
}

func (hm *HealthMonitor) ResetAccountHealth(accountID string) error {
    acc, err := hm.manager.GetAccount(accountID)
    if err != nil {
        return err
    }

    acc.Health.mu.Lock()
    defer acc.Health.mu.Unlock()

    acc.Health.Score = 100.0
    acc.Health.Status = HealthStatusHealthy
    acc.Health.ConsecutiveSuccess = 0
    acc.Health.ConsecutiveFails = 0
    acc.Health.ErrorRate = 0.0
    acc.Health.LastCheckTime = time.Now()

    hm.log.Info("account health reset",
        logger.String("account_id", accountID),
        logger.String("email", acc.Account.Email),
    )

    return nil
}

func (hm *HealthMonitor) GetAverageHealthScore() float64 {
    accounts := hm.manager.ListAccounts(nil)
    if len(accounts) == 0 {
        return 0.0
    }

    totalScore := 0.0
    for _, acc := range accounts {
        acc.Health.mu.RLock()
        totalScore += acc.Health.Score
        acc.Health.mu.RUnlock()
    }

    return totalScore / float64(len(accounts))
}

func (hm *HealthMonitor) GetHealthTrend(accountID string, duration time.Duration) []float64 {
    hm.historyMu.RLock()
    defer hm.historyMu.RUnlock()

    history, exists := hm.healthHistory[accountID]
    if !exists {
        return []float64{}
    }

    cutoff := time.Now().Add(-duration)
    scores := make([]float64, 0)

    for _, result := range history {
        if result.Timestamp.After(cutoff) {
            scores = append(scores, result.Score)
        }
    }

    return scores
}

func (hm *HealthMonitor) IsRunning() bool {
    hm.mu.RLock()
    defer hm.mu.RUnlock()
    return hm.running
}

func (hm *HealthMonitor) UpdateConfig(config *HealthMonitorConfig) {
    if config == nil {
        return
    }

    hm.config = config

    if config.CheckInterval > 0 && config.CheckInterval != hm.interval {
        hm.interval = config.CheckInterval
        
        if hm.IsRunning() {
            hm.Stop()
            time.Sleep(100 * time.Millisecond)
            hm.Start()
        }
    }

    hm.log.Info("health monitor config updated",
        logger.Duration("interval", hm.interval),
    )
}

type SuspensionState struct {
    IsSuspended bool
    Reason      string
    SuspendedAt time.Time
    ResumeAt    time.Time
    mu          sync.RWMutex
}

func (ah *AccountHealth) GetScore() float64 {
    ah.mu.RLock()
    defer ah.mu.RUnlock()
    return ah.Score
}

func (ah *AccountHealth) GetStatus() HealthStatus {
    ah.mu.RLock()
    defer ah.mu.RUnlock()
    return ah.Status
}

func (ah *AccountHealth) IsHealthy() bool {
    ah.mu.RLock()
    defer ah.mu.RUnlock()
    return ah.Status == HealthStatusHealthy
}

func (al *AccountLimits) IncrementDailySent() {
    al.mu.Lock()
    defer al.mu.Unlock()
    al.DailySent++
}

func (al *AccountLimits) IncrementRotationSent() {
    al.mu.Lock()
    defer al.mu.Unlock()
    al.RotationSent++
}

func (al *AccountLimits) ResetDaily() {
    al.mu.Lock()
    defer al.mu.Unlock()
    al.DailySent = 0
    al.LastReset = time.Now()
}

func (al *AccountLimits) ResetRotation() {
    al.mu.Lock()
    defer al.mu.Unlock()
    al.RotationSent = 0
}

func (al *AccountLimits) GetDailyRemaining() int {
    al.mu.RLock()
    defer al.mu.RUnlock()
    
    if al.DailyLimit == 0 {
        return 0
    }
    
    remaining := al.DailyLimit - al.DailySent
    if remaining < 0 {
        remaining = 0
    }
    
    return remaining
}

func (al *AccountLimits) GetRotationRemaining() int {
    al.mu.RLock()
    defer al.mu.RUnlock()
    
    if al.RotationLimit == 0 {
        return 0
    }
    
    remaining := al.RotationLimit - al.RotationSent
    if remaining < 0 {
        remaining = 0
    }
    
    return remaining
}

func (al *AccountLimits) IsDailyLimitReached() bool {
    al.mu.RLock()
    defer al.mu.RUnlock()
    
    if al.DailyLimit == 0 {
        return false
    }
    
    return al.DailySent >= al.DailyLimit
}

func (al *AccountLimits) IsRotationLimitReached() bool {
    al.mu.RLock()
    defer al.mu.RUnlock()
    
    if al.RotationLimit == 0 {
        return false
    }
    
    return al.RotationSent >= al.RotationLimit
}
