package models

import (
        "errors"
        "fmt"
        "time"
    "email-campaign-system/internal/models/common"
)

type CampaignStatus string

const (
        CampaignStatusCreated   CampaignStatus = "created"
        CampaignStatusScheduled CampaignStatus = "scheduled"
        CampaignStatusRunning   CampaignStatus = "running"
        CampaignStatusPaused    CampaignStatus = "paused"
        CampaignStatusCompleted CampaignStatus = "completed"
        CampaignStatusFailed    CampaignStatus = "failed"
        CampaignStatusCancelled CampaignStatus = "cancelled"
        CampaignStatusStopped   CampaignStatus = "stopped"
)

type CampaignPriority string

const (
        CampaignPriorityLow      CampaignPriority = "low"
        CampaignPriorityNormal   CampaignPriority = "normal"
        CampaignPriorityHigh     CampaignPriority = "high"
        CampaignPriorityCritical CampaignPriority = "critical"
)

type CampaignType string

const (
        CampaignTypeOneTime   CampaignType = "one_time"
        CampaignTypeRecurring CampaignType = "recurring"
        CampaignTypeDrip      CampaignType = "drip"
        CampaignTypeAB        CampaignType = "ab_test"
)


type RotationStrategy = common.RotationStrategy

const (
        RotationStrategySequential = common.RotationStrategySequential
        RotationStrategyRandom     = common.RotationStrategyRandom
        RotationStrategyWeighted   = common.RotationStrategyWeighted
        RotationStrategyTimeBased  = common.RotationStrategyTimeBased
        RotationStrategyHealthBased = common.RotationStrategyHealthBased
)

type Campaign struct {
        ID                    string              `json:"id" db:"id"`
        SessionID             string              `json:"session_id" db:"session_id"`
        TenantID              string              `json:"tenant_id" db:"tenant_id"`
        Name                  string              `json:"name" db:"name"`
        Description           string              `json:"description" db:"description"`
        Type                  CampaignType        `json:"type" db:"type"`
        Status                CampaignStatus      `json:"status" db:"status"`
        Priority              CampaignPriority    `json:"priority" db:"priority"`
        Tags                  []string            `json:"tags" db:"tags"`
        AccountIDs            []string            `json:"account_ids" db:"account_ids"`
        TemplateIDs           []string            `json:"template_ids" db:"template_ids"`
        RecipientGroupID      string              `json:"recipient_group_id" db:"recipient_group_id"`
        TotalRecipients       int64               `json:"total_recipients" db:"total_recipients"`
        ProxyIDs              []string            `json:"proxy_ids" db:"proxy_ids"`
        Config                CampaignRuntimeConfig      `json:"config"`
        RotationConfig        RotationConfig      `json:"rotation_config"`
        CampaignRateLimitSettings       CampaignRateLimitSettings     `json:"rate_limit_config"`
        CampaignRetrySettings           CampaignRetrySettings         `json:"retry_config"`
        CampaignNotificationSettings    CampaignNotificationSettings  `json:"notification_config"`
        Schedule              *CampaignSchedule   `json:"schedule,omitempty"`
        Stats                 CampaignStats       `json:"stats"`
        Progress              CampaignProgress    `json:"progress"`
        StateData             map[string]interface{} `json:"state_data"`
        ErrorMessage          string              `json:"error_message" db:"error_message"`
        StartedAt             *time.Time          `json:"started_at" db:"started_at"`
        CompletedAt           *time.Time          `json:"completed_at" db:"completed_at"`
        PausedAt              *time.Time          `json:"paused_at" db:"paused_at"`
        LastCheckpointAt      *time.Time          `json:"last_checkpoint_at" db:"last_checkpoint_at"`
        CreatedAt             time.Time           `json:"created_at" db:"created_at"`
        UpdatedAt             time.Time           `json:"updated_at" db:"updated_at"`
        CreatedBy             string              `json:"created_by" db:"created_by"`
        UpdatedBy             string              `json:"updated_by" db:"updated_by"`
        Version               int                 `json:"version" db:"version"`
}

