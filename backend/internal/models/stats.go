package models

import (
    "encoding/json"
    "fmt"
    "time"
)

type ProviderType = Provider

type CampaignStatsReport struct {
    ID                     string                 `json:"id"`
    CampaignID             string                 `json:"campaign_id"`
    TenantID               string                 `json:"tenant_id"`
    CampaignName           string                 `json:"campaign_name"`
    Status                 CampaignStatus         `json:"status"`
    TotalRecipients        int64                  `json:"total_recipients"`
    EmailsSent             int64                  `json:"emails_sent"`
    EmailsFailed           int64                  `json:"emails_failed"`
    EmailsQueued           int64                  `json:"emails_queued"`
    EmailsPending          int64                  `json:"emails_pending"`
    EmailsBounced          int64                  `json:"emails_bounced"`
    EmailsDelivered        int64                  `json:"emails_delivered"`
    SuccessRate            float64                `json:"success_rate"`
    FailureRate            float64                `json:"failure_rate"`
    Progress               float64                `json:"progress"`
    ThroughputPerSecond    float64                `json:"throughput_per_sec"`
    ThroughputPerMinute    float64                `json:"throughput_per_min"`
    ThroughputPerHour      float64                `json:"throughput_per_hour"`
    AverageSendTimeMs      float64                `json:"average_send_time_ms"`
    EstimatedTimeRemaining time.Duration          `json:"estimated_time_remaining"`
    ETATimestamp           *time.Time             `json:"eta_timestamp,omitempty"`
    StartedAt              *time.Time             `json:"started_at,omitempty"`
    CompletedAt            *time.Time             `json:"completed_at,omitempty"`
    PausedAt               *time.Time             `json:"paused_at,omitempty"`
    Duration               time.Duration          `json:"duration"`
    ActiveWorkers          int                    `json:"active_workers"`
    AccountsUsed           int                    `json:"accounts_used"`
    AccountsSuspended      int                    `json:"accounts_suspended"`
    TemplatesUsed          int                    `json:"templates_used"`
    ProxiesUsed            int                    `json:"proxies_used"`
    AttachmentsSent        int64                  `json:"attachments_sent"`
    TotalDataSentMB        float64                `json:"total_data_sent_mb"`
    RetryCount             int64                  `json:"retry_count"`
    ErrorsByType           map[string]int64       `json:"errors_by_type"`
    ErrorsByAccount        map[string]int64       `json:"errors_by_account"`
    LastError              string                 `json:"last_error,omitempty"`
    CreatedAt              time.Time              `json:"created_at"`
    UpdatedAt              time.Time              `json:"updated_at"`
    Metadata               map[string]interface{} `json:"metadata,omitempty"`
}

type EmailStats struct {
    ID                 string    `json:"id"`
    TenantID           string    `json:"tenant_id"`
    TimeRange          string    `json:"time_range"`
    TotalSent          int64     `json:"total_sent"`
    TotalFailed        int64     `json:"total_failed"`
    TotalQueued        int64     `json:"total_queued"`
    TotalBounced       int64     `json:"total_bounced"`
    TotalDelivered     int64     `json:"total_delivered"`
    UniqueRecipients   int64     `json:"unique_recipients"`
    AverageSendTimeMs  float64   `json:"average_send_time_ms"`
    MinSendTimeMs      float64   `json:"min_send_time_ms"`
    MaxSendTimeMs      float64   `json:"max_send_time_ms"`
    SuccessRate        float64   `json:"success_rate"`
    BounceRate         float64   `json:"bounce_rate"`
    AverageEmailSizeKB float64   `json:"average_email_size_kb"`
    TotalDataSentMB    float64   `json:"total_data_sent_mb"`
    StartTime          time.Time `json:"start_time"`
    EndTime            time.Time `json:"end_time"`
    UpdatedAt          time.Time `json:"updated_at"`
}

