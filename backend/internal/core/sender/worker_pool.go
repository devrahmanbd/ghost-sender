package sender

import (
        "context"
        "errors"
        "fmt"
        "sync"
        "sync/atomic"
        "time"

        "email-campaign-system/pkg/logger"
)

var (
        ErrPoolNotRunning = errors.New("worker pool is not running")
        ErrPoolShutdown   = errors.New("worker pool is shutting down")
        ErrJobTimeout     = errors.New("job execution timeout")
        ErrWorkerPanic    = errors.New("worker panic")
)

type WorkerPool struct {
        workers    []*Worker
        jobChan    chan interface{}
        jobHandler JobHandler
        config     WorkerPoolConfig
        log        logger.Logger
        running    atomic.Bool
        wg         sync.WaitGroup
        stopChan   chan struct{}
        mu         sync.RWMutex
        metrics    *WorkerPoolMetrics
}

type WorkerPoolConfig struct {
        WorkerCount    int
        JobChannelSize int
        JobTimeout     time.Duration
        IdleTimeout    time.Duration
        EnableMetrics  bool
        MaxJobRetries  int
        PanicRecovery  bool
}

type Worker struct {
        id            int
        pool          *WorkerPool
        state         WorkerState
        currentJob    interface{}
        jobsProcessed int64
        jobsFailed    int64
        lastActive    time.Time
        startTime     time.Time
        mu            sync.RWMutex
        stopChan      chan struct{}
        stopped       atomic.Bool
}

type WorkerState string

const (
        WorkerStateIdle       WorkerState = "idle"
        WorkerStateBusy       WorkerState = "busy"
        WorkerStateStopped    WorkerState = "stopped"
        WorkerStateTerminated WorkerState = "terminated"
)

type JobHandler func(ctx context.Context, job interface{}) error

type WorkerPoolMetrics struct {
        TotalJobsProcessed int64
        TotalJobsFailed    int64
        TotalJobsTimedOut  int64
        TotalPanics        int64
        ActiveWorkers      int64
        IdleWorkers        int64
        BusyWorkers        int64
        AverageJobDuration time.Duration
        TotalJobDuration   time.Duration
        LastJobCompletedAt time.Time
        mu                 sync.RWMutex
}

func NewWorkerPool(config WorkerPoolConfig, log logger.Logger) *WorkerPool {
        if config.WorkerCount <= 0 {
                config.WorkerCount = 4
        }
        if config.JobChannelSize <= 0 {
                config.JobChannelSize = 100
        }
        if config.JobTimeout <= 0 {
                config.JobTimeout = 30 * time.Second
        }
        if config.IdleTimeout <= 0 {
                config.IdleTimeout = 60 * time.Second
        }

        wp := &WorkerPool{
                workers:  make([]*Worker, config.WorkerCount),
                jobChan:  make(chan interface{}, config.JobChannelSize),
                config:   config,
                log:      log,
                stopChan: make(chan struct{}),
                metrics:  &WorkerPoolMetrics{},
        }

        for i := 0; i < config.WorkerCount; i++ {
                wp.workers[i] = &Worker{
                        id:        i,
                        pool:      wp,
                        state:     WorkerStateIdle,
                        stopChan:  make(chan struct{}),
                        startTime: time.Now(),
                }
        }

        return wp
}

func (wp *WorkerPool) Reset() {
        wp.mu.Lock()
        defer wp.mu.Unlock()

        wp.jobChan = make(chan interface{}, wp.config.JobChannelSize)
        wp.stopChan = make(chan struct{})
        wp.metrics = &WorkerPoolMetrics{}

        for i := 0; i < wp.config.WorkerCount; i++ {
                wp.workers[i] = &Worker{
                        id:        i,
                        pool:      wp,
                        state:     WorkerStateIdle,
                        stopChan:  make(chan struct{}),
                        startTime: time.Now(),
                }
        }
}

func (wp *WorkerPool) Start(ctx context.Context, handler JobHandler) error {
        if wp.running.Load() {
                return errors.New("worker pool already running")
        }

        if handler == nil {
                return errors.New("job handler is required")
        }

        wp.Reset()

        wp.jobHandler = handler
        wp.running.Store(true)

        for _, worker := range wp.workers {
                wp.wg.Add(1)
                go worker.run(ctx)
        }

        if wp.config.EnableMetrics {
                go wp.metricsUpdateLoop(ctx)
        }

        wp.log.Info(fmt.Sprintf("worker pool started: worker_count=%d", wp.config.WorkerCount))

        return nil
}

func (wp *WorkerPool) Stop(ctx context.Context) error {
        if !wp.running.Load() {
                return ErrPoolNotRunning
        }

        wp.running.Store(false)
        close(wp.stopChan)
        close(wp.jobChan)

        stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()

        done := make(chan struct{})
        go func() {
                wp.wg.Wait()
                close(done)
        }()

        select {
        case <-done:
                wp.log.Info("worker pool stopped gracefully")
                return nil
        case <-stopCtx.Done():
                wp.log.Warn("worker pool stop timeout, forcing shutdown")
                for _, worker := range wp.workers {
                        worker.forceStop()
                }
                return errors.New("worker pool stop timeout")
        }
}

