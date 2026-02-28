package campaign

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "sync/atomic"
    "time"

    "email-campaign-system/internal/core/account"
    "email-campaign-system/internal/core/attachment"
    "email-campaign-system/internal/core/personalization"
    "email-campaign-system/internal/core/sender"
    "email-campaign-system/internal/models"
    "email-campaign-system/internal/storage/repository"
    "email-campaign-system/pkg/logger"
)

var (
    ErrExecutorStopped  = errors.New("executor stopped")
    ErrExecutorPaused   = errors.New("executor paused")
    ErrNoRecipients     = errors.New("no recipients to process")
    ErrNoAccounts       = errors.New("no accounts available")
    ErrInvalidBatchSize = errors.New("invalid batch size")
)

type Executor struct {
    senderEngine          *sender.Engine
    accountManager        *account.Manager
    attachmentManager     *attachment.Manager
    personalizationEngine *personalization.Manager
    recipientRepo         repository.RecipientRepository
    logRepo               *repository.LogRepository
    statsRepo             repository.StatsRepository
    log                   logger.Logger
    templateRepo          repository.TemplateRepository 
    config                ExecutorConfig
}

type ExecutorConfig struct {
    BatchSize           int
    WorkerCount         int
    MaxRetries          int
    RetryDelay          time.Duration
    StatsUpdateInterval time.Duration
    CheckpointInterval  time.Duration
    EnableNotifications bool
}

type ExecutorInstance struct {
    campaign       *models.Campaign
    state          *ExecutionState
    senderInstance *sender.Engine
    recipients     []*models.Recipient
    templates      []*models.Template
    accounts       []*models.Account
    stats          *ExecutionStats
    mu             sync.RWMutex
    pauseChan      chan struct{}
    resumeChan     chan struct{}
    stopChan       chan struct{}
    stopped        atomic.Bool
    paused         atomic.Bool
}

type ExecutionState struct {
    CurrentIndex     int64
    ProcessedCount   int64
    SuccessCount     int64
    FailedCount      int64
    SkippedCount     int64
    RetryQueue       []*EmailJob
    FailedJobs       []*EmailJob
    LastCheckpoint   time.Time
    LastStatsUpdate  time.Time
    mu               sync.RWMutex
}

type ExecutionStats struct {
    Total              int64
    Sent               int64
    Failed             int64
    Pending            int64
    Skipped            int64
    InProgress         int64
    AverageSendTime    time.Duration
    CurrentThroughput  float64
    StartTime          time.Time
    LastEmailTime      time.Time
    mu                 sync.RWMutex
}

type EmailJob struct {
    ID           string
    CampaignID   string
    Recipient    *models.Recipient
    Template     *models.Template
    Account      *models.Account
    RetryCount   int
    LastAttempt  time.Time
    Error        error
    CreatedAt    time.Time
}

func NewExecutor(
    senderEngine *sender.Engine,
    accountManager *account.Manager,
    attachmentManager *attachment.Manager,
    personalizationEngine *personalization.Manager,
    recipientRepo repository.RecipientRepository,
    templateRepo repository.TemplateRepository,
    logRepo *repository.LogRepository,
    statsRepo repository.StatsRepository,
    log logger.Logger,
    config ExecutorConfig,
) *Executor {
    if config.BatchSize <= 0 {
        config.BatchSize = 100
    }
    if config.WorkerCount <= 0 {
        config.WorkerCount = 4
    }
    if config.MaxRetries <= 0 {
        config.MaxRetries = 3
    }
    if config.RetryDelay <= 0 {
        config.RetryDelay = 5 * time.Second
    }
    if config.StatsUpdateInterval <= 0 {
        config.StatsUpdateInterval = 5 * time.Second
    }
    if config.CheckpointInterval <= 0 {
        config.CheckpointInterval = 30 * time.Second
    }

    return &Executor{
        senderEngine:          senderEngine,
        accountManager:        accountManager,
        attachmentManager:     attachmentManager,
        personalizationEngine: personalizationEngine,
        templateRepo:          templateRepo,
        recipientRepo:         recipientRepo,
        logRepo:               logRepo,
        statsRepo:             statsRepo,
        log:                   log,
        config:                config,
    }
}