type AccountStatsReport struct {
    ID                  string                 `json:"id"`
    AccountID           string                 `json:"account_id"`
    TenantID            string                 `json:"tenant_id"`
    AccountEmail        string                 `json:"account_email"`
    Provider            ProviderType           `json:"provider"`
    Status              AccountStatus          `json:"status"`
    TotalEmailsSent     int64                  `json:"total_emails_sent"`
    TodayEmailsSent     int64                  `json:"today_emails_sent"`
    DailyLimit          int64                  `json:"daily_limit"`
    DailyLimitRemaining int64                  `json:"daily_limit_remaining"`
    DailyLimitPercent   float64                `json:"daily_limit_percent"`
    RotationLimit       int64                  `json:"rotation_limit"`
    RotationCount       int64                  `json:"rotation_count"`
    SuccessCount        int64                  `json:"success_count"`
    FailureCount        int64                  `json:"failure_count"`
    ConsecutiveFailures int64                  `json:"consecutive_failures"`
    SuccessRate         float64                `json:"success_rate"`
    HealthScore         float64                `json:"health_score"`
    SpamScore           float64                `json:"spam_score"`
    LastUsedAt          *time.Time             `json:"last_used_at,omitempty"`
    LastSuccessAt       *time.Time             `json:"last_success_at,omitempty"`
    LastFailureAt       *time.Time             `json:"last_failure_at,omitempty"`
    SuspendedAt         *time.Time             `json:"suspended_at,omitempty"`
    SuspensionReason    string                 `json:"suspension_reason,omitempty"`
    CooldownUntil       *time.Time             `json:"cooldown_until,omitempty"`
    AverageSendTimeMs   float64                `json:"average_send_time_ms"`
    TotalDataSentMB     float64                `json:"total_data_sent_mb"`
    CreatedAt           time.Time              `json:"created_at"`
    UpdatedAt           time.Time              `json:"updated_at"`
    Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type SystemStats struct {
    ID                  string                 `json:"id"`
    TenantID            string                 `json:"tenant_id"`
    CPUUsagePercent     float64                `json:"cpu_usage_percent"`
    MemoryUsagePercent  float64                `json:"memory_usage_percent"`
    MemoryUsedMB        float64                `json:"memory_used_mb"`
    MemoryTotalMB       float64                `json:"memory_total_mb"`
    DiskUsagePercent    float64                `json:"disk_usage_percent"`
    DiskUsedGB          float64                `json:"disk_used_gb"`
    DiskTotalGB         float64                `json:"disk_total_gb"`
    NetworkSentMB       float64                `json:"network_sent_mb"`
    NetworkReceivedMB   float64                `json:"network_received_mb"`
    ActiveConnections   int                    `json:"active_connections"`
    ActiveWorkers       int                    `json:"active_workers"`
    ActiveCampaigns     int                    `json:"active_campaigns"`
    QueuedEmails        int64                  `json:"queued_emails"`
    CacheHitRate        float64                `json:"cache_hit_rate"`
    CacheSizeMB         float64                `json:"cache_size_mb"`
    DatabaseConnections int                    `json:"database_connections"`
    Uptime              time.Duration          `json:"uptime"`
    StartedAt           time.Time              `json:"started_at"`
    Timestamp           time.Time              `json:"timestamp"`
    Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type PerformanceStats struct {
    ID                   string                 `json:"id"`
    TenantID             string                 `json:"tenant_id"`
    Operation            string                 `json:"operation"`
    TotalExecutions      int64                  `json:"total_executions"`
    SuccessfulExecutions int64                  `json:"successful_executions"`
    FailedExecutions     int64                  `json:"failed_executions"`
    AverageDurationMs    float64                `json:"average_duration_ms"`
    MinDurationMs        float64                `json:"min_duration_ms"`
    MaxDurationMs        float64                `json:"max_duration_ms"`
    MedianDurationMs     float64                `json:"median_duration_ms"`
    P95DurationMs        float64                `json:"p95_duration_ms"`
    P99DurationMs        float64                `json:"p99_duration_ms"`
    ThroughputPerSecond  float64                `json:"throughput_per_second"`
    ErrorRate            float64                `json:"error_rate"`
    LastExecutedAt       *time.Time             `json:"last_executed_at,omitempty"`
    StartTime            time.Time              `json:"start_time"`
    EndTime              time.Time              `json:"end_time"`
    UpdatedAt            time.Time              `json:"updated_at"`
    Metadata             map[string]interface{} `json:"metadata,omitempty"`
}

type AggregateStats struct {
    ID                string                 `json:"id"`
    TenantID          string                 `json:"tenant_id"`
    Period            string                 `json:"period"`
    StartTime         time.Time              `json:"start_time"`
    EndTime           time.Time              `json:"end_time"`
    TotalCampaigns    int64                  `json:"total_campaigns"`
    TotalEmails       int64                  `json:"total_emails"`
    EmailsSent        int64                  `json:"emails_sent"`
    EmailsFailed      int64                  `json:"emails_failed"`
    SuccessRate       float64                `json:"success_rate"`
    TotalAccounts     int64                  `json:"total_accounts"`
    ActiveAccounts    int64                  `json:"active_accounts"`
    SuspendedAccounts int64                  `json:"suspended_accounts"`
    TotalTemplates    int64                  `json:"total_templates"`
    TotalRecipients   int64                  `json:"total_recipients"`
    TotalProxies      int64                  `json:"total_proxies"`
    DataSentMB        float64                `json:"data_sent_mb"`
    AvgThroughput     float64                `json:"avg_throughput"`
    PeakThroughput    float64                `json:"peak_throughput"`
    CreatedAt         time.Time              `json:"created_at"`
    Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

func NewCampaignStatsReport(campaignID, tenantID, campaignName string, totalRecipients int64) *CampaignStatsReport {
    now := time.Now()
    return &CampaignStatsReport{
        ID:              generateStatsID(),
        CampaignID:      campaignID,
        TenantID:        tenantID,
        CampaignName:    campaignName,
        TotalRecipients: totalRecipients,
        ErrorsByType:    make(map[string]int64),
        ErrorsByAccount: make(map[string]int64),
        CreatedAt:       now,
        UpdatedAt:       now,
        Metadata:        make(map[string]interface{}),
    }
}

func NewAccountStatsReport(accountID, tenantID, accountEmail string, provider ProviderType) *AccountStatsReport {
    now := time.Now()
    return &AccountStatsReport{
        ID:           generateStatsID(),
        AccountID:    accountID,
        TenantID:     tenantID,
        AccountEmail: accountEmail,
        Provider:     provider,
        HealthScore:  100.0,
        CreatedAt:    now,
        UpdatedAt:    now,
        Metadata:     make(map[string]interface{}),
    }
}

func NewSystemStats(tenantID string) *SystemStats {
    return &SystemStats{
        ID:        generateStatsID(),
        TenantID:  tenantID,
        Timestamp: time.Now(),
        Metadata:  make(map[string]interface{}),
    }
}


func (cs *CampaignStatsReport) CalculateMetrics() {
    total := cs.EmailsSent + cs.EmailsFailed

    if total > 0 {
        cs.SuccessRate = (float64(cs.EmailsSent) / float64(total)) * 100
        cs.FailureRate = (float64(cs.EmailsFailed) / float64(total)) * 100
    }

    if cs.TotalRecipients > 0 {
        cs.Progress = (float64(total) / float64(cs.TotalRecipients)) * 100
    }

    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) CalculateThroughput() {
    if cs.StartedAt == nil {
        return
    }

    duration := time.Since(*cs.StartedAt)
    if duration.Seconds() > 0 {
        totalSent := float64(cs.EmailsSent)
        cs.ThroughputPerSecond = totalSent / duration.Seconds()
        cs.ThroughputPerMinute = totalSent / duration.Minutes()
        cs.ThroughputPerHour = totalSent / duration.Hours()
    }

    cs.Duration = duration
    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) CalculateETA() {
    if cs.StartedAt == nil || cs.Progress >= 100 || cs.ThroughputPerSecond <= 0 {
        cs.EstimatedTimeRemaining = 0
        cs.ETATimestamp = nil
        return
    }

    remaining := cs.TotalRecipients - (cs.EmailsSent + cs.EmailsFailed)
    if remaining <= 0 {
        cs.EstimatedTimeRemaining = 0
        cs.ETATimestamp = nil
        return
    }

    secondsRemaining := float64(remaining) / cs.ThroughputPerSecond
    cs.EstimatedTimeRemaining = time.Duration(secondsRemaining) * time.Second

    eta := time.Now().Add(cs.EstimatedTimeRemaining)
    cs.ETATimestamp = &eta
    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) IncrementSent() {
    cs.EmailsSent++
    cs.CalculateMetrics()
    cs.CalculateThroughput()
    cs.CalculateETA()
}

func (cs *CampaignStatsReport) IncrementFailed(errorType, accountID string) {
    cs.EmailsFailed++

    if errorType != "" {
        cs.ErrorsByType[errorType]++
    }

    if accountID != "" {
        cs.ErrorsByAccount[accountID]++
    }

    cs.CalculateMetrics()
    cs.CalculateThroughput()
    cs.CalculateETA()
}

func (cs *CampaignStatsReport) IncrementQueued() {
    cs.EmailsQueued++
    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) IncrementBounced() {
    cs.EmailsBounced++
    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) IncrementDelivered() {
    cs.EmailsDelivered++
    cs.UpdatedAt = time.Now()
}

func (cs *CampaignStatsReport) MarkStarted() {
    now := time.Now()
    cs.StartedAt = &now
    cs.Status = CampaignStatusRunning
    cs.UpdatedAt = now
}

func (cs *CampaignStatsReport) MarkCompleted() {
    now := time.Now()
    cs.CompletedAt = &now
    cs.Status = CampaignStatusCompleted
    cs.Progress = 100
    cs.CalculateThroughput()
    cs.UpdatedAt = now
}

func (cs *CampaignStatsReport) MarkPaused() {
    now := time.Now()
    cs.PausedAt = &now
    cs.Status = CampaignStatusPaused
    cs.UpdatedAt = now
}

func (cs *CampaignStatsReport) GetAverageSendTime() time.Duration {
    if cs.AverageSendTimeMs > 0 {
        return time.Duration(cs.AverageSendTimeMs) * time.Millisecond
    }
    return 0
}

func (cs *CampaignStatsReport) GetRemainingCount() int64 {
    return cs.TotalRecipients - (cs.EmailsSent + cs.EmailsFailed)
}

func (cs *CampaignStatsReport) IsCompleted() bool {
    return cs.Progress >= 100 || cs.Status == CampaignStatusCompleted
}

func (cs *CampaignStatsReport) GetDuration() time.Duration {
    if cs.StartedAt == nil {
        return 0
    }

    if cs.CompletedAt != nil {
        return cs.CompletedAt.Sub(*cs.StartedAt)
    }

    return time.Since(*cs.StartedAt)
}

func (cs *CampaignStatsReport) ToJSON() (string, error) {
    data, err := json.Marshal(cs)
    if err != nil {
        return "", err
    }
    return string(data), nil
}
func (as *AccountStatsReport) CalculateMetrics() {
    total := as.SuccessCount + as.FailureCount
    if total > 0 {
        as.SuccessRate = (float64(as.SuccessCount) / float64(total)) * 100
    }

    if as.DailyLimit > 0 {
        as.DailyLimitRemaining = as.DailyLimit - as.TodayEmailsSent
        as.DailyLimitPercent = (float64(as.TodayEmailsSent) / float64(as.DailyLimit)) * 100
    }

    as.UpdatedAt = time.Now()
}

func (as *AccountStatsReport) IncrementSuccess() {
    as.SuccessCount++
    as.TotalEmailsSent++
    as.TodayEmailsSent++
    as.ConsecutiveFailures = 0
    now := time.Now()
    as.LastSuccessAt = &now
    as.LastUsedAt = &now
    as.CalculateMetrics()
    as.UpdateHealthScore()
}

func (as *AccountStatsReport) IncrementFailure() {
    as.FailureCount++
    as.ConsecutiveFailures++
    now := time.Now()
    as.LastFailureAt = &now
    as.LastUsedAt = &now
    as.CalculateMetrics()
    as.UpdateHealthScore()
}

func (as *AccountStatsReport) UpdateHealthScore() {
    baseScore := 100.0

    if as.SuccessCount+as.FailureCount > 0 {
        baseScore = as.SuccessRate
    }

    if as.ConsecutiveFailures > 0 {
        baseScore -= float64(as.ConsecutiveFailures) * 10
    }

    if as.SpamScore > 0 {
        baseScore -= as.SpamScore * 20
    }

    if baseScore < 0 {
        baseScore = 0
    }
    if baseScore > 100 {
        baseScore = 100
    }

    as.HealthScore = baseScore
    as.UpdatedAt = time.Now()
}

func (as *AccountStatsReport) ResetDailyStats() {
    as.TodayEmailsSent = 0
    as.CalculateMetrics()
}

func (as *AccountStatsReport) IsDailyLimitReached() bool {
    return as.DailyLimit > 0 && as.TodayEmailsSent >= as.DailyLimit
}

func (as *AccountStatsReport) IsRotationLimitReached() bool {
    return as.RotationLimit > 0 && as.RotationCount >= as.RotationLimit
}

func (as *AccountStatsReport) IsHealthy() bool {
    return as.HealthScore >= 50
}

func (as *AccountStatsReport) IsInCooldown() bool {
    if as.CooldownUntil == nil {
        return false
    }
    return time.Now().Before(*as.CooldownUntil)
}

func (as *AccountStatsReport) SetCooldown(duration time.Duration) {
    cooldownUntil := time.Now().Add(duration)
    as.CooldownUntil = &cooldownUntil
    as.UpdatedAt = time.Now()
}

func (as *AccountStatsReport) GetDailyLimitUsagePercent() float64 {
    return as.DailyLimitPercent
}

func (ss *SystemStats) CalculateUsage(cpuPercent, memUsedMB, memTotalMB, diskUsedGB, diskTotalGB float64) {
    ss.CPUUsagePercent = cpuPercent
    ss.MemoryUsedMB = memUsedMB
    ss.MemoryTotalMB = memTotalMB
    ss.DiskUsedGB = diskUsedGB
    ss.DiskTotalGB = diskTotalGB

    if memTotalMB > 0 {
        ss.MemoryUsagePercent = (memUsedMB / memTotalMB) * 100
    }

    if diskTotalGB > 0 {
        ss.DiskUsagePercent = (diskUsedGB / diskTotalGB) * 100
    }

    ss.Timestamp = time.Now()
}

func (ss *SystemStats) IsHealthy() bool {
    return ss.CPUUsagePercent < 90 && ss.MemoryUsagePercent < 90 && ss.DiskUsagePercent < 90
}

func (ss *SystemStats) GetResourceStatus() string {
    if ss.CPUUsagePercent > 90 || ss.MemoryUsagePercent > 90 || ss.DiskUsagePercent > 90 {
        return "critical"
    }
    if ss.CPUUsagePercent > 75 || ss.MemoryUsagePercent > 75 || ss.DiskUsagePercent > 75 {
        return "warning"
    }
    return "healthy"
}

func (ps *PerformanceStats) CalculateMetrics() {
    total := ps.TotalExecutions
    if total > 0 {
        ps.ErrorRate = (float64(ps.FailedExecutions) / float64(total)) * 100
    }

    duration := ps.EndTime.Sub(ps.StartTime)
    if duration.Seconds() > 0 {
        ps.ThroughputPerSecond = float64(ps.TotalExecutions) / duration.Seconds()
    }

    ps.UpdatedAt = time.Now()
}

func (ps *PerformanceStats) RecordExecution(durationMs float64, success bool) {
    ps.TotalExecutions++

    if success {
        ps.SuccessfulExecutions++
    } else {
        ps.FailedExecutions++
    }

    if ps.MinDurationMs == 0 || durationMs < ps.MinDurationMs {
        ps.MinDurationMs = durationMs
    }

    if durationMs > ps.MaxDurationMs {
        ps.MaxDurationMs = durationMs
    }

    ps.AverageDurationMs = ((ps.AverageDurationMs * float64(ps.TotalExecutions-1)) + durationMs) / float64(ps.TotalExecutions)

    now := time.Now()
    ps.LastExecutedAt = &now
    ps.EndTime = now

    ps.CalculateMetrics()
}

func (as *AggregateStats) CalculateMetrics() {
    total := as.EmailsSent + as.EmailsFailed
    if total > 0 {
        as.SuccessRate = (float64(as.EmailsSent) / float64(total)) * 100
    }
}

func generateStatsID() string {
    return fmt.Sprintf("stats_%d", time.Now().UnixNano())
}


type StatsSnapshot struct {
    Timestamp        time.Time              `json:"timestamp"`
    CampaignStats    *CampaignStatsReport   `json:"campaign_stats,omitempty"`
    SystemStats      *SystemStats           `json:"system_stats,omitempty"`
    AccountStats     []AccountStatsReport   `json:"account_stats,omitempty"`
    PerformanceStats *PerformanceStats      `json:"performance_stats,omitempty"`
}

func NewStatsSnapshot() *StatsSnapshot {
    return &StatsSnapshot{
        Timestamp:    time.Now(),
        AccountStats: []AccountStatsReport{},
    }
}

func (ss *StatsSnapshot) AddAccountStats(stats AccountStatsReport) {
    ss.AccountStats = append(ss.AccountStats, stats)
}

func (ss *StatsSnapshot) ToJSON() (string, error) {
    data, err := json.Marshal(ss)
    if err != nil {
        return "", err
    }
    return string(data), nil
}
