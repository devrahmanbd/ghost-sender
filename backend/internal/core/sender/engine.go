package sender

import (
        "context"
        "errors"
        "fmt"
        "runtime"
        "strings"
        "sync"
        "sync/atomic"
        "time"

        "email-campaign-system/internal/core/account"
        "email-campaign-system/internal/core/attachment"
        "email-campaign-system/internal/core/personalization"
        "email-campaign-system/internal/core/proxy"
        "email-campaign-system/internal/models"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/logger"
)

var (
        ErrEngineNotRunning = errors.New("engine is not running")
        ErrEngineRunning    = errors.New("engine is already running")
        ErrNoCampaign       = errors.New("no campaign set")
        ErrNoAccounts       = errors.New("no accounts available")
        ErrNoRecipients     = errors.New("no recipients to send")
        ErrEngineStopped    = errors.New("engine has been stopped")
)

type TemplateManager interface {
        GetNextTemplate(ctx context.Context, templateIDs []string) (*models.Template, error)
        Render(ctx context.Context, template *models.Template, data map[string]interface{}) (string, error)
}

type DeliverabilityManager interface {
        BuildMessage(ctx context.Context, req *MessageRequest) (*EmailMessage, error)
}

type MessageRequest struct {
        From        string
        FromName    string
        To          string
        ToName      string
        Subject     string
        HTMLBody    string
        Attachments []*models.Attachment
        CampaignID  string
}

type EmailMessage struct {
        From          string
        FromName      string
        To            string
        ToName        string
        Subject       string
        Body          string
        Attachments   []*models.Attachment
        ProxyHost     string
        ProxyPort     int
        ProxyType     string
        ProxyUsername  string
        ProxyPassword string
}

type ProviderFactory interface {
        GetProvider(ctx context.Context, account *models.Account) (EmailProvider, error)
}

type EmailProvider interface {
        Send(ctx context.Context, message *EmailMessage) (string, error)
        Name() string
}

type RateLimiter interface {
        Wait(ctx context.Context) error
}

type Engine struct {
        campaign          *models.Campaign
        accountManager    *account.Manager
        templateManager   TemplateManager
        attachmentManager *attachment.Manager
        personalization   *personalization.Manager
        deliverability    DeliverabilityManager
        providerFactory   ProviderFactory
        rateLimiter       RateLimiter
        proxyManager      *proxy.ProxyManager
        queue             *Queue
        workerPool        *WorkerPool
        batchProcessor    *BatchProcessor
        retryHandler      *RetryHandler
        logRepo           repository.LogRepository
        statsRepo         repository.StatsRepository
        log               logger.Logger
        config            EngineConfig
        accounts          []models.Account
        accountIdx        atomic.Int64
        // Attachment slot rotation: each recipient gets exactly one (template, format) pair.
        attachSlots    []attachment.AttachmentSlot
        attachSlotIdx  atomic.Int64
        running           atomic.Bool
        paused            atomic.Bool
        stopChan          chan struct{}
        pauseChan         chan struct{}
        resumeChan        chan struct{}
        mu                sync.RWMutex
        stats             *EngineStats
        progressCallbacks []ProgressCallback
        eventCallbacks    []EventCallback
        startTime         time.Time
}

type EngineConfig struct {
        WorkerCount           int
        QueueSize             int
        BatchSize             int
        RateLimit             float64
        MaxRetries            int
        RetryDelay            time.Duration
        SendTimeout           time.Duration
        EnableRateLimiting    bool
        EnableAdaptiveRate    bool
        EnableConnectionPool  bool
        EnableBatchProcessing bool
        EnableProxyPerMail    bool
        EnableSmartSending    bool
        StatsUpdateInterval   time.Duration
        ProgressInterval      time.Duration
}

type EngineStats struct {
        TotalEmails       int64
        SentCount         int64
        FailedCount       int64
        QueuedCount       int64
        ProcessingCount   int64
        SkippedCount      int64
        RetryCount        int64
        BytesSent         int64
        EmailsPerSecond   float64
        AverageSendTime   time.Duration
        EstimatedTimeLeft time.Duration
        StartTime         time.Time
        LastUpdateTime    time.Time
        mu                sync.RWMutex
}

type SendJob struct {
        ID                    string
        CampaignID            string
        Recipient             *models.Recipient
        Template              *models.Template
        Account               *models.Account
        Attachments           []*models.Attachment
        Priority              int
        Retries               int
        MaxRetries            int
        CreatedAt             time.Time
        ScheduledAt           time.Time
        Metadata              map[string]interface{}
        EnableAttachments     bool
        AttachmentTemplateIDs []string
        AttachmentFormat      string
        EnableSmartSending    bool
        Campaign              *models.Campaign
}

