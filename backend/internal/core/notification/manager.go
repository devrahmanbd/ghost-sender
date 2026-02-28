package notification

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"

    "email-campaign-system/internal/models"
    "email-campaign-system/pkg/logger"
)

var (
    ErrNotificationDisabled = errors.New("notifications are disabled")
    ErrInvalidChannel       = errors.New("invalid notification channel")
    ErrChannelNotConfigured = errors.New("notification channel not configured")
    ErrQueueFull            = errors.New("notification queue is full")
)

type NotificationChannel string

const (
    ChannelTelegram NotificationChannel = "telegram"
    ChannelEmail    NotificationChannel = "email"
    ChannelWebhook  NotificationChannel = "webhook"
    ChannelWebSocket NotificationChannel = "websocket"
)

type NotificationLevel string

const (
    LevelInfo     NotificationLevel = "info"
    LevelWarning  NotificationLevel = "warning"
    LevelError    NotificationLevel = "error"
    LevelCritical NotificationLevel = "critical"
    LevelSuccess  NotificationLevel = "success"
)

type EventType string

const (
    EventCampaignStarted   EventType = "campaign.started"
    EventCampaignPaused    EventType = "campaign.paused"
    EventCampaignResumed   EventType = "campaign.resumed"
    EventCampaignCompleted EventType = "campaign.completed"
    EventCampaignFailed    EventType = "campaign.failed"
    EventAccountSuspended  EventType = "account.suspended"
    EventAccountRestored   EventType = "account.restored"
    EventSendSuccess       EventType = "send.success"
    EventSendFailed        EventType = "send.failed"
    EventQuotaReached      EventType = "quota.reached"
    EventSystemError       EventType = "system.error"
    EventSystemWarning     EventType = "system.warning"
)

type NotificationManager struct {
    mu               sync.RWMutex
    config           *ManagerConfig
    channels         map[NotificationChannel]Channel
    dispatcher       *Dispatcher
    queue            *NotificationQueue
    templateManager  *TemplateManager
    formatter        *MessageFormatter
    metrics          *NotificationMetrics
    logger           logger.Logger
    enabled          bool
    eventFilters     map[EventType]bool
    rateLimiter      *NotificationRateLimiter
    shutdownCh       chan struct{}
    wg               sync.WaitGroup
}

type ManagerConfig struct {
    Enabled           bool
    DefaultChannel    NotificationChannel
    EnabledChannels   []NotificationChannel
    QueueSize         int
    WorkerCount       int
    RetryAttempts     int
    RetryDelay        time.Duration
    RateLimitPerMin   int
    EnableRateLimit   bool
    EnableFiltering   bool
    MinLevel          NotificationLevel
    BatchSize         int
    FlushInterval     time.Duration
}

type Channel interface {
    Send(ctx context.Context, notification *Notification) error
    SendBatch(ctx context.Context, notifications []*Notification) error
    IsConfigured() bool
    TestConnection() error
    GetStats() ChannelStats
}

type Notification struct {
    ID        string
    Channel   NotificationChannel
    Level     NotificationLevel
    Event     EventType
    Title     string
    Message   string
    Data      map[string]interface{}
    Timestamp time.Time
    Retries   int
    MaxRetries int
    Priority  int
}

type NotificationMetrics struct {
    mu                sync.RWMutex
    TotalSent         int64
    TotalFailed       int64
    ByChannel         map[NotificationChannel]int64
    ByLevel           map[NotificationLevel]int64
    ByEvent           map[EventType]int64
    AverageLatency    time.Duration
    LastNotification  time.Time
}

type ChannelStats struct {
    Sent         int64
    Failed       int64
    Pending      int64
    LastSent     time.Time
    AverageDelay time.Duration
}

type NotificationRateLimiter struct {
    mu          sync.Mutex
    tokens      int
    maxTokens   int
    refillRate  time.Duration
    lastRefill  time.Time
}

