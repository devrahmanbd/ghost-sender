package sender

import (
    "context"
    "errors"
    "sync"
    "sync/atomic"
    "time"

    "email-campaign-system/pkg/logger"
)

var (
    ErrBatchFull        = errors.New("batch is full")
    ErrBatchClosed      = errors.New("batch processor is closed")
    ErrBatchProcessing  = errors.New("batch is currently processing")
    ErrInvalidBatchSize = errors.New("invalid batch size")
)

type BatchProcessor struct {
    config       BatchConfig
    log          logger.Logger // Changed from *logger.Logger to logger.Logger
    currentBatch []interface{}
    mu           sync.RWMutex
    flushTimer   *time.Timer
    lastFlush    time.Time
    closed       atomic.Bool
    processing   atomic.Bool
    metrics      *BatchMetrics
    handlers     []BatchHandler
    stopChan     chan struct{}
    flushChan    chan struct{}
    wg           sync.WaitGroup
}

type BatchConfig struct {
    BatchSize       int
    FlushInterval   time.Duration
    MaxWaitTime     time.Duration
    EnableAutoFlush bool
    MaxRetries      int
}

type BatchHandler func(ctx context.Context, items []interface{}) error

type BatchMetrics struct {
    TotalBatches       int64
    TotalItems         int64
    SuccessfulBatches  int64
    FailedBatches      int64
    AverageBatchSize   float64
    AverageProcessTime time.Duration
    TotalProcessTime   time.Duration
    LastBatchAt        time.Time
    LastFlushAt        time.Time
    CurrentBatchSize   int64
    mu                 sync.RWMutex
}

func NewBatchProcessor(config BatchConfig, log logger.Logger) *BatchProcessor {
    if config.BatchSize <= 0 {
        config.BatchSize = 100
    }
    if config.FlushInterval <= 0 {
        config.FlushInterval = 10 * time.Second
    }
    if config.MaxWaitTime <= 0 {
        config.MaxWaitTime = 30 * time.Second
    }
    if config.MaxRetries <= 0 {
        config.MaxRetries = 3
    }

    bp := &BatchProcessor{
        config:       config,
        log:          log,
        currentBatch: make([]interface{}, 0, config.BatchSize),
        metrics:      &BatchMetrics{},
        handlers:     make([]BatchHandler, 0),
        stopChan:     make(chan struct{}),
        flushChan:    make(chan struct{}, 1),
        lastFlush:    time.Now(),
    }

    if config.EnableAutoFlush {
        bp.flushTimer = time.NewTimer(config.FlushInterval)
        bp.wg.Add(1)
        go bp.autoFlushLoop()
    }

    return bp
}

func (bp *BatchProcessor) Add(ctx context.Context, item interface{}) error {
    if bp.closed.Load() {
        return ErrBatchClosed
    }

    bp.mu.Lock()

    if len(bp.currentBatch) >= bp.config.BatchSize {
        batch := bp.currentBatch
        bp.currentBatch = make([]interface{}, 0, bp.config.BatchSize)
        bp.mu.Unlock()

        if err := bp.processBatch(ctx, batch); err != nil {
            bp.log.Error("failed to process full batch: " + err.Error())
        }

        bp.mu.Lock()
    }

    bp.currentBatch = append(bp.currentBatch, item)
    atomic.StoreInt64(&bp.metrics.CurrentBatchSize, int64(len(bp.currentBatch)))

    shouldFlush := len(bp.currentBatch) >= bp.config.BatchSize

    bp.mu.Unlock()

    if shouldFlush {
        select {
        case bp.flushChan <- struct{}{}:
        default:
        }
    }

    return nil
}

func (bp *BatchProcessor) AddBatch(ctx context.Context, items []interface{}) error {
    if bp.closed.Load() {
        return ErrBatchClosed
    }

    if len(items) == 0 {
        return nil
    }

    bp.mu.Lock()
    defer bp.mu.Unlock()

    for _, item := range items {
        bp.currentBatch = append(bp.currentBatch, item)

        if len(bp.currentBatch) >= bp.config.BatchSize {
            batch := bp.currentBatch
            bp.currentBatch = make([]interface{}, 0, bp.config.BatchSize)

            go func(b []interface{}) {
                if err := bp.processBatch(ctx, b); err != nil {
                    bp.log.Error("failed to process batch: " + err.Error())
                }
            }(batch)
        }
    }

    atomic.StoreInt64(&bp.metrics.CurrentBatchSize, int64(len(bp.currentBatch)))

    return nil
}

func (bp *BatchProcessor) Flush(ctx context.Context) error {
    if bp.closed.Load() {
        return ErrBatchClosed
    }

    bp.mu.Lock()

    if len(bp.currentBatch) == 0 {
        bp.mu.Unlock()
        return nil
    }

    batch := bp.currentBatch
    bp.currentBatch = make([]interface{}, 0, bp.config.BatchSize)
    atomic.StoreInt64(&bp.metrics.CurrentBatchSize, 0)

    bp.mu.Unlock()

    return bp.processBatch(ctx, batch)
}

func (bp *BatchProcessor) ForceFlush(ctx context.Context) error {
    return bp.Flush(ctx)
}

