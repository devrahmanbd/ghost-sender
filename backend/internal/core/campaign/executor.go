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

    "github.com/google/uuid"
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

func (ei *ExecutorInstance) run(ctx context.Context, executor *Executor) {
    defer func() {
        if r := recover(); r != nil {
            executor.log.Error("executor panic recovered",
                logger.Any("error", r),
                logger.String("campaignid", ei.campaign.ID),
            )
        }
    }()

    ei.stats.StartTime = time.Now()

    // Register engine delivery callbacks.
    // Stats only update on real SMTP confirmation, not on queue entry.
    ei.senderInstance.OnEvent(func(event *sender.SendEvent) {
        if event.Job.CampaignID != ei.campaign.ID {
            return
        }
        switch event.Type {
        case sender.EventTypeSent:
            ei.state.mu.Lock()
            ei.state.SuccessCount++
            ei.state.mu.Unlock()
            ei.stats.IncrementSent()
            ei.stats.DecrementInProgress()
            ei.stats.LastEmailTime = time.Now()
            logEntry := &repository.LogEntry{
                ID:          uuid.New().String(),
                CampaignID:  ei.campaign.ID,
                RecipientID: event.Job.Recipient.ID,
                AccountID:   event.Job.Account.ID,
                Level:       repository.LogLevelInfo,
                Category:    repository.LogCategoryCampaign,
                Message:     "Email sent successfully",
                Time:        time.Now(),
                CreatedAt:   time.Now(),
            }
            if insertErr := executor.logRepo.Insert(context.Background(), logEntry); insertErr != nil {
                executor.log.Error("failed to insert sent log entry",
                    logger.String("campaign_id", ei.campaign.ID),
                    logger.String("error", insertErr.Error()),
                )
            }

        case sender.EventTypeFailed:
            // Only count final failure, not intermediate retries
            if event.Job.Retries >= event.Job.MaxRetries {
                ei.state.mu.Lock()
                ei.state.FailedCount++
                ei.state.mu.Unlock()
                ei.stats.IncrementFailed()
                ei.stats.DecrementInProgress()
                logEntry := &repository.LogEntry{
                    ID:          uuid.New().String(),
                    CampaignID:  ei.campaign.ID,
                    RecipientID: event.Job.Recipient.ID,
                    AccountID:   event.Job.Account.ID,
                    Level:       repository.LogLevelError,
                    Category:    repository.LogCategoryCampaign,
                    Message:     "Email send failed permanently",
                    ErrorClass:  event.Result.Error.Error(),
                    Time:        time.Now(),
                    CreatedAt:   time.Now(),
                }
                if insertErr := executor.logRepo.Insert(context.Background(), logEntry); insertErr != nil {
                    executor.log.Error("failed to insert failed log entry",
                        logger.String("campaign_id", ei.campaign.ID),
                        logger.String("error", insertErr.Error()),
                    )
                }
            }
        }
    })

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

    waitTimeout := 5 * time.Minute
    waitDeadline := time.After(waitTimeout)
    pollInterval := 500 * time.Millisecond
    for {
        ei.stats.mu.RLock()
        inProgress := ei.stats.InProgress
        ei.stats.mu.RUnlock()
        if inProgress <= 0 {
            break
        }
        select {
        case <-ctx.Done():
            executor.log.Warn("context cancelled while waiting for in-progress sends",
                logger.String("campaign_id", ei.campaign.ID),
                logger.Int64("in_progress", inProgress),
            )
            ei.handleStop(executor)
            return
        case <-ei.stopChan:
            ei.handleStop(executor)
            return
        case <-waitDeadline:
            executor.log.Warn("timed out waiting for in-progress sends to complete",
                logger.String("campaign_id", ei.campaign.ID),
                logger.Int64("in_progress", inProgress),
            )
            goto complete
        case <-time.After(pollInterval):
        }
    }
complete:
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
    job := EmailJob{
        ID:         fmt.Sprintf("%s-%s", ei.campaign.ID, recipient.ID),
        CampaignID: ei.campaign.ID,
        Recipient:  recipient,
        RetryCount: 0,
        CreatedAt:  time.Now(),
    }

    if len(ei.accounts) == 0 {
        ei.handleJobFailure(&job, ErrNoAccounts, executor)
        return
    }
    job.Account = ei.accounts[0]

    if len(ei.templates) == 0 {
        ei.handleJobFailure(&job, errors.New("no templates available"), executor)
        return
    }
    job.Template = ei.templates[0]

    campaignCustomFields := extractCampaignCustomFields(ei.campaign)
    
    if job.Template.CustomVariables != nil {
        if campaignCustomFields == nil {
            campaignCustomFields = make(map[string][]string)
        }
        for key, values := range job.Template.CustomVariables {
            campaignCustomFields[key] = values
        }
    }

    personalizedResult, err := executor.personalizationEngine.Personalize(
        job.Template.HTMLContent, ei.campaign.Name, job.Account.Name,
        &personalization.PersonalizationContext{
            Recipient:            recipient,
            Campaign:             ei.campaign,
            Account:              job.Account,
            CustomFields:         recipient.CustomFields,
            CampaignCustomFields: campaignCustomFields,
            Timestamp:            time.Now(),
            Timezone:             "UTC",
            AdditionalData:       make(map[string]interface{}),
        },
    )
    if err != nil {
        ei.handleJobFailure(&job, fmt.Errorf("personalization failed: %w", err), executor)
        return
    }

    htmlContent := personalizedResult.Content
    if htmlContent == "" {
        htmlContent = job.Template.HTMLContent
    }

    startTime := time.Now()

    // InProgress tracks jobs submitted to the engine queue but not yet confirmed by SMTP.
    // It is decremented by the OnEvent callback in run() on EventTypeSent or EventTypeFailed.
    ei.stats.IncrementInProgress()

    err = ei.senderInstance.QueueEmail(ctx, recipient, sender.PriorityNormal)
    if err != nil {
        // Failed to even enqueue — revert InProgress immediately
        ei.stats.DecrementInProgress()
        job.Error = err
        job.LastAttempt = time.Now()
        if job.RetryCount < executor.config.MaxRetries {
            ei.addToRetryQueue(&job)
            executor.log.Warn("email queuing failed, added to retry",
                logger.String("campaignid", ei.campaign.ID),
                logger.String("recipient", recipient.Email),
                logger.Int("retrycount", job.RetryCount),
                logger.String("error", err.Error()),
            )
        } else {
            ei.handleJobFailure(&job, err, executor)
        }
        return
    }

    // Successfully handed to the send engine queue.
    // Do NOT call IncrementSent here — that only happens in the OnEvent callback
    // after the SMTP server returns 250 OK.
    ei.stats.DecrementPending()
    ei.stats.UpdateAverageSendTime(time.Since(startTime))

    executor.log.Info("email queued for delivery",
        logger.String("campaignid", ei.campaign.ID),
        logger.String("recipient", recipient.Email),
    )
}

 
func (ei *ExecutorInstance) handleJobQueued(job *EmailJob, executor *Executor) {
    ei.stats.DecrementPending()
    ei.stats.LastEmailTime = time.Now()

    executor.log.Info("email queued for delivery",
        logger.String("campaign_id", ei.campaign.ID),
        logger.String("recipient", job.Recipient.Email),
    )
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


    logEntry := &repository.LogEntry{
        ID:          uuid.New().String(),
        CampaignID:  ei.campaign.ID,
        RecipientID: job.Recipient.ID,
        AccountID:   job.Account.ID,
        Level:       repository.LogLevelInfo,
        Category:    repository.LogCategoryCampaign,
        Message:     "Email sent successfully",
        Time:        time.Now(),
        CreatedAt:   time.Now(),
    }
    if insertErr := executor.logRepo.Insert(context.Background(), logEntry); insertErr != nil {
        executor.log.Error("failed to insert success log entry",
            logger.String("campaign_id", ei.campaign.ID),
            logger.String("error", insertErr.Error()),
        )
    }
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
                ID:          uuid.New().String(),
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

    if insertErr := executor.logRepo.Insert(context.Background(), logEntry); insertErr != nil {
        executor.log.Error("failed to insert failure log entry",
            logger.String("campaign_id", ei.campaign.ID),
            logger.String("error", insertErr.Error()),
        )
    }
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
        listID := campaign.RecipientGroupID
        if listID == "" {
                return nil, fmt.Errorf("campaign has no recipient list assigned")
        }
        filter := &repository.RecipientFilter{
                ListIDs: []string{listID},
                Limit:   10000,
        }

        repoRecipients, _, err := e.recipientRepo.List(ctx, filter)
        if err != nil {
                return nil, fmt.Errorf("failed to load recipients: %w", err)
        }

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
    opts := &account.ListOptions{
        Page:      1,
        PageSize:  100,
        SortBy:    "created_at",
        SortOrder: "desc",
    }

    accounts, _, err := e.accountManager.List(ctx, opts)
    if err != nil {
        return nil, fmt.Errorf("failed to load accounts: %w", err)
    }

    if len(campaign.AccountIDs) > 0 {
        accountIDSet := make(map[string]struct{}, len(campaign.AccountIDs))
        for _, id := range campaign.AccountIDs {
            accountIDSet[id] = struct{}{}
        }

        filtered := make([]*models.Account, 0, len(campaign.AccountIDs))
        for _, acc := range accounts {
            if _, ok := accountIDSet[acc.ID]; ok {
                filtered = append(filtered, acc)
            }
        }
        return filtered, nil
    }

    return accounts, nil
}