func NewNotificationManager(config *ManagerConfig, log logger.Logger) *NotificationManager {
    if config == nil {
        config = DefaultManagerConfig()
    }

    nm := &NotificationManager{
        config:       config,
        channels:     make(map[NotificationChannel]Channel),
        enabled:      config.Enabled,
        eventFilters: make(map[EventType]bool),
        metrics:      &NotificationMetrics{
            ByChannel: make(map[NotificationChannel]int64),
            ByLevel:   make(map[NotificationLevel]int64),
            ByEvent:   make(map[EventType]int64),
        },
        logger:     log,
        shutdownCh: make(chan struct{}),
    }

    nm.queue = NewNotificationQueue(config.QueueSize)
    nm.dispatcher = NewDispatcher(nm)
    nm.templateManager = NewTemplateManager()
    nm.formatter = NewMessageFormatter()

    if config.EnableRateLimit {
        nm.rateLimiter = NewNotificationRateLimiter(config.RateLimitPerMin)
    }

    return nm
}

func DefaultManagerConfig() *ManagerConfig {
    return &ManagerConfig{
        Enabled:         true,
        DefaultChannel:  ChannelTelegram,
        EnabledChannels: []NotificationChannel{ChannelTelegram},
        QueueSize:       1000,
        WorkerCount:     5,
        RetryAttempts:   3,
        RetryDelay:      5 * time.Second,
        RateLimitPerMin: 20,
        EnableRateLimit: true,
        EnableFiltering: true,
        MinLevel:        LevelInfo,
        BatchSize:       10,
        FlushInterval:   30 * time.Second,
    }
}

func (nm *NotificationManager) Start(ctx context.Context) error {
    if !nm.enabled {
        return ErrNotificationDisabled
    }

    nm.logger.Info("starting notification manager")

    for i := 0; i < nm.config.WorkerCount; i++ {
        nm.wg.Add(1)
        go nm.worker(ctx, i)
    }

    nm.wg.Add(1)
    go nm.rateLimiterRefiller(ctx)

    return nil
}

func (nm *NotificationManager) Stop(ctx context.Context) error {
    nm.logger.Info("stopping notification manager")

    close(nm.shutdownCh)
    nm.wg.Wait()

    if err := nm.queue.Close(); err != nil {
        nm.logger.Error("failed to close notification queue", logger.Error(err))
    }

    return nil
}

func (nm *NotificationManager) worker(ctx context.Context, id int) {
    defer nm.wg.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case <-nm.shutdownCh:
            return
        default:
            notification, err := nm.queue.Dequeue(ctx, time.Second)
            if err != nil {
                if err != context.DeadlineExceeded {
                    nm.logger.Error("worker dequeue error", logger.Int("worker_id", id), logger.Error(err))
                }
                continue
            }

            if notification == nil {
                continue
            }

            if err := nm.processNotification(ctx, notification); err != nil {
                nm.handleSendError(ctx, notification, err)
            }
        }
    }
}

func (nm *NotificationManager) processNotification(ctx context.Context, notification *Notification) error {
    if nm.rateLimiter != nil && !nm.rateLimiter.Allow() {
        return nm.queue.Enqueue(notification)
    }

    channel, exists := nm.channels[notification.Channel]
    if !exists {
        return fmt.Errorf("%w: %s", ErrInvalidChannel, notification.Channel)
    }

    if !channel.IsConfigured() {
        return fmt.Errorf("%w: %s", ErrChannelNotConfigured, notification.Channel)
    }

    startTime := time.Now()

    if err := channel.Send(ctx, notification); err != nil {
        nm.recordFailure(notification)
        return err
    }

    nm.recordSuccess(notification, time.Since(startTime))
    return nil
}

func (nm *NotificationManager) handleSendError(ctx context.Context, notification *Notification, err error) {
    notification.Retries++

    if notification.Retries < notification.MaxRetries {
        nm.logger.Warn("notification send failed, retrying",
            logger.String("notification_id", notification.ID),
            logger.Int("retry", notification.Retries),
            logger.Error(err))

        time.Sleep(nm.config.RetryDelay)

        if err := nm.queue.Enqueue(notification); err != nil {
            nm.logger.Error("failed to requeue notification", logger.Error(err))
        }
    } else {
        nm.logger.Error("notification send failed after max retries",
            logger.String("notification_id", notification.ID),
            logger.Int("retries", notification.Retries),
            logger.Error(err))
    }
}