type SendResult struct {
        Job          *SendJob
        Success      bool
        Error        error
        MessageID    string
        SentAt       time.Time
        Duration     time.Duration
        BytesSent    int64
        AccountUsed  string
        ProviderUsed string
        ProxyUsed    string
        RetryAttempt int
}

type ProgressCallback func(stats *EngineStats)
type EventCallback func(event *SendEvent)

type SendEvent struct {
        Type      SendEventType
        Job       *SendJob
        Result    *SendResult
        Timestamp time.Time
        Message   string
}

type SendEventType string

const (
        EventTypeQueued     SendEventType = "queued"
        EventTypeProcessing SendEventType = "processing"
        EventTypeSent       SendEventType = "sent"
        EventTypeFailed     SendEventType = "failed"
        EventTypeRetrying   SendEventType = "retrying"
        EventTypeSkipped    SendEventType = "skipped"
)

func NewEngine(
        accountManager *account.Manager,
        templateManager TemplateManager,
        attachmentManager *attachment.Manager,
        personalization *personalization.Manager,
        deliverability DeliverabilityManager,
        providerFactory ProviderFactory,
        rateLimiter RateLimiter,
        logRepo repository.LogRepository,
        statsRepo repository.StatsRepository,
        log logger.Logger,
        config EngineConfig,
) *Engine {
        if config.WorkerCount <= 0 {
                config.WorkerCount = runtime.NumCPU()
                if config.WorkerCount < 1 {
                        config.WorkerCount = 2
                }
        }
        if config.QueueSize <= 0 {
                config.QueueSize = 1000
        }
        if config.BatchSize <= 0 {
                config.BatchSize = 100
        }
        if config.MaxRetries <= 0 {
                config.MaxRetries = 3
        }
        if config.RetryDelay <= 0 {
                config.RetryDelay = 5 * time.Second
        }
        if config.SendTimeout <= 0 {
                config.SendTimeout = 30 * time.Second
        }
        if config.StatsUpdateInterval <= 0 {
                config.StatsUpdateInterval = 5 * time.Second
        }
        if config.ProgressInterval <= 0 {
                config.ProgressInterval = 1 * time.Second
        }

        e := &Engine{
                accountManager:    accountManager,
                templateManager:   templateManager,
                attachmentManager: attachmentManager,
                personalization:   personalization,
                deliverability:    deliverability,
                providerFactory:   providerFactory,
                rateLimiter:       rateLimiter,
                logRepo:           logRepo,
                statsRepo:         statsRepo,
                log:               log,
                config:            config,
                stopChan:          make(chan struct{}),
                pauseChan:         make(chan struct{}),
                resumeChan:        make(chan struct{}),
                stats:             &EngineStats{},
                progressCallbacks: make([]ProgressCallback, 0),
                eventCallbacks:    make([]EventCallback, 0),
        }

        queueConfig := QueueConfig{
                MaxSize:         config.QueueSize,
                PriorityEnabled: true,
                BlockOnFull:     false,
        }
        e.queue = NewQueue(queueConfig, log)

        workerConfig := WorkerPoolConfig{
                WorkerCount:    config.WorkerCount,
                JobTimeout:     config.SendTimeout,
                IdleTimeout:    30 * time.Second,
                EnableMetrics:  true,
                JobChannelSize: config.QueueSize,
        }
        e.workerPool = NewWorkerPool(workerConfig, log)

        batchConfig := BatchConfig{
                BatchSize:     config.BatchSize,
                FlushInterval: 10 * time.Second,
                MaxWaitTime:   30 * time.Second,
        }
        e.batchProcessor = NewBatchProcessor(batchConfig, log)

        retryConfig := RetryConfig{
                MaxRetries:   config.MaxRetries,
                InitialDelay: config.RetryDelay,
                MaxDelay:     5 * time.Minute,
                Multiplier:   2.0,
                EnableJitter: true,
        }
        e.retryHandler = NewRetryHandler(retryConfig, log)

        return e
}

func (e *Engine) SetCampaign(campaign *models.Campaign) {
        e.mu.Lock()
        e.campaign = campaign
        e.mu.Unlock()
        e.buildAttachmentSlots()
}