func (wp *WorkerPool) Submit(job interface{}) error {
        if !wp.running.Load() {
                return ErrPoolNotRunning
        }

        select {
        case wp.jobChan <- job:
                return nil
        case <-wp.stopChan:
                return ErrPoolShutdown
        default:
                return errors.New("job channel full")
        }
}

func (wp *WorkerPool) SubmitWithTimeout(ctx context.Context, job interface{}, timeout time.Duration) error {
        if !wp.running.Load() {
                return ErrPoolNotRunning
        }

        timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
        defer cancel()

        select {
        case wp.jobChan <- job:
                return nil
        case <-timeoutCtx.Done():
                return fmt.Errorf("submit timeout: %w", timeoutCtx.Err())
        case <-wp.stopChan:
                return ErrPoolShutdown
        }
}

func (wp *WorkerPool) GetMetrics() *WorkerPoolMetrics {
        wp.metrics.mu.RLock()
        defer wp.metrics.mu.RUnlock()

        return &WorkerPoolMetrics{
                TotalJobsProcessed: atomic.LoadInt64(&wp.metrics.TotalJobsProcessed),
                TotalJobsFailed:    atomic.LoadInt64(&wp.metrics.TotalJobsFailed),
                TotalJobsTimedOut:  atomic.LoadInt64(&wp.metrics.TotalJobsTimedOut),
                TotalPanics:        atomic.LoadInt64(&wp.metrics.TotalPanics),
                ActiveWorkers:      atomic.LoadInt64(&wp.metrics.ActiveWorkers),
                IdleWorkers:        atomic.LoadInt64(&wp.metrics.IdleWorkers),
                BusyWorkers:        atomic.LoadInt64(&wp.metrics.BusyWorkers),
                AverageJobDuration: func() time.Duration { wp.metrics.mu.RLock(); defer wp.metrics.mu.RUnlock(); return wp.metrics.AverageJobDuration }(),
                TotalJobDuration:   func() time.Duration { wp.metrics.mu.RLock(); defer wp.metrics.mu.RUnlock(); return wp.metrics.TotalJobDuration }(),
                LastJobCompletedAt: func() time.Time { wp.metrics.mu.RLock(); defer wp.metrics.mu.RUnlock(); return wp.metrics.LastJobCompletedAt }(),
        }
}

func (wp *WorkerPool) Utilization() float64 {
        busyWorkers := atomic.LoadInt64(&wp.metrics.BusyWorkers)
        totalWorkers := int64(wp.config.WorkerCount)

        if totalWorkers == 0 {
                return 0
        }

        return float64(busyWorkers) / float64(totalWorkers) * 100
}

func (wp *WorkerPool) IsRunning() bool {
        return wp.running.Load()
}

func (wp *WorkerPool) WorkerCount() int {
        return wp.config.WorkerCount
}

func (wp *WorkerPool) GetWorkerStates() map[int]WorkerState {
        wp.mu.RLock()
        defer wp.mu.RUnlock()

        states := make(map[int]WorkerState)
        for _, worker := range wp.workers {
                worker.mu.RLock()
                states[worker.id] = worker.state
                worker.mu.RUnlock()
        }

        return states
}

func (wp *WorkerPool) metricsUpdateLoop(ctx context.Context) {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()

        for {
                select {
                case <-ctx.Done():
                        return
                case <-wp.stopChan:
                        return
                case <-ticker.C:
                        wp.updateMetrics()
                }
        }
}

func (wp *WorkerPool) updateMetrics() {
        var active, idle, busy int64

        for _, worker := range wp.workers {
                worker.mu.RLock()
                state := worker.state
                worker.mu.RUnlock()

                switch state {
                case WorkerStateBusy:
                        busy++
                        active++
                case WorkerStateIdle:
                        idle++
                        active++
                }
        }

        atomic.StoreInt64(&wp.metrics.ActiveWorkers, active)
        atomic.StoreInt64(&wp.metrics.IdleWorkers, idle)
        atomic.StoreInt64(&wp.metrics.BusyWorkers, busy)

        totalJobs := atomic.LoadInt64(&wp.metrics.TotalJobsProcessed)
        if totalJobs > 0 {
                wp.metrics.mu.Lock()
                wp.metrics.AverageJobDuration = time.Duration(
                        int64(wp.metrics.TotalJobDuration) / totalJobs,
                )
                wp.metrics.mu.Unlock()
        }
}