func (nm *NotificationManager) RegisterChannel(channelType NotificationChannel, channel Channel) error {
    if channel == nil {
        return errors.New("channel is nil")
    }

    nm.mu.Lock()
    defer nm.mu.Unlock()

    nm.channels[channelType] = channel
    nm.logger.Info("registered notification channel", logger.String("channel", string(channelType)))

    return nil
}

func (nm *NotificationManager) UnregisterChannel(channelType NotificationChannel) {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    delete(nm.channels, channelType)
    nm.logger.Info("unregistered notification channel", logger.String("channel", string(channelType)))
}

func (nm *NotificationManager) Send(ctx context.Context, notification *Notification) error {
    if !nm.enabled {
        return ErrNotificationDisabled
    }

    if notification == nil {
        return errors.New("notification is nil")
    }

    if notification.Channel == "" {
        notification.Channel = nm.config.DefaultChannel
    }

    if notification.Timestamp.IsZero() {
        notification.Timestamp = time.Now()
    }

    if notification.ID == "" {
        notification.ID = generateNotificationID()
    }

    notification.MaxRetries = nm.config.RetryAttempts

    if nm.config.EnableFiltering && !nm.shouldSend(notification) {
        return nil
    }

    return nm.queue.Enqueue(notification)
}

func (nm *NotificationManager) SendBatch(ctx context.Context, notifications []*Notification) error {
    if !nm.enabled {
        return ErrNotificationDisabled
    }

    for _, notification := range notifications {
        if err := nm.Send(ctx, notification); err != nil {
            nm.logger.Error("failed to send notification in batch", logger.Error(err))
        }
    }

    return nil
}
func (nm *NotificationManager) NotifyCampaignStarted(ctx context.Context, campaign *models.Campaign) error {
    message := fmt.Sprintf("Campaign '%s' has started at %s", campaign.Name, time.Now().Format(time.RFC1123))

    return nm.Send(ctx, &Notification{
        Level:   LevelInfo,
        Event:   EventCampaignStarted,
        Title:   "Campaign Started",
        Message: message,
        Data: map[string]interface{}{
            "campaign_id": campaign.ID,
        },
    })
}

func (nm *NotificationManager) NotifyCampaignCompleted(ctx context.Context, campaign *models.Campaign, stats *models.CampaignStats) error {
    var duration time.Duration
    if campaign.StartedAt != nil {
        duration = time.Since(*campaign.StartedAt)
    }
    
    successRate := calculateSuccessRate(stats.TotalSent, stats.TotalFailed)
    
    message := fmt.Sprintf(
        "Campaign '%s' completed successfully\n"+
        "Sent: %d\n"+
        "Failed: %d\n"+
        "Success Rate: %.2f%%\n"+
        "Duration: %s",
        campaign.Name,
        stats.TotalSent,
        stats.TotalFailed,
        successRate,
        duration.Round(time.Second),
    )

    return nm.Send(ctx, &Notification{
        Level:   LevelSuccess,
        Event:   EventCampaignCompleted,
        Title:   "Campaign Completed",
        Message: message,
        Data: map[string]interface{}{
            "campaign_id":  campaign.ID,
            "stats":        stats,
            "success_rate": successRate,
        },
    })
}

func (nm *NotificationManager) NotifyCampaignFailed(ctx context.Context, campaign *models.Campaign, reason error) error {
    message := fmt.Sprintf(
        "Campaign '%s' failed at %s\n"+
        "Error: %s",
        campaign.Name,
        time.Now().Format(time.RFC1123),
        reason.Error(),
    )

    return nm.Send(ctx, &Notification{
        Level:   LevelError,
        Event:   EventCampaignFailed,
        Title:   "Campaign Failed",
        Message: message,
        Data: map[string]interface{}{
            "campaign_id": campaign.ID,
            "error":       reason.Error(),
        },
        Priority: 10,
    })
}

func (nm *NotificationManager) NotifyAccountSuspended(ctx context.Context, account *models.Account, reason string) error {
    message := fmt.Sprintf(
        "Account '%s' has been suspended at %s\n"+
        "Reason: %s",
        account.Email,
        time.Now().Format(time.RFC1123),
        reason,
    )

    return nm.Send(ctx, &Notification{
        Level:   LevelWarning,
        Event:   EventAccountSuspended,
        Title:   "Account Suspended",
        Message: message,
        Data: map[string]interface{}{
            "account_id": account.ID,
            "reason":     reason,
        },
        Priority: 8,
    })
}


