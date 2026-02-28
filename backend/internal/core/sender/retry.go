package sender

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"email-campaign-system/pkg/logger"
)

var (
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
	ErrNonRetryable       = errors.New("non-retryable error")
	ErrRetryScheduled     = errors.New("retry scheduled")
)

type RetryHandler struct {
	config         RetryConfig
	log            logger.Logger
	retryQueue     chan *RetryTask
	scheduledTasks map[string]*RetryTask
	mu             sync.RWMutex
	metrics        *RetryMetrics
	stopChan       chan struct{}
	wg             sync.WaitGroup
	running        atomic.Bool
}

type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	EnableJitter bool
	JitterFactor float64
	QueueSize    int
	WorkerCount  int
}

type RetryTask struct {
	ID            string
	Job           interface{}
	Error         error
	Attempt       int
	MaxAttempts   int
	NextRetryAt   time.Time
	CreatedAt     time.Time
	LastAttemptAt time.Time
	Delays        []time.Duration
}

type RetryMetrics struct {
	TotalRetries      int64
	SuccessfulRetries int64
	FailedRetries     int64
	ScheduledRetries  int64
	ExhaustedRetries  int64
	AverageRetryDelay time.Duration
	TotalRetryDelay   time.Duration
	RetrySuccessRate  float64
	LastRetryAt       time.Time
	mu                sync.RWMutex
}

type ErrorClassifier func(error) bool

var (
	defaultRetryableErrors = []error{
		errors.New("connection timeout"),
		errors.New("temporary failure"),
		errors.New("rate limit exceeded"),
		errors.New("service unavailable"),
	}

	defaultNonRetryableErrors = []error{
		errors.New("authentication failed"),
		errors.New("invalid recipient"),
		errors.New("permanent failure"),
		errors.New("account suspended"),
	}
)

func NewRetryHandler(config RetryConfig, log logger.Logger) *RetryHandler {
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 1 * time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 5 * time.Minute
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	if config.JitterFactor <= 0 {
		config.JitterFactor = 0.1
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 2
	}

	rh := &RetryHandler{
		config:         config,
		log:            log,
		retryQueue:     make(chan *RetryTask, config.QueueSize),
		scheduledTasks: make(map[string]*RetryTask),
		metrics:        &RetryMetrics{},
		stopChan:       make(chan struct{}),
	}

	return rh
}

func (rh *RetryHandler) Start(ctx context.Context) error {
	if rh.running.Load() {
		return errors.New("retry handler already running")
	}

	rh.running.Store(true)

	for i := 0; i < rh.config.WorkerCount; i++ {
		rh.wg.Add(1)
		go rh.retryWorker(ctx, i)
	}

	rh.wg.Add(1)
	go rh.schedulerLoop(ctx)

	rh.log.Info(fmt.Sprintf("retry handler started: workers=%d", rh.config.WorkerCount))

	return nil
}

func (rh *RetryHandler) Stop(ctx context.Context) error {
	if !rh.running.Load() {
		return errors.New("retry handler not running")
	}

	rh.running.Store(false)
	close(rh.stopChan)
	close(rh.retryQueue)

	done := make(chan struct{})
	go func() {
		rh.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		rh.log.Info("retry handler stopped gracefully")
		return nil
	case <-ctx.Done():
		rh.log.Warn("retry handler stop timeout")
		return ctx.Err()
	}
}

func (rh *RetryHandler) Schedule(ctx context.Context, job interface{}, err error) error {
	if !rh.running.Load() {
		return errors.New("retry handler not running")
	}

	sendJob, ok := job.(*SendJob)
	if !ok {
		return errors.New("invalid job type")
	}

	if !rh.IsRetryable(err) {
		rh.log.Debug(fmt.Sprintf("error not retryable: %v", err))
		return ErrNonRetryable
	}

	if sendJob.Retries >= sendJob.MaxRetries {
		atomic.AddInt64(&rh.metrics.ExhaustedRetries, 1)
		return ErrMaxRetriesExceeded
	}

	delay := rh.calculateDelay(sendJob.Retries)
	nextRetryAt := time.Now().Add(delay)

	task := &RetryTask{
		ID:            fmt.Sprintf("%s-retry-%d", sendJob.ID, sendJob.Retries),
		Job:           job,
		Error:         err,
		Attempt:       sendJob.Retries,
		MaxAttempts:   sendJob.MaxRetries,
		NextRetryAt:   nextRetryAt,
		CreatedAt:     time.Now(),
		LastAttemptAt: time.Now(),
		Delays:        []time.Duration{delay},
	}

	rh.mu.Lock()
	rh.scheduledTasks[task.ID] = task
	rh.mu.Unlock()

	atomic.AddInt64(&rh.metrics.ScheduledRetries, 1)
	atomic.AddInt64(&rh.metrics.TotalRetries, 1)

	rh.metrics.mu.Lock()
	rh.metrics.TotalRetryDelay += delay
	totalRetries := atomic.LoadInt64(&rh.metrics.TotalRetries)
	if totalRetries > 0 {
		rh.metrics.AverageRetryDelay = time.Duration(
			int64(rh.metrics.TotalRetryDelay) / totalRetries,
		)
	}
	rh.metrics.mu.Unlock()

	rh.log.Debug(fmt.Sprintf("retry scheduled: job_id=%s, attempt=%d, max_attempts=%d, delay=%v, next_retry=%v",
		sendJob.ID, sendJob.Retries+1, sendJob.MaxRetries, delay, nextRetryAt))

	return ErrRetryScheduled
}