// buildAttachmentSlots pre-computes the flattened (templateID, format) rotation list
// for the current campaign. Called whenever a new campaign is set.
func (e *Engine) buildAttachmentSlots() {
        if e.attachmentManager == nil || e.campaign == nil {
                e.attachSlots = nil
                return
        }
        if !e.campaign.Config.EnableAttachments {
                e.attachSlots = nil
                return
        }

        var templateIDs []string
        if meta := e.campaign.Config.Metadata; meta != nil {
                if raw, ok := meta["attachment_template_ids"]; ok {
                        switch v := raw.(type) {
                        case []string:
                                for _, s := range v {
                                        templateIDs = append(templateIDs, strings.TrimSuffix(strings.TrimSuffix(s, ".html"), ".htm"))
                                }
                        case []interface{}:
                                for _, item := range v {
                                        if s, ok := item.(string); ok {
                                                templateIDs = append(templateIDs, strings.TrimSuffix(strings.TrimSuffix(s, ".html"), ".htm"))
                                        }
                                }
                        }
                }
        }

        if len(templateIDs) == 0 {
                e.attachSlots = nil
                return
        }

        slots := e.attachmentManager.GetAttachmentSlots(templateIDs)
        e.attachSlots = slots
        e.attachSlotIdx.Store(0)
        e.log.Info(fmt.Sprintf("[ATTACH-SLOTS] built %d rotation slots for campaign %s: %v",
                len(slots), e.campaign.ID, func() []string {
                        s := make([]string, len(slots))
                        for i, sl := range slots {
                                s[i] = sl.TemplateID + "." + string(sl.Format)
                        }
                        return s
                }()))
}
func (e *Engine) Start(ctx context.Context) error {
    if e.running.Load() {
        return ErrEngineRunning
    }

    e.mu.Lock()
    if e.campaign == nil {
        e.mu.Unlock()
        return ErrNoCampaign
    }

    e.stopChan = make(chan struct{})
    e.pauseChan = make(chan struct{})
    e.resumeChan = make(chan struct{})
    e.mu.Unlock()

    e.running.Store(true)
    e.paused.Store(false)
    e.startTime = time.Now()

    e.stats.mu.Lock()
    e.stats.StartTime = time.Now()
    e.stats.mu.Unlock()

    if err := e.workerPool.Start(ctx, e.handleJob); err != nil {
        return fmt.Errorf("failed to start worker pool: %w", err)
    }

    go e.statsUpdateLoop(ctx)
    go e.progressUpdateLoop(ctx)
    go e.monitorLoop(ctx)

    e.log.Info(fmt.Sprintf("engine started: campaign_id=%s, workers=%d",
        e.campaign.ID, e.config.WorkerCount))

    return nil
}


func (e *Engine) Stop(ctx context.Context) error {
    if !e.running.Load() {
        return ErrEngineNotRunning
    }

    e.running.Store(false)


    select {
    case <-e.stopChan:
        // already closed
    default:
        close(e.stopChan)
    }

    e.queue.Close()

    if err := e.workerPool.Stop(ctx); err != nil {
        e.log.Error(fmt.Sprintf("error stopping worker pool: %v", err))
    }

    e.log.Info(fmt.Sprintf("engine stopped: campaign_id=%s, sent=%d, failed=%d, duration=%v",
        e.campaign.ID, e.stats.SentCount, e.stats.FailedCount, time.Since(e.startTime)))

    return nil
}


func (e *Engine) Pause(ctx context.Context) error {
        if !e.running.Load() {
                return ErrEngineNotRunning
        }

        if e.paused.Load() {
                return errors.New("engine already paused")
        }

        e.paused.Store(true)
        close(e.pauseChan)

        e.log.Info(fmt.Sprintf("engine paused: campaign_id=%s", e.campaign.ID))

        return nil
}

func (e *Engine) Resume(ctx context.Context) error {
        if !e.running.Load() {
                return ErrEngineNotRunning
        }

        if !e.paused.Load() {
                return errors.New("engine not paused")
        }

        e.paused.Store(false)

        if !e.workerPool.IsRunning() {
                e.log.Info("worker pool not running, restarting for resume")
                if err := e.workerPool.Start(ctx, e.handleJob); err != nil {
                        return fmt.Errorf("failed to restart worker pool on resume: %w", err)
                }
        }

        e.resumeChan = make(chan struct{})

        e.log.Info(fmt.Sprintf("engine resumed: campaign_id=%s", e.campaign.ID))

        return nil
}
func (e *Engine) SetAccounts(accounts []models.Account) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.accounts = accounts
    e.accountIdx.Store(0)
}