func (nm *NotificationManager) shouldSend(notification *Notification) bool {
    if nm.eventFilters != nil {
        if filtered, exists := nm.eventFilters[notification.Event]; exists && filtered {
            return false
        }
    }

    levelPriority := map[NotificationLevel]int{
        LevelInfo:     1,
        LevelSuccess:  2,
        LevelWarning:  3,
        LevelError:    4,
        LevelCritical: 5,
    }

    if levelPriority[notification.Level] < levelPriority[nm.config.MinLevel] {
        return false
    }

    return true
}

func (nm *NotificationManager) FilterEvent(event EventType, filter bool) {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    nm.eventFilters[event] = filter
}

func (nm *NotificationManager) Enable() {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    nm.enabled = true
    nm.logger.Info("notifications enabled")
}

func (nm *NotificationManager) Disable() {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    nm.enabled = false
    nm.logger.Info("notifications disabled")
}

func (nm *NotificationManager) IsEnabled() bool {
    nm.mu.RLock()
    defer nm.mu.RUnlock()

    return nm.enabled
}

func (nm *NotificationManager) GetMetrics() *NotificationMetrics {
    nm.metrics.mu.RLock()
    defer nm.metrics.mu.RUnlock()

    return &NotificationMetrics{
        TotalSent:        nm.metrics.TotalSent,
        TotalFailed:      nm.metrics.TotalFailed,
        ByChannel:        copyInt64Map(nm.metrics.ByChannel),
        ByLevel:          copyInt64Map(nm.metrics.ByLevel),
        ByEvent:          copyInt64Map(nm.metrics.ByEvent),
        AverageLatency:   nm.metrics.AverageLatency,
        LastNotification: nm.metrics.LastNotification,
    }
}

func (nm *NotificationManager) recordSuccess(notification *Notification, latency time.Duration) {
    nm.metrics.mu.Lock()
    defer nm.metrics.mu.Unlock()

    nm.metrics.TotalSent++
    nm.metrics.ByChannel[notification.Channel]++
    nm.metrics.ByLevel[notification.Level]++
    nm.metrics.ByEvent[notification.Event]++
    nm.metrics.LastNotification = time.Now()

    if nm.metrics.AverageLatency == 0 {
        nm.metrics.AverageLatency = latency
    } else {
        nm.metrics.AverageLatency = (nm.metrics.AverageLatency + latency) / 2
    }
}

func (nm *NotificationManager) recordFailure(notification *Notification) {
    nm.metrics.mu.Lock()
    defer nm.metrics.mu.Unlock()

    nm.metrics.TotalFailed++
}

func (nm *NotificationManager) rateLimiterRefiller(ctx context.Context) {
    defer nm.wg.Done()

    if nm.rateLimiter == nil {
        return
    }

    ticker := time.NewTicker(nm.rateLimiter.refillRate)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-nm.shutdownCh:
            return
        case <-ticker.C:
            nm.rateLimiter.Refill()
        }
    }
}

func NewNotificationRateLimiter(tokensPerMinute int) *NotificationRateLimiter {
    return &NotificationRateLimiter{
        tokens:     tokensPerMinute,
        maxTokens:  tokensPerMinute,
        refillRate: time.Minute / time.Duration(tokensPerMinute),
        lastRefill: time.Now(),
    }
}

func (nrl *NotificationRateLimiter) Allow() bool {
    nrl.mu.Lock()
    defer nrl.mu.Unlock()

    if nrl.tokens > 0 {
        nrl.tokens--
        return true
    }

    return false
}

func (nrl *NotificationRateLimiter) Refill() {
    nrl.mu.Lock()
    defer nrl.mu.Unlock()

    if nrl.tokens < nrl.maxTokens {
        nrl.tokens++
    }
    nrl.lastRefill = time.Now()
}

func generateNotificationID() string {
    return fmt.Sprintf("notif_%d", time.Now().UnixNano())
}

func copyInt64Map[K comparable](src map[K]int64) map[K]int64 {
    dst := make(map[K]int64, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func calculateSuccessRate(sent, failed int64) float64 {
    if sent == 0 {
        return 0.0
    }
    return float64(sent-failed) / float64(sent) * 100.0
}