func (rh *RetryHandler) calculateDelay(attempt int) time.Duration {
	delay := float64(rh.config.InitialDelay) * math.Pow(rh.config.Multiplier, float64(attempt))

	if delay > float64(rh.config.MaxDelay) {
		delay = float64(rh.config.MaxDelay)
	}

	if rh.config.EnableJitter {
		jitter := delay * rh.config.JitterFactor * (rand.Float64()*2 - 1)
		delay += jitter
	}

	if delay < float64(rh.config.InitialDelay) {
		delay = float64(rh.config.InitialDelay)
	}

	return time.Duration(delay)
}

func (rh *RetryHandler) schedulerLoop(ctx context.Context) {
	defer rh.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rh.stopChan:
			return
		case <-ticker.C:
			rh.processScheduledTasks()
		}
	}
}

func (rh *RetryHandler) processScheduledTasks() {
	now := time.Now()

	rh.mu.Lock()
	tasksToRetry := make([]*RetryTask, 0)

	for id, task := range rh.scheduledTasks {
		if task.NextRetryAt.Before(now) || task.NextRetryAt.Equal(now) {
			tasksToRetry = append(tasksToRetry, task)
			delete(rh.scheduledTasks, id)
		}
	}
	rh.mu.Unlock()

	for _, task := range tasksToRetry {
		select {
		case rh.retryQueue <- task:
			atomic.AddInt64(&rh.metrics.ScheduledRetries, -1)
		default:
			rh.log.Warn(fmt.Sprintf("retry queue full, rescheduling task: task_id=%s", task.ID))
			rh.mu.Lock()
			rh.scheduledTasks[task.ID] = task
			rh.mu.Unlock()
		}
	}
}

func (rh *RetryHandler) retryWorker(ctx context.Context, workerID int) {
	defer rh.wg.Done()

	rh.log.Debug(fmt.Sprintf("retry worker started: worker_id=%d", workerID))

	for {
		select {
		case <-ctx.Done():
			return
		case <-rh.stopChan:
			return
		case task, ok := <-rh.retryQueue:
			if !ok {
				return
			}
			rh.executeRetry(ctx, task)
		}
	}
}

func (rh *RetryHandler) executeRetry(ctx context.Context, task *RetryTask) {
	rh.log.Debug(fmt.Sprintf("executing retry: task_id=%s, attempt=%d, max_attempts=%d",
		task.ID, task.Attempt+1, task.MaxAttempts))

	rh.metrics.mu.Lock()
	rh.metrics.LastRetryAt = time.Now()
	rh.metrics.mu.Unlock()
}

func (rh *RetryHandler) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	for _, nonRetryable := range defaultNonRetryableErrors {
		if errStr == nonRetryable.Error() {
			return false
		}
	}

	return true
}

func (rh *RetryHandler) GetMetrics() *RetryMetrics {
	rh.metrics.mu.RLock()
	defer rh.metrics.mu.RUnlock()

	totalRetries := atomic.LoadInt64(&rh.metrics.TotalRetries)
	successfulRetries := atomic.LoadInt64(&rh.metrics.SuccessfulRetries)

	var successRate float64
	if totalRetries > 0 {
		successRate = float64(successfulRetries) / float64(totalRetries) * 100
	}

	return &RetryMetrics{
		TotalRetries:      totalRetries,
		SuccessfulRetries: successfulRetries,
		FailedRetries:     atomic.LoadInt64(&rh.metrics.FailedRetries),
		ScheduledRetries:  atomic.LoadInt64(&rh.metrics.ScheduledRetries),
		ExhaustedRetries:  atomic.LoadInt64(&rh.metrics.ExhaustedRetries),
		AverageRetryDelay: rh.metrics.AverageRetryDelay,
		TotalRetryDelay:   rh.metrics.TotalRetryDelay,
		RetrySuccessRate:  successRate,
		LastRetryAt:       rh.metrics.LastRetryAt,
	}
}

func (rh *RetryHandler) GetScheduledCount() int {
	rh.mu.RLock()
	defer rh.mu.RUnlock()
	return len(rh.scheduledTasks)
}

func (rh *RetryHandler) ClearScheduled() {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.scheduledTasks = make(map[string]*RetryTask)
	atomic.StoreInt64(&rh.metrics.ScheduledRetries, 0)
}

func (rh *RetryHandler) IsRunning() bool {
	return rh.running.Load()
}