func (e *Engine) SetProxyManager(pm *proxy.ProxyManager) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.proxyManager = pm
}

func (e *Engine) SetSmartSending(enabled bool) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.config.EnableSmartSending = enabled
}
func (e *Engine) QueueEmail(ctx context.Context, recipient *models.Recipient, priority int) error {
        if !e.running.Load() {
                return ErrEngineNotRunning
        }

        campaignSnapshot := *e.campaign
        campaignSnapshot.Config = e.campaign.Config
        campaignSnapshot.TemplateIDs = append([]string(nil), e.campaign.TemplateIDs...)

        // Pick exactly one (templateID, format) slot for this recipient via round-robin.
        // This ensures every recipient gets a single attachment file, and the combination
        // rotates across all (template × format) pairs to maximise inbox-ratio diversity.
        var attachTplIDs []string
        var attachFormat string
        if e.campaign.Config.EnableAttachments && len(e.attachSlots) > 0 {
                idx := e.attachSlotIdx.Add(1) - 1
                slot := e.attachSlots[idx%int64(len(e.attachSlots))]
                attachTplIDs = []string{slot.TemplateID}
                attachFormat = string(slot.Format)
                e.log.Debug(fmt.Sprintf("[QUEUE-DEBUG] slot picked: template=%s format=%s (slot %d/%d)",
                        slot.TemplateID, slot.Format, idx%int64(len(e.attachSlots))+1, len(e.attachSlots)))
        } else if e.campaign.Config.EnableAttachments {
                // Fallback: no pre-built slots (attachment manager unavailable at SetCampaign time).
                // Read template IDs from metadata directly.
                if meta := e.campaign.Config.Metadata; meta != nil {
                        if raw, ok := meta["attachment_template_ids"]; ok {
                                switch v := raw.(type) {
                                case []string:
                                        for _, s := range v {
                                                attachTplIDs = append(attachTplIDs, strings.TrimSuffix(strings.TrimSuffix(s, ".html"), ".htm"))
                                        }
                                case []interface{}:
                                        for _, item := range v {
                                                if s, ok := item.(string); ok {
                                                        attachTplIDs = append(attachTplIDs, strings.TrimSuffix(strings.TrimSuffix(s, ".html"), ".htm"))
                                                }
                                        }
                                }
                        }
                        if raw, ok := meta["attachment_format"]; ok {
                                if s, ok := raw.(string); ok && s != "" {
                                        attachFormat = strings.ToLower(s)
                                }
                        }
                }
        }

        e.log.Debug(fmt.Sprintf("[QUEUE-DEBUG] EnableAttachments=%v, attachTplIDs=%v, attachFormat=%s",
                e.campaign.Config.EnableAttachments, attachTplIDs, attachFormat))

        job := &SendJob{
                ID:                    fmt.Sprintf("%s-%s-%d", e.campaign.ID, recipient.Email, time.Now().UnixNano()),
                CampaignID:            e.campaign.ID,
                Recipient:             recipient,
                Priority:              priority,
                MaxRetries:            e.config.MaxRetries,
                CreatedAt:             time.Now(),
                ScheduledAt:           time.Now(),
                Metadata:              make(map[string]interface{}),
                EnableAttachments:     e.campaign.Config.EnableAttachments,
                AttachmentTemplateIDs: attachTplIDs,
                AttachmentFormat:      attachFormat,
                EnableSmartSending:    e.campaign.Config.SmartSending,
                Campaign:              &campaignSnapshot,
        }

        atomic.AddInt64(&e.stats.QueuedCount, 1)
        atomic.AddInt64(&e.stats.TotalEmails, 1)

        e.publishEvent(&SendEvent{
                Type:      EventTypeQueued,
                Job:       job,
                Timestamp: time.Now(),
        })

        e.log.Info(fmt.Sprintf("email queued for delivery: campaign_id=%s recipient=%s", job.CampaignID, recipient.Email))

        if err := e.workerPool.Submit(job); err != nil {
                atomic.AddInt64(&e.stats.QueuedCount, -1)
                atomic.AddInt64(&e.stats.TotalEmails, -1)
                return fmt.Errorf("failed to submit email to worker pool: %w", err)
        }

        return nil
}

func (e *Engine) QueueBatch(ctx context.Context, recipients []*models.Recipient, priority int) error {
        if !e.running.Load() {
                return ErrEngineNotRunning
        }

        for _, recipient := range recipients {
                if err := e.QueueEmail(ctx, recipient, priority); err != nil {
                        e.log.Error(fmt.Sprintf("failed to queue recipient %s: %v", recipient.Email, err))
                        continue
                }
        }

        return nil
}