func (e *Executor) Start(ctx context.Context, campaign *models.Campaign) (*ExecutorInstance, error) {
    
    recipients, err := e.loadRecipients(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load recipients: %w", err)
    }

    if len(recipients) == 0 {
        return nil, ErrNoRecipients
    }

    accounts, err := e.loadAccounts(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load accounts: %w", err)
    }

    if len(accounts) == 0 {
        return nil, ErrNoAccounts
    }

    templates, err := e.loadTemplates(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load templates: %w", err)
    }

    instance := &ExecutorInstance{
        campaign:       campaign,
        state:          NewExecutionState(),
        recipients:     recipients,
        templates:      templates,
        accounts:       accounts,
        stats:          NewExecutionStats(int64(len(recipients))),
        senderInstance: e.senderEngine,
        pauseChan:      make(chan struct{}),
        resumeChan:     make(chan struct{}),
        stopChan:       make(chan struct{}),
    }

    go instance.run(ctx, e)

    e.log.Info("executor started",
        logger.String("campaign_id", campaign.ID),
        logger.Int("recipients", len(recipients)),
        logger.Int("accounts", len(accounts)),
        logger.Int("templates", len(templates)),
    )

    return instance, nil
}

func (ei *ExecutorInstance) run(ctx context.Context, executor *Executor) {
    defer func() {
        if r := recover(); r != nil {
            executor.log.Error("executor panic recovered",
                logger.Any("error", r),
                logger.String("campaign_id", ei.campaign.ID),
            )
        }
    }()

    ei.stats.StartTime = time.Now()

    statsTicker := time.NewTicker(executor.config.StatsUpdateInterval)
    defer statsTicker.Stop()

    checkpointTicker := time.NewTicker(executor.config.CheckpointInterval)
    defer checkpointTicker.Stop()

    batchIndex := 0
    totalBatches := (len(ei.recipients) + executor.config.BatchSize - 1) / executor.config.BatchSize

    for batchIndex < totalBatches {
        select {
        case <-ctx.Done():
            ei.handleStop(executor)
            return
        case <-ei.stopChan:
            ei.handleStop(executor)
            return
        case <-ei.pauseChan:
            ei.handlePause(executor)
            <-ei.resumeChan
            ei.handleResume(executor)
        case <-statsTicker.C:
            ei.updateStats(executor)
        case <-checkpointTicker.C:
            ei.saveCheckpoint(ctx, executor)
        default:
            if ei.stopped.Load() {
                return
            }

            if ei.paused.Load() {
                time.Sleep(100 * time.Millisecond)
                continue
            }

            start := batchIndex * executor.config.BatchSize
            end := start + executor.config.BatchSize
            if end > len(ei.recipients) {
                end = len(ei.recipients)
            }

            batch := ei.recipients[start:end]
            ei.processBatch(ctx, batch, executor)

            batchIndex++
        }
    }

    ei.processRetryQueue(ctx, executor)
    ei.handleCompletion(executor)
}

func (ei *ExecutorInstance) processBatch(ctx context.Context, batch []*models.Recipient, executor *Executor) {
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, executor.config.WorkerCount)

    for _, recipient := range batch {
        if ei.stopped.Load() {
            break
        }

        wg.Add(1)
        semaphore <- struct{}{}

        go func(r *models.Recipient) {
            defer wg.Done()
            defer func() { <-semaphore }()

            ei.processRecipient(ctx, r, executor)
        }(recipient)
    }

    wg.Wait()
}