type CampaignRuntimeConfig struct {
        WorkerCount            int               `json:"worker_count"`
        BatchSize              int               `json:"batch_size"`
        DelayBetweenBatchesMs  int64             `json:"delay_between_batches_ms"`
        DelayBetweenEmailsMs   int64             `json:"delay_between_emails_ms"`
        MaxRetriesPerEmail     int               `json:"max_retries_per_email"`
        EnableProxy            bool              `json:"enable_proxy"`
        EnableAccountRotation  bool              `json:"enable_account_rotation"`
        EnableTemplateRotation bool              `json:"enable_template_rotation"`
        EnableAttachments      bool              `json:"enable_attachments"`
        EnableTracking         bool              `json:"enable_tracking"`
        EnableUnsubscribe      bool              `json:"enable_unsubscribe"`
        EnableDKIM             bool              `json:"enable_dkim"`
        TrackOpens             bool              `json:"track_opens"`
        TrackClicks            bool              `json:"track_clicks"`
        UseHTMLContent         bool              `json:"use_html_content"`
        UsePlainTextFallback   bool              `json:"use_plain_text_fallback"`
        PersonalizationEnabled bool              `json:"personalization_enabled"`
        SpamCheckEnabled       bool              `json:"spam_check_enabled"`
        SmartSending           bool              `json:"smart_sending"`
        TestMode               bool              `json:"test_mode"`
        TestEmailAddress       string            `json:"test_email_address,omitempty"`
        CustomHeaders          map[string]string `json:"custom_headers,omitempty"`
        Metadata               map[string]interface{} `json:"metadata,omitempty"`
}

type RotationConfig struct {
        AccountRotationStrategy    RotationStrategy `json:"account_rotation_strategy"`
        TemplateRotationStrategy   RotationStrategy `json:"template_rotation_strategy"`
        SenderNameRotationStrategy RotationStrategy `json:"sender_name_rotation_strategy"`
        SubjectRotationStrategy    RotationStrategy `json:"subject_rotation_strategy"`
        ProxyRotationStrategy      RotationStrategy `json:"proxy_rotation_strategy"`
        AccountRotationLimit       int              `json:"account_rotation_limit"`
        TemplateRotationLimit      int              `json:"template_rotation_limit"`
        EnableUnlimitedRotation    bool             `json:"enable_unlimited_rotation"`
        RotateOnError              bool             `json:"rotate_on_error"`
        RotateOnSpamDetection      bool             `json:"rotate_on_spam_detection"`
        CooldownPeriodSeconds      int              `json:"cooldown_period_seconds"`
}

type CampaignRateLimitSettings struct {
        GlobalRequestsPerSecond   float64 `json:"global_requests_per_second"`
        GlobalRequestsPerMinute   int     `json:"global_requests_per_minute"`
        GlobalRequestsPerHour     int     `json:"global_requests_per_hour"`
        AccountRequestsPerSecond  float64 `json:"account_requests_per_second"`
        AccountRequestsPerMinute  int     `json:"account_requests_per_minute"`
        AccountRequestsPerHour    int     `json:"account_requests_per_hour"`
        AccountDailyLimit         int     `json:"account_daily_limit"`
        EnableAdaptiveRateLimiting bool   `json:"enable_adaptive_rate_limiting"`
        EnableDistributedLimiter  bool    `json:"enable_distributed_limiter"`
}

type CampaignRetrySettings struct {
        MaxRetries             int     `json:"max_retries"`
        InitialBackoffMs       int64   `json:"initial_backoff_ms"`
        MaxBackoffMs           int64   `json:"max_backoff_ms"`
        BackoffMultiplier      float64 `json:"backoff_multiplier"`
        EnableExponentialBackoff bool  `json:"enable_exponential_backoff"`
        RetryableErrorCodes    []string `json:"retryable_error_codes"`
        FailOnMaxRetries       bool    `json:"fail_on_max_retries"`
}