func (e *Engine) handleJob(ctx context.Context, jobData interface{}) error {
        job, ok := jobData.(*SendJob)
        if !ok {
                return errors.New("invalid job type")
        }

        if e.paused.Load() {
                <-e.resumeChan
        }

        atomic.AddInt64(&e.stats.ProcessingCount, 1)
        atomic.AddInt64(&e.stats.QueuedCount, -1)

        e.publishEvent(&SendEvent{
                Type:      EventTypeProcessing,
                Job:       job,
                Timestamp: time.Now(),
        })

        result := e.processJob(ctx, job)

        atomic.AddInt64(&e.stats.ProcessingCount, -1)

        if result.Success {
                atomic.AddInt64(&e.stats.SentCount, 1)
                atomic.AddInt64(&e.stats.BytesSent, result.BytesSent)

                e.publishEvent(&SendEvent{
                        Type:      EventTypeSent,
                        Job:       job,
                        Result:    result,
                        Timestamp: time.Now(),
                })

                e.logSuccess(ctx, job, result)
        } else {
                if job.Retries < job.MaxRetries {
                        job.Retries++
                        atomic.AddInt64(&e.stats.RetryCount, 1)

                        e.publishEvent(&SendEvent{
                                Type:      EventTypeRetrying,
                                Job:       job,
                                Result:    result,
                                Timestamp: time.Now(),
                        })

                        e.retryHandler.Schedule(ctx, job, result.Error)
                } else {
                        atomic.AddInt64(&e.stats.FailedCount, 1)

                        e.publishEvent(&SendEvent{
                                Type:      EventTypeFailed,
                                Job:       job,
                                Result:    result,
                                Timestamp: time.Now(),
                        })

                        e.logFailure(ctx, job, result)
                }
        }

        return result.Error
}