func (e *Executor) loadTemplates(ctx context.Context, campaign *models.Campaign) ([]*models.Template, error) {
    if len(campaign.TemplateIDs) == 0 {
        return nil, fmt.Errorf("no template IDs in campaign")
    }

    templates := make([]*models.Template, 0, len(campaign.TemplateIDs))
    for _, id := range campaign.TemplateIDs {
        tmpl, err := e.templateRepo.GetByID(ctx, id)
        if err != nil {
            continue
        }
        var subjects []string
        if tmpl.Subject != "" {
            subjects = []string{tmpl.Subject}
        }
        status := models.TemplateStatusInactive
        if tmpl.IsActive {
            status = models.TemplateStatusActive
        }
        templates = append(templates, &models.Template{
            ID:               tmpl.ID,
            Name:             tmpl.Name,
            Description:      tmpl.Description,
            HTMLContent:      tmpl.HtmlContent,
            PlainTextContent: tmpl.TextContent,
            Subjects:         subjects,
            Tags:             tmpl.Tags,
            Status:           status,
            SpamScore:        tmpl.SpamScore,
            Metadata:         tmpl.Metadata,
            CustomVariables:  tmpl.CustomVariables,
            CreatedAt:        tmpl.CreatedAt,
            UpdatedAt:        tmpl.UpdatedAt,
        })
    }

    if len(templates) == 0 {
        return nil, fmt.Errorf("no templates could be loaded")
    }

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
func (e *Executor) Start(ctx context.Context, campaign *models.Campaign) (instance *ExecutorInstance, err error) {
    e.senderEngine.SetCampaign(campaign)

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

    accountValues := make([]models.Account, 0, len(accounts))
    for _, a := range accounts {
        if a != nil {
            accountValues = append(accountValues, *a)
        }
    }
    if len(accountValues) == 0 {
        return nil, ErrNoAccounts
    }
    e.senderEngine.SetAccounts(accountValues)

    templates, err := e.loadTemplates(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load templates: %w", err)
    }

    e.senderEngine.SetSmartSending(campaign.Config.SmartSending)

    if !e.senderEngine.IsRunning() {
        if err := e.senderEngine.Start(ctx); err != nil {
            return nil, fmt.Errorf("failed to start sender engine: %w", err)
        }
    }

    instance = &ExecutorInstance{
        campaign:       campaign,
        state:          NewExecutionState(),
        recipients:     recipients,
        templates:      templates,
        accounts:       accounts,
        stats:          NewExecutionStats(int64(len(recipients))),
        senderInstance: e.senderEngine,
        pauseChan:      make(chan struct{}, 1),
        resumeChan:     make(chan struct{}, 1),
        stopChan:       make(chan struct{}, 1),
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
func (e *Executor) StartFromCheckpoint(ctx context.Context, campaign *models.Campaign, processedCount int64) (instance *ExecutorInstance, err error) {
    e.senderEngine.SetCampaign(campaign)
    e.senderEngine.SetSmartSending(campaign.Config.SmartSending)

    allRecipients, err := e.loadRecipients(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load recipients: %w", err)
    }

    offset := int(processedCount)
    if offset >= len(allRecipients) {
        return nil, fmt.Errorf("campaign already fully processed: offset=%d total=%d", offset, len(allRecipients))
    }
    remaining := allRecipients[offset:]

    accounts, err := e.loadAccounts(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load accounts: %w", err)
    }
    if len(accounts) == 0 {
        return nil, ErrNoAccounts
    }

    acctVals := make([]models.Account, len(accounts))
    for i, a := range accounts {
        acctVals[i] = *a
    }
    e.senderEngine.SetAccounts(acctVals)

    templates, err := e.loadTemplates(ctx, campaign)
    if err != nil {
        return nil, fmt.Errorf("failed to load templates: %w", err)
    }

    if !e.senderEngine.IsRunning() {
        if err := e.senderEngine.Start(ctx); err != nil {
            return nil, fmt.Errorf("failed to start sender engine: %w", err)
        }
    }

    instance = &ExecutorInstance{
        campaign:       campaign,
        state:          NewExecutionState(),
        recipients:     remaining,
        templates:      templates,
        accounts:       accounts,
        stats:          NewExecutionStats(int64(len(remaining))),
        senderInstance: e.senderEngine,
        pauseChan:      make(chan struct{}, 1),
        resumeChan:     make(chan struct{}, 1),
        stopChan:       make(chan struct{}, 1),
    }

    go instance.run(ctx, e)

    e.log.Info("executor resumed from checkpoint",
        logger.String("campaign_id", campaign.ID),
        logger.Int64("skipped", processedCount),
        logger.Int("remaining", len(remaining)),
    )

    return instance, nil
}

func extractCampaignCustomFields(campaign *models.Campaign) map[string][]string {
    if campaign == nil || campaign.Config.Metadata == nil {
        return nil
    }
    raw, ok := campaign.Config.Metadata["custom_fields"]
    if !ok || raw == nil {
        return nil
    }

    result := make(map[string][]string)

    switch cf := raw.(type) {
    case map[string]interface{}:
        for key, val := range cf {
            switch v := val.(type) {
            case []interface{}:
                values := make([]string, 0, len(v))
                for _, item := range v {
                    if s, ok := item.(string); ok && s != "" {
                        values = append(values, s)
                    }
                }
                if len(values) > 0 {
                    result[key] = values
                }
            case []string:
                if len(v) > 0 {
                    result[key] = v
                }
            }
        }
    }

    if len(result) == 0 {
        return nil
    }
    return result
}