type CampaignNotificationSettings struct {
        EnableNotifications       bool     `json:"enable_notifications"`
        NotifyOnStart             bool     `json:"notify_on_start"`
        NotifyOnComplete          bool     `json:"notify_on_complete"`
        NotifyOnFailure           bool     `json:"notify_on_failure"`
        NotifyOnPause             bool     `json:"notify_on_pause"`
        NotifyOnMilestone         bool     `json:"notify_on_milestone"`
        MilestonePercentages      []int    `json:"milestone_percentages"`
        TelegramEnabled           bool     `json:"telegram_enabled"`
        TelegramChatID            string   `json:"telegram_chat_id,omitempty"`
        EmailEnabled              bool     `json:"email_enabled"`
        EmailRecipients           []string `json:"email_recipients,omitempty"`
        WebhookEnabled            bool     `json:"webhook_enabled"`
        WebhookURL                string   `json:"webhook_url,omitempty"`
}

type CampaignSchedule struct {
        ScheduledStartTime    *time.Time `json:"scheduled_start_time,omitempty"`
        ScheduledEndTime      *time.Time `json:"scheduled_end_time,omitempty"`
        Timezone              string     `json:"timezone"`
        RecurringEnabled      bool       `json:"recurring_enabled"`
        RecurringPattern      string     `json:"recurring_pattern,omitempty"`
        RecurringInterval     int        `json:"recurring_interval"`
        RecurringUnit         string     `json:"recurring_unit"`
        RecurringEndDate      *time.Time `json:"recurring_end_date,omitempty"`
        RecurringOccurrences  int        `json:"recurring_occurrences"`
        SendOnWeekdays        []string   `json:"send_on_weekdays,omitempty"`
        SendBetweenHoursStart int        `json:"send_between_hours_start"`
        SendBetweenHoursEnd   int        `json:"send_between_hours_end"`
        PauseDuringOffHours   bool       `json:"pause_during_off_hours"`
}

type CampaignStats struct {
        TotalQueued       int64     `json:"total_queued"`
        TotalSent         int64     `json:"total_sent"`
        TotalDelivered    int64     `json:"total_delivered"`
        TotalFailed       int64     `json:"total_failed"`
        TotalBounced      int64     `json:"total_bounced"`
        HardBounces       int64     `json:"hard_bounces"`
        SoftBounces       int64     `json:"soft_bounces"`
        TotalOpens        int64     `json:"total_opens"`
        UniqueOpens       int64     `json:"unique_opens"`
        TotalClicks       int64     `json:"total_clicks"`
        UniqueClicks      int64     `json:"unique_clicks"`
        TotalComplaints   int64     `json:"total_complaints"`
        TotalUnsubscribes int64     `json:"total_unsubscribes"`
        AverageOpenRate   float64   `json:"average_open_rate"`
        AverageClickRate  float64   `json:"average_click_rate"`
        DeliveryRate      float64   `json:"delivery_rate"`
        BounceRate        float64   `json:"bounce_rate"`
        ComplaintRate     float64   `json:"complaint_rate"`
        UnsubscribeRate   float64   `json:"unsubscribe_rate"`
        ThroughputPerMin  float64   `json:"throughput_per_min"`
        AverageLatencyMs  float64   `json:"average_latency_ms"`
        P95LatencyMs      float64   `json:"p95_latency_ms"`
        P99LatencyMs      float64   `json:"p99_latency_ms"`
        LastSentAt        *time.Time `json:"last_sent_at,omitempty"`
        LastUpdatedAt     time.Time `json:"last_updated_at"`
}