func (w *Worker) run(ctx context.Context) {
        defer w.pool.wg.Done()
        defer w.cleanup()

        w.setState(WorkerStateIdle)

        w.pool.log.Debug(fmt.Sprintf("worker started: worker_id=%d", w.id))

        for {
                select {
                case <-ctx.Done():
                        w.pool.log.Debug(fmt.Sprintf("worker context cancelled: worker_id=%d", w.id))
                        return
                case <-w.stopChan:
                        w.pool.log.Debug(fmt.Sprintf("worker stop signal received: worker_id=%d", w.id))
                        return
                case <-w.pool.stopChan:
                        w.pool.log.Debug(fmt.Sprintf("pool stop signal received: worker_id=%d", w.id))
                        return
                case job, ok := <-w.pool.jobChan:
                        if !ok {
                                w.pool.log.Debug(fmt.Sprintf("job channel closed: worker_id=%d", w.id))
                                return
                        }

                        w.processJob(ctx, job)
                }
        }
}

func (w *Worker) processJob(ctx context.Context, job interface{}) {
        w.setState(WorkerStateBusy)
        w.setCurrentJob(job)
        defer w.clearCurrentJob()
        defer w.setState(WorkerStateIdle)

        startTime := time.Now()

        if w.pool.config.PanicRecovery {
                defer w.recoverPanic()
        }

        jobCtx, cancel := context.WithTimeout(ctx, w.pool.config.JobTimeout)
        defer cancel()

        done := make(chan error, 1)

        go func() {
                done <- w.pool.jobHandler(jobCtx, job)
        }()

        select {
        case err := <-done:
                duration := time.Since(startTime)
                w.updateJobMetrics(duration, err)

                if err != nil {
                        w.pool.log.Error(fmt.Sprintf("job failed: worker_id=%d, error=%v, duration=%v",
                                w.id, err, duration))
                        atomic.AddInt64(&w.jobsFailed, 1)
                        atomic.AddInt64(&w.pool.metrics.TotalJobsFailed, 1)
                } else {
                        atomic.AddInt64(&w.jobsProcessed, 1)
                        atomic.AddInt64(&w.pool.metrics.TotalJobsProcessed, 1)
                }

        case <-jobCtx.Done():
                duration := time.Since(startTime)
                w.pool.log.Warn(fmt.Sprintf("job timeout: worker_id=%d, duration=%v, timeout=%v",
                        w.id, duration, w.pool.config.JobTimeout))
                atomic.AddInt64(&w.jobsFailed, 1)
                atomic.AddInt64(&w.pool.metrics.TotalJobsTimedOut, 1)
        }
}

func (w *Worker) recoverPanic() {
        if r := recover(); r != nil {
                w.pool.log.Error(fmt.Sprintf("worker panic recovered: worker_id=%d, panic=%v",
                        w.id, r))
                atomic.AddInt64(&w.pool.metrics.TotalPanics, 1)
        }
}

func (w *Worker) updateJobMetrics(duration time.Duration, err error) {
        w.mu.Lock()
        w.lastActive = time.Now()
        w.mu.Unlock()

        w.pool.metrics.mu.Lock()
        w.pool.metrics.TotalJobDuration += duration
        w.pool.metrics.LastJobCompletedAt = time.Now()
        w.pool.metrics.mu.Unlock()
}

func (w *Worker) setState(state WorkerState) {
        w.mu.Lock()
        w.state = state
        w.mu.Unlock()
}

func (w *Worker) getState() WorkerState {
        w.mu.RLock()
        defer w.mu.RUnlock()
        return w.state
}

func (w *Worker) setCurrentJob(job interface{}) {
        w.mu.Lock()
        w.currentJob = job
        w.lastActive = time.Now()
        w.mu.Unlock()
}

func (w *Worker) clearCurrentJob() {
        w.mu.Lock()
        w.currentJob = nil
        w.mu.Unlock()
}

func (w *Worker) forceStop() {
        if !w.stopped.Load() {
                w.stopped.Store(true)
                close(w.stopChan)
        }
}

func (w *Worker) cleanup() {
        w.setState(WorkerStateTerminated)
        w.pool.log.Debug(fmt.Sprintf("worker terminated: worker_id=%d, jobs_processed=%d, jobs_failed=%d, uptime=%v",
                w.id, atomic.LoadInt64(&w.jobsProcessed), atomic.LoadInt64(&w.jobsFailed), time.Since(w.startTime)))
}

func (w *Worker) GetStats() *WorkerStats {
        w.mu.RLock()
        defer w.mu.RUnlock()

        return &WorkerStats{
                ID:            w.id,
                State:         w.state,
                JobsProcessed: atomic.LoadInt64(&w.jobsProcessed),
                JobsFailed:    atomic.LoadInt64(&w.jobsFailed),
                LastActive:    w.lastActive,
                Uptime:        time.Since(w.startTime),
        }
}

type WorkerStats struct {
        ID            int
        State         WorkerState
        JobsProcessed int64
        JobsFailed    int64
        LastActive    time.Time
        Uptime        time.Duration
}