func (ei *ExecutorInstance) processRecipient(ctx context.Context, recipient *models.Recipient, executor *Executor) {
    job := &EmailJob{
        ID:          fmt.Sprintf("%s-%s", ei.campaign.ID, recipient.ID),
        CampaignID:  ei.campaign.ID,
        Recipient:   recipient,
        RetryCount:  0,
        CreatedAt:   time.Now(),
    }

    if len(ei.accounts) == 0 {
        ei.handleJobFailure(job, ErrNoAccounts, executor)
        return
    }
    job.Account = ei.accounts[0]

    if len(ei.templates) == 0 {
        ei.handleJobFailure(job, errors.New("no templates available"), executor)
        return
    }
    job.Template = ei.templates[0]

    // ✅ FIXED: Use ei.campaign directly (not &ei.campaign)
    personalizedResult, err := executor.personalizationEngine.Personalize(
        job.Template.HTMLContent,
        ei.campaign.Name,
        job.Account.Name,
        &personalization.PersonalizationContext{
            Recipient:      recipient,
            Campaign:       ei.campaign,  // ✅ Fixed: ei.campaign is already *models.Campaign
            Account:        job.Account,
            CustomFields:   make(map[string]string),
            Timestamp:      time.Now(),
            Timezone:       "UTC",
            AdditionalData: make(map[string]interface{}),
        },
    )
    if err != nil {
        ei.handleJobFailure(job, fmt.Errorf("personalization failed: %w", err), executor)
        return
    }

    htmlContent := personalizedResult.Content
    if htmlContent == "" {
        htmlContent = job.Template.HTMLContent
    }

    startTime := time.Now()
    ei.stats.IncrementInProgress()

    err = ei.senderInstance.QueueEmail(ctx, recipient, sender.PriorityNormal)


    sendDuration := time.Since(startTime)
    ei.stats.UpdateAverageSendTime(sendDuration)
    ei.stats.DecrementInProgress()

    if err != nil {
        job.Error = err
        job.LastAttempt = time.Now()

        if job.RetryCount < executor.config.MaxRetries {
            ei.addToRetryQueue(job)
            executor.log.Warn("email send failed, queued for retry",
                logger.String("campaign_id", ei.campaign.ID),
                logger.String("recipient", recipient.Email),
                logger.Int("retry_count", job.RetryCount),
                logger.String("error", err.Error()),
            )
        } else {
            ei.handleJobFailure(job, err, executor)
        }
        return
    }

    ei.handleJobSuccess(job, executor)
}

func (ei *ExecutorInstance) processRetryQueue(ctx context.Context, executor *Executor) {
    ei.state.mu.Lock()
    retryJobs := make([]*EmailJob, len(ei.state.RetryQueue))
    copy(retryJobs, ei.state.RetryQueue)
    ei.state.RetryQueue = nil
    ei.state.mu.Unlock()

    for _, job := range retryJobs {
        if ei.stopped.Load() {
            break
        }

        time.Sleep(executor.config.RetryDelay)

        job.RetryCount++
        job.LastAttempt = time.Now()

        ei.processRecipient(ctx, job.Recipient, executor)
    }
}

func (ei *ExecutorInstance) handleJobSuccess(job *EmailJob, executor *Executor) {
    ei.state.mu.Lock()
    ei.state.ProcessedCount++
    ei.state.SuccessCount++
    ei.state.mu.Unlock()

    ei.stats.IncrementSent()
    ei.stats.DecrementPending()
    ei.stats.LastEmailTime = time.Now()

    // ✅ FIXED: Use Insert instead of Save
    logEntry := &repository.LogEntry{
        ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
        CampaignID:  ei.campaign.ID,
        RecipientID: job.Recipient.ID,
        AccountID:   job.Account.ID,
        Level:       repository.LogLevelInfo,
        Category:    repository.LogCategoryCampaign,
        Message:     "Email sent successfully",
        Time:        time.Now(),
        CreatedAt:   time.Now(),
    }
    _ = executor.logRepo.Insert(context.Background(), logEntry)
}