type CampaignProgress struct {
        CurrentPosition      int64      `json:"current_position"`
        TotalRecipients      int64      `json:"total_recipients"`
        ProcessedRecipients  int64      `json:"processed_recipients"`
        RemainingRecipients  int64      `json:"remaining_recipients"`
        ProgressPercentage   float64    `json:"progress_percentage"`
        EstimatedTimeRemaining int64    `json:"estimated_time_remaining_seconds"`
        EstimatedCompletionTime *time.Time `json:"estimated_completion_time,omitempty"`
        CurrentBatch         int        `json:"current_batch"`
        TotalBatches         int        `json:"total_batches"`
        CurrentAccountID     string     `json:"current_account_id,omitempty"`
        CurrentTemplateID    string     `json:"current_template_id,omitempty"`
        CurrentProxyID       string     `json:"current_proxy_id,omitempty"`
        ActiveWorkers        int        `json:"active_workers"`
        QueueSize            int        `json:"queue_size"`
}

func NewCampaign(name, tenantID, createdBy string) *Campaign {
        now := time.Now()
        return &Campaign{
                ID:          generateID(),
                SessionID:   generateSessionID(),
                TenantID:    tenantID,
                Name:        name,
                Status:      CampaignStatusCreated,
                Priority:    CampaignPriorityNormal,
                Type:        CampaignTypeOneTime,
                Tags:        []string{},
                AccountIDs:  []string{},
                TemplateIDs: []string{},
                ProxyIDs:    []string{},
                Config:      DefaultCampaignConfig(),
                RotationConfig: DefaultRotationConfig(),
                CampaignRateLimitSettings: DefaultRateLimitConfig(),
                CampaignRetrySettings: DefaultRetryConfig(),
                CampaignNotificationSettings: DefaultNotificationConfig(),
                Stats:       CampaignStats{},
                Progress:    CampaignProgress{},
                StateData:   make(map[string]interface{}),
                CreatedAt:   now,
                UpdatedAt:   now,
                CreatedBy:   createdBy,
                UpdatedBy:   createdBy,
                Version:     1,
        }
}

func DefaultCampaignConfig() CampaignRuntimeConfig {
        return CampaignRuntimeConfig{
                WorkerCount:            2,
                BatchSize:              100,
                DelayBetweenBatchesMs:  1000,
                DelayBetweenEmailsMs:   500,
                MaxRetriesPerEmail:     3,
                EnableProxy:            false,
                EnableAccountRotation:  true,
                EnableTemplateRotation: false,
                EnableAttachments:      false,
                EnableTracking:         true,
                EnableUnsubscribe:      true,
                EnableDKIM:             false,
                TrackOpens:             true,
                TrackClicks:            true,
                UseHTMLContent:         true,
                UsePlainTextFallback:   true,
                PersonalizationEnabled: true,
                SpamCheckEnabled:       true,
                TestMode:               false,
                CustomHeaders:          make(map[string]string),
                Metadata:               make(map[string]interface{}),
        }
}

func DefaultRotationConfig() RotationConfig {
        return RotationConfig{
                AccountRotationStrategy:    RotationStrategyHealthBased,
                TemplateRotationStrategy:   RotationStrategySequential,
                SenderNameRotationStrategy: RotationStrategyRandom,
                SubjectRotationStrategy:    RotationStrategyRandom,
                ProxyRotationStrategy:      RotationStrategyRandom,
                AccountRotationLimit:       100,
                TemplateRotationLimit:      0,
                EnableUnlimitedRotation:    true,
                RotateOnError:              true,
                RotateOnSpamDetection:      true,
                CooldownPeriodSeconds:      300,
        }
}

func DefaultRateLimitConfig() CampaignRateLimitSettings {
        return CampaignRateLimitSettings{
                GlobalRequestsPerSecond:    10,
                GlobalRequestsPerMinute:    500,
                GlobalRequestsPerHour:      10000,
                AccountRequestsPerSecond:   2,
                AccountRequestsPerMinute:   100,
                AccountRequestsPerHour:     2000,
                AccountDailyLimit:          5000,
                EnableAdaptiveRateLimiting: true,
                EnableDistributedLimiter:   false,
        }
}

func DefaultRetryConfig() CampaignRetrySettings {
        return CampaignRetrySettings{
                MaxRetries:               3,
                InitialBackoffMs:         1000,
                MaxBackoffMs:             30000,
                BackoffMultiplier:        2.0,
                EnableExponentialBackoff: true,
                RetryableErrorCodes:      []string{"RATE_LIMIT", "TIMEOUT", "NETWORK_ERROR", "TEMPORARY_FAILURE"},
                FailOnMaxRetries:         false,
        }
}

