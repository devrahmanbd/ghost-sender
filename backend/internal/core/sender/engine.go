package sender

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
	GetNextTemplate(ctx context.Context) (*models.Template, error)
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
	From    string
	To      string
	Subject string
	Body    string
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
	queue             *Queue
	workerPool        *WorkerPool
	batchProcessor    *BatchProcessor
	retryHandler      *RetryHandler
	logRepo           repository.LogRepository
	statsRepo         repository.StatsRepository
	log               logger.Logger
	config            EngineConfig
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
	ID          string
	CampaignID  string
	Recipient   *models.Recipient
	Template    *models.Template
	Account     *models.Account
	Attachments []*models.Attachment
	Priority    int
	Retries     int
	MaxRetries  int
	CreatedAt   time.Time
	ScheduledAt time.Time
	Metadata    map[string]interface{}
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
		config.WorkerCount = 4
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
		WorkerCount:   config.WorkerCount,
		JobTimeout:    config.SendTimeout,
		IdleTimeout:   30 * time.Second,
		EnableMetrics: true,
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
	defer e.mu.Unlock()
	e.campaign = campaign
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
	close(e.stopChan)

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
	e.resumeChan = make(chan struct{})

	e.log.Info(fmt.Sprintf("engine resumed: campaign_id=%s", e.campaign.ID))

	return nil
}

func (e *Engine) QueueEmail(ctx context.Context, recipient *models.Recipient, priority int) error {
	if !e.running.Load() {
		return ErrEngineNotRunning
	}

	job := &SendJob{
		ID:          fmt.Sprintf("%s-%s-%d", e.campaign.ID, recipient.Email, time.Now().UnixNano()),
		CampaignID:  e.campaign.ID,
		Recipient:   recipient,
		Priority:    priority,
		MaxRetries:  e.config.MaxRetries,
		CreatedAt:   time.Now(),
		ScheduledAt: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	if err := e.queue.Enqueue(job); err != nil {
		return fmt.Errorf("failed to queue email: %w", err)
	}

	atomic.AddInt64(&e.stats.QueuedCount, 1)
	atomic.AddInt64(&e.stats.TotalEmails, 1)

	e.publishEvent(&SendEvent{
		Type:      EventTypeQueued,
		Job:       job,
		Timestamp: time.Now(),
	})

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

	// Create a stub account since account management methods are not implemented
	stubAccount := &models.Account{
		ID:       "stub-account-1",
		Email:    "noreply@example.com",
		Provider: "smtp",
	}
	
	job.Account = stubAccount
	result.AccountUsed = stubAccount.Email

	selectedTemplate, err := e.templateManager.GetNextTemplate(ctx)
	if err != nil {
		result.Error = fmt.Errorf("failed to get template: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	job.Template = selectedTemplate

	personalizedData, err := e.personalization.Generate(ctx, job.Recipient, job.Template)
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
	if e.attachmentManager != nil {
		preparedAttachments, err := e.attachmentManager.Prepare(ctx, job.Recipient, personalizedData)
		if err != nil {
			e.log.Warn(fmt.Sprintf("failed to prepare attachments: %v", err))
		} else {
			for _, att := range preparedAttachments {
				if modelAtt, ok := att.(*models.Attachment); ok {
					attachments = append(attachments, modelAtt)
				}
			}
		}
	}

	job.Attachments = attachments

	senderName := "Unknown Sender"
	if name, ok := personalizedData["SENDER_NAME"].(string); ok {
		senderName = name
	}

	subject := "No Subject"
	if subj, ok := personalizedData["SUBJECT"].(string); ok {
		subject = subj
	}

	emailMessage, err := e.deliverability.BuildMessage(ctx, &MessageRequest{
		From:        stubAccount.Email,
		FromName:    senderName,
		To:          job.Recipient.Email,
		ToName:      job.Recipient.Email,
		Subject:     subject,
		HTMLBody:    renderedContent,
		Attachments: attachments,
		CampaignID:  e.campaign.ID,
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to build message: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	emailProvider, err := e.providerFactory.GetProvider(ctx, stubAccount)
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

	messageID, err := emailProvider.Send(sendCtx, emailMessage)
	if err != nil {
		result.Error = fmt.Errorf("failed to send email: %w", err)
		result.Duration = time.Since(startTime)
		e.log.Error(fmt.Sprintf("failed to send email for account %s: %v", stubAccount.Email, err))
		return result
	}

	result.Success = true
	result.MessageID = messageID
	result.Duration = time.Since(startTime)
	result.BytesSent = int64(len(renderedContent))
	result.ProviderUsed = emailProvider.Name()

	e.log.Debug(fmt.Sprintf("email sent successfully from account %s", stubAccount.Email))

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
	stats := e.GetStats()
	for _, callback := range e.progressCallbacks {
		callback(stats)
	}
}

func (e *Engine) publishEvent(event *SendEvent) {
	for _, callback := range e.eventCallbacks {
		go callback(event)
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