func (e *Engine) processJob(ctx context.Context, job *SendJob) *SendResult {
        startTime := time.Now()

        result := &SendResult{
                Job:          job,
                Success:      false,
                SentAt:       time.Now(),
                RetryAttempt: job.Retries,
        }

        var selectedAccount *models.Account
        if job.EnableSmartSending && job.Recipient != nil {
                selectedAccount = e.getSmartAccount(job.Recipient.Email)
        } else {
                selectedAccount = e.getNextAccount()
        }
        if selectedAccount == nil {
                result.Error = fmt.Errorf("no accounts available for sending")
                result.Duration = time.Since(startTime)
                return result
        }
        
        job.Account = selectedAccount
        result.AccountUsed = selectedAccount.Email

        if e.config.EnableProxyPerMail && e.proxyManager != nil {
                assignedProxy, proxyErr := e.proxyManager.GetNextForMail()
                if proxyErr != nil {
                        e.log.Warn(fmt.Sprintf("no proxy available for mail to %s: %v", job.Recipient.Email, proxyErr))
                } else if assignedProxy != nil {
                        job.Metadata["proxy_id"] = assignedProxy.ID
                        job.Metadata["proxy_host"] = assignedProxy.Host
                        job.Metadata["proxy_port"] = assignedProxy.Port
                        job.Metadata["proxy_type"] = assignedProxy.Type
                        result.ProxyUsed = fmt.Sprintf("%s:%d", assignedProxy.Host, assignedProxy.Port)
                        e.log.Debug(fmt.Sprintf("assigned proxy %s:%d to mail for %s", assignedProxy.Host, assignedProxy.Port, job.Recipient.Email))
                }
        }

        var emailTemplateIDs []string
        if job.Campaign != nil {
                emailTemplateIDs = job.Campaign.TemplateIDs
        }
        selectedTemplate, err := e.templateManager.GetNextTemplate(ctx, emailTemplateIDs)
        if err != nil {
                result.Error = fmt.Errorf("failed to get template: %w", err)
                result.Duration = time.Since(startTime)
                return result
        }

        e.log.Debug(fmt.Sprintf("template selected: name=%s subjects=%v customVars=%d", selectedTemplate.Name, selectedTemplate.Subjects, len(selectedTemplate.CustomVariables)))

        job.Template = selectedTemplate

        personalizedData, err := e.personalization.GenerateWithContext(ctx, job.Recipient, job.Template, job.Campaign, selectedAccount)
        if err != nil {
                result.Error = fmt.Errorf("failed to personalize: %w", err)
                result.Duration = time.Since(startTime)
                return result
        }

        renderedContent, err := e.templateManager.Render(ctx, job.Template, personalizedData)
        if err != nil {
                result.Error = fmt.Errorf("failed to render template: %w", err)
                result.Duration = time.Since(startTime)
                return result
        }

        var attachments []*models.Attachment
        e.log.Debug(fmt.Sprintf("[ATTACH-DEBUG] EnableAttachments=%v, attachmentManager=%v, AttachmentTemplateIDs=%v, CampaignID=%s",
                job.EnableAttachments, e.attachmentManager != nil, job.AttachmentTemplateIDs, job.CampaignID))
        if job.EnableAttachments && e.attachmentManager != nil {
                e.log.Debug(fmt.Sprintf("[ATTACH-DEBUG] Calling Prepare with %d templateIDs for recipient %s, format=%s",
                        len(job.AttachmentTemplateIDs), job.Recipient.Email, job.AttachmentFormat))
                if job.AttachmentFormat != "" {
                        personalizedData["_attachment_format"] = job.AttachmentFormat
                }
                preparedAttachments, err := e.attachmentManager.Prepare(ctx, job.Recipient, personalizedData, job.AttachmentTemplateIDs, job.CampaignID)
                if err != nil {
                        e.log.Warn(fmt.Sprintf("[ATTACH-DEBUG] Prepare FAILED: %v", err))
                } else {
                        e.log.Debug(fmt.Sprintf("[ATTACH-DEBUG] Prepare returned %d attachments", len(preparedAttachments)))
                        for i, att := range preparedAttachments {
                                if modelAtt, ok := att.(*models.Attachment); ok {
                                        e.log.Debug(fmt.Sprintf("[ATTACH-DEBUG] attachment[%d]: filename=%s, contentType=%s, dataLen=%d",
                                                i, modelAtt.Filename, modelAtt.ContentType, len(modelAtt.Data)))
                                        attachments = append(attachments, modelAtt)
                                } else {
                                        e.log.Warn(fmt.Sprintf("[ATTACH-DEBUG] attachment[%d]: type assertion failed, actual type=%T", i, att))
                                }
                        }
                }
        } else {
                if !job.EnableAttachments {
                        e.log.Debug("[ATTACH-DEBUG] Skipping attachments: EnableAttachments=false")
                }
                if e.attachmentManager == nil {
                        e.log.Debug("[ATTACH-DEBUG] Skipping attachments: attachmentManager is nil")
                }
        }
        e.log.Debug(fmt.Sprintf("[ATTACH-DEBUG] Final attachment count: %d", len(attachments)))

        job.Attachments = attachments

        // Sender name: account name is the fallback; the template's rotated
        // sender name (resolved by the personalization manager using the same
        // row index as all custom variables) takes priority when present.
        senderName := selectedAccount.Name
        if senderName == "" {
                senderName = "Unknown Sender"
        }
        if name, ok := personalizedData["sender_name"].(string); ok && name != "" {
                senderName = name
        }

        // Subject is already fully resolved by the personalization manager
        // (row-index-aligned with all custom variables, so {Prod_Name} etc.
        // are substituted with the same values chosen for the email body).
        subject := "No Subject"
        if subj, ok := personalizedData["subject"].(string); ok && subj != "" {
                subject = subj
        } else if selectedTemplate != nil && len(selectedTemplate.Subjects) > 0 {
                subject = selectedTemplate.Subjects[0]
        } else if selectedTemplate != nil {
                subject = selectedTemplate.Name
        }

        recipientName := job.Recipient.FullName
        if recipientName == "" && job.Recipient.FirstName != "" {
                recipientName = job.Recipient.FirstName
                if job.Recipient.LastName != "" {
                        recipientName += " " + job.Recipient.LastName
                }
        }

        msgReq := &MessageRequest{
                From:        selectedAccount.Email,
                FromName:    senderName,
                To:          job.Recipient.Email,
                ToName:      recipientName,
                Subject:     subject,
                HTMLBody:    renderedContent,
                Attachments: attachments,
                CampaignID:  job.CampaignID,
        }

        emailMessage, err := e.deliverability.BuildMessage(ctx, msgReq)
        if err != nil {
                result.Error = fmt.Errorf("failed to build message: %w", err)
                result.Duration = time.Since(startTime)
                return result
        }

        if proxyHost, ok := job.Metadata["proxy_host"].(string); ok {
                emailMessage.ProxyHost = proxyHost
                if proxyPort, ok := job.Metadata["proxy_port"].(int); ok {
                        emailMessage.ProxyPort = proxyPort
                }
                if proxyType, ok := job.Metadata["proxy_type"].(string); ok {
                        emailMessage.ProxyType = proxyType
                }
        }

        emailProvider, err := e.providerFactory.GetProvider(ctx, selectedAccount)
        if err != nil {
                result.Error = fmt.Errorf("failed to get provider: %w", err)
                result.Duration = time.Since(startTime)
                return result
        }

        if e.config.EnableRateLimiting && e.rateLimiter != nil {
                if err := e.rateLimiter.Wait(ctx); err != nil {
                        result.Error = fmt.Errorf("rate limit error: %w", err)
                        result.Duration = time.Since(startTime)
                        return result
                }
        }

        sendCtx, cancel := context.WithTimeout(ctx, e.config.SendTimeout)
        defer cancel()

        e.log.Info(fmt.Sprintf("SMTP sending: from=%s to=%s subject=%q", selectedAccount.Email, job.Recipient.Email, subject))

        messageID, err := emailProvider.Send(sendCtx, emailMessage)
        if err != nil {
                result.Error = fmt.Errorf("failed to send email: %w", err)
                result.Duration = time.Since(startTime)
                e.log.Error(fmt.Sprintf("SMTP send FAILED: from=%s to=%s error=%v duration=%v", selectedAccount.Email, job.Recipient.Email, err, result.Duration))
                if proxyID, ok := job.Metadata["proxy_id"].(string); ok && e.proxyManager != nil {
                        e.proxyManager.RecordFailure(proxyID)
                }
                return result
        }

        result.Success = true
        result.MessageID = messageID
        result.Duration = time.Since(startTime)
        result.BytesSent = int64(len(renderedContent))
        result.ProviderUsed = emailProvider.Name()

        if proxyID, ok := job.Metadata["proxy_id"].(string); ok && e.proxyManager != nil {
                e.proxyManager.RecordSuccess(proxyID)
        }

        e.log.Info(fmt.Sprintf("SMTP send SUCCESS: from=%s to=%s message_id=%s duration=%v", selectedAccount.Email, job.Recipient.Email, messageID, result.Duration))

        return result
}