func DefaultNotificationConfig() CampaignNotificationSettings {
        return CampaignNotificationSettings{
                EnableNotifications:  true,
                NotifyOnStart:        true,
                NotifyOnComplete:     true,
                NotifyOnFailure:      true,
                NotifyOnPause:        false,
                NotifyOnMilestone:    true,
                MilestonePercentages: []int{25, 50, 75},
                TelegramEnabled:      false,
                EmailEnabled:         false,
                WebhookEnabled:       false,
        }
}

func (c *Campaign) Validate() error {
        if c.Name == "" {
                return errors.New("campaign name is required")
        }
        if c.TenantID == "" {
                return errors.New("tenant ID is required")
        }
        if len(c.AccountIDs) == 0 {
                return errors.New("at least one account is required")
        }
        if len(c.TemplateIDs) == 0 {
                return errors.New("at least one template is required")
        }
        if c.RecipientGroupID == "" && c.TotalRecipients == 0 {
                return errors.New("recipient group or recipient count is required")
        }
        if c.Config.WorkerCount < 1 || c.Config.WorkerCount > 10 {
                return errors.New("worker count must be between 1 and 10")
        }
        if c.Config.BatchSize < 1 || c.Config.BatchSize > 1000 {
                return errors.New("batch size must be between 1 and 1000")
        }
        return nil
}

func (c *Campaign) CanStart() bool {
        return c.Status == CampaignStatusCreated || c.Status == CampaignStatusScheduled
}

func (c *Campaign) CanPause() bool {
        return c.Status == CampaignStatusRunning
}

func (c *Campaign) CanResume() bool {
        return c.Status == CampaignStatusPaused
}

func (c *Campaign) CanStop() bool {
        return c.Status == CampaignStatusRunning || c.Status == CampaignStatusPaused
}

func (c *Campaign) CanDelete() bool {
        return c.Status != CampaignStatusRunning
}

func (c *Campaign) IsActive() bool {
        return c.Status == CampaignStatusRunning
}

func (c *Campaign) IsCompleted() bool {
        return c.Status == CampaignStatusCompleted || c.Status == CampaignStatusFailed || c.Status == CampaignStatusCancelled || c.Status == CampaignStatusStopped
}

func (c *Campaign) Start() error {
        if !c.CanStart() {
                return fmt.Errorf("cannot start campaign with status: %s", c.Status)
        }
        now := time.Now()
        c.Status = CampaignStatusRunning
        c.StartedAt = &now
        c.UpdatedAt = now
        return nil
}

func (c *Campaign) Pause() error {
        if !c.CanPause() {
                return fmt.Errorf("cannot pause campaign with status: %s", c.Status)
        }
        now := time.Now()
        c.Status = CampaignStatusPaused
        c.PausedAt = &now
        c.UpdatedAt = now
        return nil
}

func (c *Campaign) Resume() error {
        if !c.CanResume() {
                return fmt.Errorf("cannot resume campaign with status: %s", c.Status)
        }
        c.Status = CampaignStatusRunning
        c.PausedAt = nil
        c.UpdatedAt = time.Now()
        return nil
}

func (c *Campaign) Stop() error {
        if !c.CanStop() {
                return fmt.Errorf("cannot stop campaign with status: %s", c.Status)
        }
        now := time.Now()
        c.Status = CampaignStatusStopped
        c.CompletedAt = &now
        c.UpdatedAt = now
        return nil
}

func (c *Campaign) Complete() error {
        now := time.Now()
        c.Status = CampaignStatusCompleted
        c.CompletedAt = &now
        c.UpdatedAt = now
        c.Progress.ProgressPercentage = 100.0
        return nil
}