func (bp *BatchProcessor) processBatch(ctx context.Context, batch []interface{}) error {
    if len(batch) == 0 {
        return nil
    }

    if bp.processing.Load() {
        bp.log.Warn("batch already processing, queueing")
    }

    bp.processing.Store(true)
    defer bp.processing.Store(false)

    startTime := time.Now()

    bp.log.Debug("processing batch")

    var lastErr error
    for _, handler := range bp.handlers {
        if err := bp.executeWithRetry(ctx, handler, batch); err != nil {
            bp.log.Error("batch handler failed: " + err.Error())
            lastErr = err
        }
    }

    duration := time.Since(startTime)

    atomic.AddInt64(&bp.metrics.TotalBatches, 1)
    atomic.AddInt64(&bp.metrics.TotalItems, int64(len(batch)))

    bp.metrics.mu.Lock()
    bp.metrics.TotalProcessTime += duration
    bp.metrics.LastBatchAt = time.Now()
    bp.metrics.LastFlushAt = time.Now()

    totalBatches := atomic.LoadInt64(&bp.metrics.TotalBatches)
    if totalBatches > 0 {
        bp.metrics.AverageProcessTime = time.Duration(
            int64(bp.metrics.TotalProcessTime) / totalBatches,
        )
        bp.metrics.AverageBatchSize = float64(atomic.LoadInt64(&bp.metrics.TotalItems)) / float64(totalBatches)
    }
    bp.metrics.mu.Unlock()

    if lastErr != nil {
        atomic.AddInt64(&bp.metrics.FailedBatches, 1)
        return lastErr
    }

    atomic.AddInt64(&bp.metrics.SuccessfulBatches, 1)

    bp.log.Debug("batch processed")

    return nil
}

func (bp *BatchProcessor) executeWithRetry(ctx context.Context, handler BatchHandler, batch []interface{}) error {
    var lastErr error

    for attempt := 0; attempt <= bp.config.MaxRetries; attempt++ {
        if attempt > 0 {
            delay := time.Duration(attempt) * time.Second
            bp.log.Debug("retrying batch")

            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }

        if err := handler(ctx, batch); err != nil {
            lastErr = err
            bp.log.Warn("batch handler error: " + err.Error())
            continue
        }

        return nil
    }

    return lastErr
}

func (bp *BatchProcessor) autoFlushLoop() {
    defer bp.wg.Done()

    for {
        select {
        case <-bp.stopChan:
            return

        case <-bp.flushChan:
            ctx, cancel := context.WithTimeout(context.Background(), bp.config.MaxWaitTime)
            if err := bp.Flush(ctx); err != nil {
                bp.log.Error("auto flush failed: " + err.Error())
            }
            cancel()

        case <-bp.flushTimer.C:
            ctx, cancel := context.WithTimeout(context.Background(), bp.config.MaxWaitTime)
            if err := bp.Flush(ctx); err != nil {
                bp.log.Error("timer flush failed: " + err.Error())
            }
            cancel()

            bp.flushTimer.Reset(bp.config.FlushInterval)
        }
    }
}

func (bp *BatchProcessor) AddHandler(handler BatchHandler) {
    bp.mu.Lock()
    defer bp.mu.Unlock()
    bp.handlers = append(bp.handlers, handler)
}

func (bp *BatchProcessor) Size() int {
    return int(atomic.LoadInt64(&bp.metrics.CurrentBatchSize))
}

func (bp *BatchProcessor) IsEmpty() bool {
    return bp.Size() == 0
}

func (bp *BatchProcessor) IsClosed() bool {
    return bp.closed.Load()
}

func (bp *BatchProcessor) IsProcessing() bool {
    return bp.processing.Load()
}

func (bp *BatchProcessor) GetMetrics() *BatchMetrics {
    bp.metrics.mu.RLock()
    defer bp.metrics.mu.RUnlock()

    return &BatchMetrics{
        TotalBatches:       atomic.LoadInt64(&bp.metrics.TotalBatches),
        TotalItems:         atomic.LoadInt64(&bp.metrics.TotalItems),
        SuccessfulBatches:  atomic.LoadInt64(&bp.metrics.SuccessfulBatches),
        FailedBatches:      atomic.LoadInt64(&bp.metrics.FailedBatches),
        AverageBatchSize:   bp.metrics.AverageBatchSize,
        AverageProcessTime: bp.metrics.AverageProcessTime,
        TotalProcessTime:   bp.metrics.TotalProcessTime,
        LastBatchAt:        bp.metrics.LastBatchAt,
        LastFlushAt:        bp.metrics.LastFlushAt,
        CurrentBatchSize:   atomic.LoadInt64(&bp.metrics.CurrentBatchSize),
    }
}

func (bp *BatchProcessor) Close(ctx context.Context) error {
    if bp.closed.Swap(true) {
        return nil
    }

    close(bp.stopChan)

    if bp.flushTimer != nil {
        bp.flushTimer.Stop()
    }

    if err := bp.Flush(ctx); err != nil {
        bp.log.Error("final flush failed: " + err.Error())
    }

    bp.wg.Wait()

    bp.log.Info("batch processor closed")

    return nil
}

func (bp *BatchProcessor) getSuccessRate() float64 {
    totalBatches := atomic.LoadInt64(&bp.metrics.TotalBatches)
    if totalBatches == 0 {
        return 0
    }

    successfulBatches := atomic.LoadInt64(&bp.metrics.SuccessfulBatches)
    return float64(successfulBatches) / float64(totalBatches) * 100
}