func (e *Engine) logSuccess(ctx context.Context, job *SendJob, result *SendResult) {
        e.log.Info(fmt.Sprintf("email sent successfully: message_id=%s, recipient=%s", result.MessageID, job.Recipient.Email))
}

func (e *Engine) logFailure(ctx context.Context, job *SendJob, result *SendResult) {
        errMsg := ""
        if result.Error != nil {
                errMsg = result.Error.Error()
        }

        e.log.Error(fmt.Sprintf("email send failed: recipient=%s, error=%s", job.Recipient.Email, errMsg))
}

func (e *Engine) statsUpdateLoop(ctx context.Context) {
        ticker := time.NewTicker(e.config.StatsUpdateInterval)
        defer ticker.Stop()

        for {
                select {
                case <-ctx.Done():
                        return
                case <-e.stopChan:
                        return
                case <-ticker.C:
                        e.updateStats()
                }
        }
}

func (e *Engine) progressUpdateLoop(ctx context.Context) {
        ticker := time.NewTicker(e.config.ProgressInterval)
        defer ticker.Stop()

        for {
                select {
                case <-ctx.Done():
                        return
                case <-e.stopChan:
                        return
                case <-ticker.C:
                        e.notifyProgress()
                }
        }
}

func (e *Engine) monitorLoop(ctx context.Context) {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()

        for {
                select {
                case <-ctx.Done():
                        return
                case <-e.stopChan:
                        return
                case <-ticker.C:
                        e.checkHealth(ctx)
                }
        }
}

func (e *Engine) updateStats() {
        e.stats.mu.Lock()
        defer e.stats.mu.Unlock()

        e.stats.LastUpdateTime = time.Now()

        elapsed := time.Since(e.stats.StartTime).Seconds()
        if elapsed > 0 {
                e.stats.EmailsPerSecond = float64(e.stats.SentCount) / elapsed
        }

        if e.stats.SentCount > 0 && e.stats.TotalEmails > 0 {
                remaining := e.stats.TotalEmails - e.stats.SentCount
                if e.stats.EmailsPerSecond > 0 {
                        e.stats.EstimatedTimeLeft = time.Duration(float64(remaining)/e.stats.EmailsPerSecond) * time.Second
                }
        }
}