func (ei *ExecutorInstance) handleJobFailure(job *EmailJob, err error, executor *Executor) {
    ei.state.mu.Lock()
    ei.state.ProcessedCount++
    ei.state.FailedCount++
    ei.state.FailedJobs = append(ei.state.FailedJobs, job)
    ei.state.mu.Unlock()

    ei.stats.IncrementFailed()
    ei.stats.DecrementPending()

    executor.log.Error("email send failed permanently",
        logger.String("campaign_id", ei.campaign.ID),
        logger.String("recipient", job.Recipient.Email),
        logger.String("error", err.Error()),
    )
	logEntry := &repository.LogEntry{
		ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
		CampaignID:  ei.campaign.ID,
		RecipientID:  job.Recipient.ID,
		AccountID:    job.Account.ID,
		Level:        repository.LogLevelError,
		Category:     repository.LogCategoryCampaign,
		Message:      "Email send failed",
		ErrorClass:   err.Error(),  // Use ErrorClass instead of ErrorMessage
		Time:         time.Now(),
		CreatedAt:    time.Now(),
	}

    _ = executor.logRepo.Insert(context.Background(), logEntry)
}

func (ei *ExecutorInstance) addToRetryQueue(job *EmailJob) {
    ei.state.mu.Lock()
    defer ei.state.mu.Unlock()
    ei.state.RetryQueue = append(ei.state.RetryQueue, job)
}

func (ei *ExecutorInstance) handlePause(executor *Executor) {
    ei.paused.Store(true)
    executor.log.Info("executor paused", logger.String("campaign_id", ei.campaign.ID))
}

func (ei *ExecutorInstance) handleResume(executor *Executor) {
    ei.paused.Store(false)
    executor.log.Info("executor resumed", logger.String("campaign_id", ei.campaign.ID))
}

func (ei *ExecutorInstance) handleStop(executor *Executor) {
    ei.stopped.Store(true)
    executor.log.Info("executor stopped", logger.String("campaign_id", ei.campaign.ID))
}

func (ei *ExecutorInstance) handleCompletion(executor *Executor) {
    executor.log.Info("executor completed",
        logger.String("campaign_id", ei.campaign.ID),
        logger.Int64("sent", ei.stats.Sent),
        logger.Int64("failed", ei.stats.Failed),
        logger.Duration("duration", time.Since(ei.stats.StartTime)),
    )
}

func (ei *ExecutorInstance) updateStats(executor *Executor) {
    ei.state.mu.Lock()
    ei.state.LastStatsUpdate = time.Now()
    ei.state.mu.Unlock()
}

func (ei *ExecutorInstance) saveCheckpoint(ctx context.Context, executor *Executor) {
    ei.state.mu.Lock()
    ei.state.LastCheckpoint = time.Now()
    ei.state.mu.Unlock()

    executor.log.Debug("checkpoint saved", logger.String("campaign_id", ei.campaign.ID))
}