func (c *Campaign) Fail(errorMsg string) error {
        now := time.Now()
        c.Status = CampaignStatusFailed
        c.ErrorMessage = errorMsg
        c.CompletedAt = &now
        c.UpdatedAt = now
        return nil
}

func (c *Campaign) UpdateProgress(processed, total int64) {
        c.Progress.ProcessedRecipients = processed
        c.Progress.TotalRecipients = total
        c.Progress.RemainingRecipients = total - processed
        
        if total > 0 {
                c.Progress.ProgressPercentage = float64(processed) / float64(total) * 100.0
        }

        if c.StartedAt != nil && processed > 0 {
                elapsed := time.Since(*c.StartedAt).Seconds()
                rate := float64(processed) / elapsed
                if rate > 0 {
                        remaining := float64(total - processed) / rate
                        c.Progress.EstimatedTimeRemaining = int64(remaining)
                        eta := time.Now().Add(time.Duration(remaining) * time.Second)
                        c.Progress.EstimatedCompletionTime = &eta
                }
        }

        c.UpdatedAt = time.Now()
}

func (c *Campaign) UpdateStats(stats CampaignStats) {
        c.Stats = stats
        c.Stats.LastUpdatedAt = time.Now()
        c.UpdatedAt = time.Now()
        
        c.calculateRates()
}

func (c *Campaign) calculateRates() {
        total := c.Stats.TotalSent
        if total > 0 {
                c.Stats.DeliveryRate = float64(c.Stats.TotalDelivered) / float64(total)
                c.Stats.BounceRate = float64(c.Stats.TotalBounced) / float64(total)
                c.Stats.ComplaintRate = float64(c.Stats.TotalComplaints) / float64(total)
                c.Stats.UnsubscribeRate = float64(c.Stats.TotalUnsubscribes) / float64(total)
        }
        
        delivered := c.Stats.TotalDelivered
        if delivered > 0 {
                c.Stats.AverageOpenRate = float64(c.Stats.UniqueOpens) / float64(delivered)
                c.Stats.AverageClickRate = float64(c.Stats.UniqueClicks) / float64(delivered)
        }
}

func (c *Campaign) IncrementSent() {
        c.Stats.TotalSent++
        c.Stats.LastUpdatedAt = time.Now()
        c.UpdatedAt = time.Now()
}

func (c *Campaign) IncrementFailed() {
        c.Stats.TotalFailed++
        c.Stats.LastUpdatedAt = time.Now()
        c.UpdatedAt = time.Now()
}

func (c *Campaign) Checkpoint() {
        now := time.Now()
        c.LastCheckpointAt = &now
}

func generateID() string {
        return fmt.Sprintf("camp_%d", time.Now().UnixNano())
}

func generateSessionID() string {
        return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func (c *Campaign) Clone() *Campaign {
        clone := *c
        clone.ID = generateID()
        clone.SessionID = generateSessionID()
        clone.Status = CampaignStatusCreated
        clone.StartedAt = nil
        clone.CompletedAt = nil
        clone.PausedAt = nil
        clone.LastCheckpointAt = nil
        now := time.Now()
        clone.CreatedAt = now
        clone.UpdatedAt = now
        clone.Stats = CampaignStats{}
        clone.Progress = CampaignProgress{}
        clone.ErrorMessage = ""
        return &clone
}

func (s CampaignStatus) IsValid() bool {
        switch s {
        case CampaignStatusCreated, CampaignStatusScheduled, CampaignStatusRunning,
                CampaignStatusPaused, CampaignStatusCompleted, CampaignStatusFailed,
                CampaignStatusCancelled, CampaignStatusStopped:
                return true
        }
        return false
}

func (p CampaignPriority) IsValid() bool {
        switch p {
        case CampaignPriorityLow, CampaignPriorityNormal, CampaignPriorityHigh, CampaignPriorityCritical:
                return true
        }
        return false
}

func (t CampaignType) IsValid() bool {
        switch t {
        case CampaignTypeOneTime, CampaignTypeRecurring, CampaignTypeDrip, CampaignTypeAB:
                return true
        }
        return false
}