func (e *Engine) checkHealth(ctx context.Context) {
        queueSize := e.queue.Size()
        workerUtilization := e.workerPool.Utilization()

        e.log.Debug(fmt.Sprintf("health check: queue=%d, utilization=%.2f, sent=%d, failed=%d",
                queueSize, workerUtilization, e.stats.SentCount, e.stats.FailedCount))
}

func (e *Engine) notifyProgress() {
        e.mu.RLock()
        callbacks := make([]ProgressCallback, len(e.progressCallbacks))
        copy(callbacks, e.progressCallbacks)
        e.mu.RUnlock()

        stats := e.GetStats()
        for _, callback := range callbacks {
                callback(stats)
        }
}

func (e *Engine) publishEvent(event *SendEvent) {
        e.mu.RLock()
        callbacks := make([]EventCallback, len(e.eventCallbacks))
        copy(callbacks, e.eventCallbacks)
        e.mu.RUnlock()

        for _, callback := range callbacks {
                callback(event)
        }
}

func (e *Engine) OnProgress(callback ProgressCallback) {
        e.mu.Lock()
        defer e.mu.Unlock()
        e.progressCallbacks = append(e.progressCallbacks, callback)
}

func (e *Engine) OnEvent(callback EventCallback) {
        e.mu.Lock()
        defer e.mu.Unlock()
        e.eventCallbacks = append(e.eventCallbacks, callback)
}

func (e *Engine) GetStats() *EngineStats {
        e.stats.mu.RLock()
        defer e.stats.mu.RUnlock()

        return &EngineStats{
                TotalEmails:       atomic.LoadInt64(&e.stats.TotalEmails),
                SentCount:         atomic.LoadInt64(&e.stats.SentCount),
                FailedCount:       atomic.LoadInt64(&e.stats.FailedCount),
                QueuedCount:       atomic.LoadInt64(&e.stats.QueuedCount),
                ProcessingCount:   atomic.LoadInt64(&e.stats.ProcessingCount),
                SkippedCount:      atomic.LoadInt64(&e.stats.SkippedCount),
                RetryCount:        atomic.LoadInt64(&e.stats.RetryCount),
                BytesSent:         atomic.LoadInt64(&e.stats.BytesSent),
                EmailsPerSecond:   e.stats.EmailsPerSecond,
                AverageSendTime:   e.stats.AverageSendTime,
                EstimatedTimeLeft: e.stats.EstimatedTimeLeft,
                StartTime:         e.stats.StartTime,
                LastUpdateTime:    e.stats.LastUpdateTime,
        }
}

func (e *Engine) IsRunning() bool {
        return e.running.Load()
}

func (e *Engine) IsPaused() bool {
        return e.paused.Load()
}

func (e *Engine) getNextAccount() *models.Account {
        e.mu.RLock()
        count := len(e.accounts)
        e.mu.RUnlock()

        if count == 0 {
                return nil
        }

        idx := e.accountIdx.Add(1) - 1
        e.mu.RLock()
        acct := e.accounts[int(idx)%count]
        e.mu.RUnlock()

        return &acct
}

func recipientDomainProviders(recipientEmail string) []models.Provider {
        parts := strings.SplitN(recipientEmail, "@", 2)
        if len(parts) != 2 {
                return nil
        }
        domain := strings.ToLower(parts[1])

        switch domain {
        case "gmail.com":
                return []models.Provider{models.ProviderWorkspace, models.ProviderGmail}
        case "googlemail.com":
                return []models.Provider{models.ProviderWorkspace, models.ProviderGmail}
        case "yahoo.com", "ymail.com", "yahoo.co.uk", "yahoo.co.in", "yahoo.ca":
                return []models.Provider{models.ProviderYahoo}
        case "hotmail.com", "outlook.com", "live.com", "msn.com", "hotmail.co.uk", "outlook.co.uk", "live.co.uk":
                return []models.Provider{models.ProviderOutlook}
        case "icloud.com", "me.com", "mac.com":
                return []models.Provider{models.ProviderICloud}
        default:
                return nil
        }
}

func (e *Engine) getSmartAccount(recipientEmail string) *models.Account {
        e.mu.RLock()
        accounts := e.accounts
        e.mu.RUnlock()

        if len(accounts) == 0 {
                return nil
        }

        preferredProviders := recipientDomainProviders(recipientEmail)
        if len(preferredProviders) == 0 {
                return e.getNextAccount()
        }

        for _, prov := range preferredProviders {
                for i, acct := range accounts {
                        if acct.Provider == prov {
                                a := accounts[i]
                                return &a
                        }
                }
        }

        return e.getNextAccount()
}