func (ei *ExecutorInstance) Pause(ctx context.Context) error {
    if ei.stopped.Load() {
        return ErrExecutorStopped
    }
    if ei.paused.Load() {
        return ErrExecutorPaused
    }

    select {
    case ei.pauseChan <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (ei *ExecutorInstance) Resume(ctx context.Context) error {
    if ei.stopped.Load() {
        return ErrExecutorStopped
    }
    if !ei.paused.Load() {
        return nil
    }

    select {
    case ei.resumeChan <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (ei *ExecutorInstance) Stop(ctx context.Context) error {
    if ei.stopped.Load() {
        return nil
    }

    select {
    case ei.stopChan <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (ei *ExecutorInstance) IsComplete() bool {
    ei.stats.mu.RLock()
    defer ei.stats.mu.RUnlock()
    return ei.stats.Pending == 0 && ei.stats.InProgress == 0
}

func (ei *ExecutorInstance) GetStats() *ExecutionStats {
    ei.stats.mu.RLock()
    defer ei.stats.mu.RUnlock()

    return &ExecutionStats{
        Total:             ei.stats.Total,
        Sent:              ei.stats.Sent,
        Failed:            ei.stats.Failed,
        Pending:           ei.stats.Pending,
        Skipped:           ei.stats.Skipped,
        InProgress:        ei.stats.InProgress,
        AverageSendTime:   ei.stats.AverageSendTime,
        CurrentThroughput: ei.stats.CurrentThroughput,
        StartTime:         ei.stats.StartTime,
        LastEmailTime:     ei.stats.LastEmailTime,
    }
}
func (e *Executor) loadRecipients(ctx context.Context, campaign *models.Campaign) ([]*models.Recipient, error) {
	fmt.Printf("🟢 DEBUG loadRecipients: campaignID=%s\n", campaign.ID)

	filter := &repository.RecipientFilter{
		ListIDs: []string{campaign.ID}, // scope to THIS campaign
		Limit:   10000,
		// No Status filter — accept any status
	}

	repoRecipients, total, err := e.recipientRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to load recipients: %w", err)
	}

	fmt.Printf("🟢 DEBUG loadRecipients: found=%d total=%d\n", len(repoRecipients), total)

	recipients := make([]*models.Recipient, len(repoRecipients))
	for i, repoRec := range repoRecipients {
		recipients[i] = &models.Recipient{
			ID:     repoRec.ID,
			Email:  repoRec.Email,
			Status: models.RecipientStatus(repoRec.Status),
		}
	}
	return recipients, nil
}

func (e *Executor) loadAccounts(ctx context.Context, campaign *models.Campaign) ([]*models.Account, error) {
    accounts, _, err := e.accountManager.List(ctx, nil)
    return accounts, err
}

func (e *Executor) loadTemplates(ctx context.Context, campaign *models.Campaign) ([]*models.Template, error) {
    fmt.Printf("🟢 DEBUG loadTemplates: templateIDs=%v\n", campaign.TemplateIDs)

    if len(campaign.TemplateIDs) == 0 {
        return nil, fmt.Errorf("no template IDs in campaign")
    }

    templates := make([]*models.Template, 0, len(campaign.TemplateIDs))
    for _, id := range campaign.TemplateIDs {
        tmpl, err := e.templateRepo.GetByID(ctx, id)
        if err != nil {
            fmt.Printf("🔴 DEBUG loadTemplates: error fetching id=%s: %v\n", id, err)
            continue
        }
        templates = append(templates, &models.Template{
            ID:               tmpl.ID,
            HTMLContent:      tmpl.HtmlContent,      // repo field is HtmlContent
            PlainTextContent: tmpl.TextContent,      // repo field is TextContent
        })
    }

    if len(templates) == 0 {
        return nil, fmt.Errorf("no templates could be loaded")
    }

    fmt.Printf("🟢 DEBUG loadTemplates: loaded=%d\n", len(templates))
    return templates, nil
}

func NewExecutionState() *ExecutionState {
    return &ExecutionState{
        RetryQueue: make([]*EmailJob, 0),
        FailedJobs: make([]*EmailJob, 0),
    }
}

func NewExecutionStats(total int64) *ExecutionStats {
    return &ExecutionStats{
        Total:   total,
        Pending: total,
        Sent:    0,
        Failed:  0,
    }
}

func (es *ExecutionStats) IncrementSent() {
    es.mu.Lock()
    defer es.mu.Unlock()
    es.Sent++
}

func (es *ExecutionStats) IncrementFailed() {
    es.mu.Lock()
    defer es.mu.Unlock()
    es.Failed++
}

func (es *ExecutionStats) DecrementPending() {
    es.mu.Lock()
    defer es.mu.Unlock()
    if es.Pending > 0 {
        es.Pending--
    }
}

func (es *ExecutionStats) IncrementInProgress() {
    es.mu.Lock()
    defer es.mu.Unlock()
    es.InProgress++
}

func (es *ExecutionStats) DecrementInProgress() {
    es.mu.Lock()
    defer es.mu.Unlock()
    if es.InProgress > 0 {
        es.InProgress--
    }
}

func (es *ExecutionStats) UpdateAverageSendTime(duration time.Duration) {
    es.mu.Lock()
    defer es.mu.Unlock()

    if es.Sent == 0 {
        es.AverageSendTime = duration
    } else {
        total := time.Duration(es.Sent) * es.AverageSendTime
        es.AverageSendTime = (total + duration) / time.Duration(es.Sent+1)
    }

    if time.Since(es.StartTime).Seconds() > 0 {
        es.CurrentThroughput = float64(es.Sent) / time.Since(es.StartTime).Seconds()
    }
}
