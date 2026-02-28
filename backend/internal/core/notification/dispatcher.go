package notification

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"

    "email-campaign-system/pkg/logger"
)

var (
    ErrHandlerNotFound     = errors.New("event handler not found")
    ErrInvalidEvent        = errors.New("invalid event")
    ErrDispatcherClosed    = errors.New("dispatcher is closed")
    ErrEventQueueFull      = errors.New("event queue is full")
)

type Dispatcher struct {
    mu               sync.RWMutex
    manager          *NotificationManager
    handlers         map[EventType][]EventHandler
    eventQueue       chan *Event
    logger           logger.Logger
    config           *DispatcherConfig
    middleware       []DispatcherMiddleware
    eventHistory     *EventHistory
    filters          []EventFilter
    transformers     map[EventType]EventTransformer
    subscribers      map[string]*Subscriber
    shutdownCh       chan struct{}
    wg               sync.WaitGroup
    metrics          *DispatcherMetrics
}

type DispatcherConfig struct {
    QueueSize         int
    WorkerCount       int
    EnableHistory     bool
    HistorySize       int
    EnableDebounce    bool
    DebounceInterval  time.Duration
    EnableBatching    bool
    BatchSize         int
    BatchTimeout      time.Duration
    EnableFiltering   bool
    MaxRetries        int
    RetryDelay        time.Duration
}

type Event struct {
    ID        string
    Type      EventType
    Timestamp time.Time
    Source    string
    Data      map[string]interface{}
    Priority  int
    Metadata  map[string]string
    Retries   int
}

type EventHandler func(ctx context.Context, event *Event) error

type EventFilter func(event *Event) bool

type EventTransformer func(event *Event) (*Notification, error)

type DispatcherMiddleware func(next EventHandler) EventHandler

type Subscriber struct {
    ID           string
    Events       []EventType
    Handler      EventHandler
    Filter       EventFilter
    Priority     int
    CreatedAt    time.Time
    Active       bool
}

type EventHistory struct {
    mu      sync.RWMutex
    events  []*HistoryEntry
    maxSize int
    index   int
}

type HistoryEntry struct {
    Event       *Event
    ProcessedAt time.Time
    Success     bool
    Error       error
    Duration    time.Duration
}

type DispatcherMetrics struct {
    mu                sync.RWMutex
    TotalDispatched   int64
    TotalFailed       int64
    ByEventType       map[EventType]int64
    AverageLatency    time.Duration
    LastDispatch      time.Time
}

type EventBatch struct {
    Events    []*Event
    CreatedAt time.Time
}

func NewDispatcher(manager *NotificationManager) *Dispatcher {
    config := DefaultDispatcherConfig()

    d := &Dispatcher{
        manager:      manager,
        handlers:     make(map[EventType][]EventHandler),
        eventQueue:   make(chan *Event, config.QueueSize),
        logger:       manager.logger,
        config:       config,
        transformers: make(map[EventType]EventTransformer),
        subscribers:  make(map[string]*Subscriber),
        shutdownCh:   make(chan struct{}),
        metrics: &DispatcherMetrics{
            ByEventType: make(map[EventType]int64),
        },
    }

    if config.EnableHistory {
        d.eventHistory = NewEventHistory(config.HistorySize)
    }

    d.registerDefaultTransformers()
    d.registerDefaultFilters()

    return d
}

func DefaultDispatcherConfig() *DispatcherConfig {
    return &DispatcherConfig{
        QueueSize:        1000,
        WorkerCount:      5,
        EnableHistory:    true,
        HistorySize:      1000,
        EnableDebounce:   false,
        DebounceInterval: 5 * time.Second,
        EnableBatching:   false,
        BatchSize:        10,
        BatchTimeout:     30 * time.Second,
        EnableFiltering:  true,
        MaxRetries:       3,
        RetryDelay:       2 * time.Second,
    }
}

func (d *Dispatcher) Start(ctx context.Context) error {
    d.logger.Info("starting event dispatcher", logger.Int("workers", d.config.WorkerCount))

    for i := 0; i < d.config.WorkerCount; i++ {
        d.wg.Add(1)
        go d.worker(ctx, i)
    }

    if d.config.EnableBatching {
        d.wg.Add(1)
        go d.batchProcessor(ctx)
    }

    return nil
}

func (d *Dispatcher) Stop() error {
    d.logger.Info("stopping event dispatcher")

    close(d.shutdownCh)
    close(d.eventQueue)
    d.wg.Wait()

    return nil
}

func (d *Dispatcher) worker(ctx context.Context, id int) {
    defer d.wg.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case <-d.shutdownCh:
            return
        case event, ok := <-d.eventQueue:
            if !ok {
                return
            }

            if err := d.processEvent(ctx, event); err != nil {
                d.logger.Error("failed to process event",
                    logger.Int("worker", id),
                    logger.String("event_type", string(event.Type)),
                    logger.Error(err))
                d.handleEventError(ctx, event, err)
            }
        }
    }
}

func (d *Dispatcher) Dispatch(ctx context.Context, event *Event) error {
    if event == nil {
        return ErrInvalidEvent
    }

    if event.ID == "" {
        event.ID = generateEventID()
    }

    if event.Timestamp.IsZero() {
        event.Timestamp = time.Now()
    }

    if d.config.EnableFiltering && !d.shouldDispatch(event) {
        d.logger.Debug("event filtered out", logger.String("event_type", string(event.Type)))
        return nil
    }

    select {
    case d.eventQueue <- event:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(5 * time.Second):
        return ErrEventQueueFull
    }
}

func (d *Dispatcher) DispatchBatch(ctx context.Context, events []*Event) error {
    for _, event := range events {
        if err := d.Dispatch(ctx, event); err != nil {
            d.logger.Error("failed to dispatch event in batch", logger.Error(err))
        }
    }

    return nil
}

func (d *Dispatcher) processEvent(ctx context.Context, event *Event) error {
    startTime := time.Now()

    if d.config.EnableHistory {
        defer func() {
            d.recordHistory(event, time.Since(startTime), nil)
        }()
    }

    d.mu.RLock()
    handlers := d.handlers[event.Type]
    subscribers := d.getSubscribersForEvent(event)
    d.mu.RUnlock()

    if len(handlers) == 0 && len(subscribers) == 0 {
        notification, err := d.transformEvent(event)
        if err != nil {
            return err
        }

        if notification != nil {
            return d.manager.Send(ctx, notification)
        }

        return nil
    }

    var lastErr error

    for _, handler := range handlers {
        wrappedHandler := d.applyMiddleware(handler)
        if err := wrappedHandler(ctx, event); err != nil {
            lastErr = err
            d.logger.Error("handler failed", logger.String("event_type", string(event.Type)), logger.Error(err))
        }
    }

    for _, sub := range subscribers {
        if !sub.Active {
            continue
        }

        if sub.Filter != nil && !sub.Filter(event) {
            continue
        }

        wrappedHandler := d.applyMiddleware(sub.Handler)
        if err := wrappedHandler(ctx, event); err != nil {
            lastErr = err
            d.logger.Error("subscriber handler failed",
                logger.String("subscriber_id", sub.ID),
                logger.String("event_type", string(event.Type)),
                logger.Error(err))
        }
    }

    if lastErr == nil {
        d.recordSuccess(event, time.Since(startTime))
    } else {
        d.recordFailure(event)
    }

    return lastErr
}

func (d *Dispatcher) RegisterHandler(eventType EventType, handler EventHandler) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.handlers[eventType] = append(d.handlers[eventType], handler)
    d.logger.Debug("registered event handler", logger.String("event_type", string(eventType)))
}

func (d *Dispatcher) UnregisterHandler(eventType EventType) {
    d.mu.Lock()
    defer d.mu.Unlock()

    delete(d.handlers, eventType)
    d.logger.Debug("unregistered event handler", logger.String("event_type", string(eventType)))
}

func (d *Dispatcher) Subscribe(subscriber *Subscriber) error {
    if subscriber == nil {
        return errors.New("subscriber is nil")
    }

    if subscriber.ID == "" {
        subscriber.ID = generateSubscriberID()
    }

    subscriber.CreatedAt = time.Now()
    subscriber.Active = true

    d.mu.Lock()
    defer d.mu.Unlock()

    d.subscribers[subscriber.ID] = subscriber
    d.logger.Info("subscriber registered", logger.String("subscriber_id", subscriber.ID))

    return nil
}

func (d *Dispatcher) Unsubscribe(subscriberID string) error {
    d.mu.Lock()
    defer d.mu.Unlock()

    if _, exists := d.subscribers[subscriberID]; !exists {
        return errors.New("subscriber not found")
    }

    delete(d.subscribers, subscriberID)
    d.logger.Info("subscriber unregistered", logger.String("subscriber_id", subscriberID))

    return nil
}

func (d *Dispatcher) getSubscribersForEvent(event *Event) []*Subscriber {
    var matching []*Subscriber

    for _, sub := range d.subscribers {
        if !sub.Active {
            continue
        }

        for _, eventType := range sub.Events {
            if eventType == event.Type {
                matching = append(matching, sub)
                break
            }
        }
    }

    return matching
}

func (d *Dispatcher) RegisterTransformer(eventType EventType, transformer EventTransformer) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.transformers[eventType] = transformer
    d.logger.Debug("registered event transformer", logger.String("event_type", string(eventType)))
}

func (d *Dispatcher) transformEvent(event *Event) (*Notification, error) {
    d.mu.RLock()
    transformer, exists := d.transformers[event.Type]
    d.mu.RUnlock()

    if !exists {
        return d.defaultTransform(event), nil
    }

    return transformer(event)
}

func (d *Dispatcher) defaultTransform(event *Event) *Notification {
    level := LevelInfo
    
    switch event.Type {
    case EventCampaignFailed, EventSendFailed, EventSystemError:
        level = LevelError
    case EventAccountSuspended, EventSystemWarning:
        level = LevelWarning
    case EventCampaignCompleted, EventSendSuccess:
        level = LevelSuccess
    }

    return &Notification{
        Level:     level,
        Event:     event.Type,
        Title:     formatEventTitle(event.Type),
        Message:   formatEventMessage(event),
        Data:      event.Data,
        Timestamp: event.Timestamp,
        Priority:  event.Priority,
    }
}

func (d *Dispatcher) AddFilter(filter EventFilter) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.filters = append(d.filters, filter)
}

func (d *Dispatcher) shouldDispatch(event *Event) bool {
    for _, filter := range d.filters {
        if !filter(event) {
            return false
        }
    }
    return true
}

func (d *Dispatcher) AddMiddleware(middleware DispatcherMiddleware) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.middleware = append(d.middleware, middleware)
}

func (d *Dispatcher) applyMiddleware(handler EventHandler) EventHandler {
    for i := len(d.middleware) - 1; i >= 0; i-- {
        handler = d.middleware[i](handler)
    }
    return handler
}

func (d *Dispatcher) handleEventError(ctx context.Context, event *Event, err error) {
    event.Retries++

    if event.Retries < d.config.MaxRetries {
        d.logger.Warn("retrying event",
            logger.String("event_id", event.ID),
            logger.Int("retry", event.Retries),
            logger.Error(err))

        time.Sleep(d.config.RetryDelay * time.Duration(event.Retries))

        if dispatchErr := d.Dispatch(ctx, event); dispatchErr != nil {
            d.logger.Error("failed to redispatch event", logger.Error(dispatchErr))
        }
    } else {
        d.logger.Error("event failed after max retries",
            logger.String("event_id", event.ID),
            logger.Int("retries", event.Retries),
            logger.Error(err))
    }
}

func (d *Dispatcher) batchProcessor(ctx context.Context) {
    defer d.wg.Done()

    ticker := time.NewTicker(d.config.BatchTimeout)
    defer ticker.Stop()

    batch := &EventBatch{
        Events:    make([]*Event, 0, d.config.BatchSize),
        CreatedAt: time.Now(),
    }

    for {
        select {
        case <-ctx.Done():
            d.processBatch(ctx, batch)
            return
        case <-d.shutdownCh:
            d.processBatch(ctx, batch)
            return
        case <-ticker.C:
            if len(batch.Events) > 0 {
                d.processBatch(ctx, batch)
                batch = &EventBatch{
                    Events:    make([]*Event, 0, d.config.BatchSize),
                    CreatedAt: time.Now(),
                }
            }
        }
    }
}

func (d *Dispatcher) processBatch(ctx context.Context, batch *EventBatch) {
    if len(batch.Events) == 0 {
        return
    }

    d.logger.Debug("processing event batch", logger.Int("size", len(batch.Events)))

    for _, event := range batch.Events {
        if err := d.processEvent(ctx, event); err != nil {
            d.logger.Error("failed to process batched event", logger.Error(err))
        }
    }
}

func (d *Dispatcher) GetHistory() []*HistoryEntry {
    if d.eventHistory == nil {
        return nil
    }

    return d.eventHistory.GetAll()
}

func (d *Dispatcher) GetMetrics() *DispatcherMetrics {
    d.metrics.mu.RLock()
    defer d.metrics.mu.RUnlock()

    return &DispatcherMetrics{
        TotalDispatched: d.metrics.TotalDispatched,
        TotalFailed:     d.metrics.TotalFailed,
        ByEventType:     copyEventTypeMap(d.metrics.ByEventType),
        AverageLatency:  d.metrics.AverageLatency,
        LastDispatch:    d.metrics.LastDispatch,
    }
}

func (d *Dispatcher) recordSuccess(event *Event, latency time.Duration) {
    d.metrics.mu.Lock()
    defer d.metrics.mu.Unlock()

    d.metrics.TotalDispatched++
    d.metrics.ByEventType[event.Type]++
    d.metrics.LastDispatch = time.Now()

    if d.metrics.AverageLatency == 0 {
        d.metrics.AverageLatency = latency
    } else {
        d.metrics.AverageLatency = (d.metrics.AverageLatency + latency) / 2
    }
}

func (d *Dispatcher) recordFailure(event *Event) {
    d.metrics.mu.Lock()
    defer d.metrics.mu.Unlock()

    d.metrics.TotalFailed++
}

func (d *Dispatcher) recordHistory(event *Event, duration time.Duration, err error) {
    if d.eventHistory == nil {
        return
    }

    entry := &HistoryEntry{
        Event:       event,
        ProcessedAt: time.Now(),
        Success:     err == nil,
        Error:       err,
        Duration:    duration,
    }

    d.eventHistory.Add(entry)
}

func (d *Dispatcher) registerDefaultTransformers() {
    d.RegisterTransformer(EventCampaignStarted, d.transformCampaignStarted)
    d.RegisterTransformer(EventCampaignCompleted, d.transformCampaignCompleted)
    d.RegisterTransformer(EventCampaignFailed, d.transformCampaignFailed)
    d.RegisterTransformer(EventAccountSuspended, d.transformAccountSuspended)
}

func (d *Dispatcher) registerDefaultFilters() {
    d.AddFilter(func(event *Event) bool {
        return event.Type != ""
    })
}

func (d *Dispatcher) transformCampaignStarted(event *Event) (*Notification, error) {
    return &Notification{
        Level:     LevelInfo,
        Event:     EventCampaignStarted,
        Title:     "Campaign Started",
        Message:   fmt.Sprintf("Campaign '%v' has started", event.Data["campaign_name"]),
        Data:      event.Data,
        Timestamp: event.Timestamp,
        Priority:  5,
    }, nil
}

func (d *Dispatcher) transformCampaignCompleted(event *Event) (*Notification, error) {
    return &Notification{
        Level:     LevelSuccess,
        Event:     EventCampaignCompleted,
        Title:     "Campaign Completed",
        Message:   fmt.Sprintf("Campaign '%v' completed successfully", event.Data["campaign_name"]),
        Data:      event.Data,
        Timestamp: event.Timestamp,
        Priority:  7,
    }, nil
}

func (d *Dispatcher) transformCampaignFailed(event *Event) (*Notification, error) {
    return &Notification{
        Level:     LevelError,
        Event:     EventCampaignFailed,
        Title:     "Campaign Failed",
        Message:   fmt.Sprintf("Campaign '%v' failed: %v", event.Data["campaign_name"], event.Data["error"]),
        Data:      event.Data,
        Timestamp: event.Timestamp,
        Priority:  10,
    }, nil
}

func (d *Dispatcher) transformAccountSuspended(event *Event) (*Notification, error) {
    return &Notification{
        Level:     LevelWarning,
        Event:     EventAccountSuspended,
        Title:     "Account Suspended",
        Message:   fmt.Sprintf("Account '%v' suspended: %v", event.Data["account_email"], event.Data["reason"]),
        Data:      event.Data,
        Timestamp: event.Timestamp,
        Priority:  8,
    }, nil
}

func NewEventHistory(maxSize int) *EventHistory {
    return &EventHistory{
        events:  make([]*HistoryEntry, 0, maxSize),
        maxSize: maxSize,
        index:   0,
    }
}

func (eh *EventHistory) Add(entry *HistoryEntry) {
    eh.mu.Lock()
    defer eh.mu.Unlock()

    if len(eh.events) < eh.maxSize {
        eh.events = append(eh.events, entry)
    } else {
        eh.events[eh.index] = entry
        eh.index = (eh.index + 1) % eh.maxSize
    }
}

func (eh *EventHistory) GetAll() []*HistoryEntry {
    eh.mu.RLock()
    defer eh.mu.RUnlock()

    result := make([]*HistoryEntry, len(eh.events))
    copy(result, eh.events)
    return result
}

func (eh *EventHistory) Clear() {
    eh.mu.Lock()
    defer eh.mu.Unlock()

    eh.events = make([]*HistoryEntry, 0, eh.maxSize)
    eh.index = 0
}

func formatEventTitle(eventType EventType) string {
    switch eventType {
    case EventCampaignStarted:
        return "Campaign Started"
    case EventCampaignPaused:
        return "Campaign Paused"
    case EventCampaignResumed:
        return "Campaign Resumed"
    case EventCampaignCompleted:
        return "Campaign Completed"
    case EventCampaignFailed:
        return "Campaign Failed"
    case EventAccountSuspended:
        return "Account Suspended"
    case EventAccountRestored:
        return "Account Restored"
    case EventSendSuccess:
        return "Email Sent"
    case EventSendFailed:
        return "Email Send Failed"
    case EventQuotaReached:
        return "Quota Reached"
    case EventSystemError:
        return "System Error"
    case EventSystemWarning:
        return "System Warning"
    default:
        return string(eventType)
    }
}

func formatEventMessage(event *Event) string {
    if msg, ok := event.Data["message"].(string); ok {
        return msg
    }

    return fmt.Sprintf("Event: %s", event.Type)
}

func generateEventID() string {
    return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

func generateSubscriberID() string {
    return fmt.Sprintf("sub_%d", time.Now().UnixNano())
}

func copyEventTypeMap(src map[EventType]int64) map[EventType]int64 {
    dst := make(map[EventType]int64, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func LoggingMiddleware(log logger.Logger) DispatcherMiddleware {
    return func(next EventHandler) EventHandler {
        return func(ctx context.Context, event *Event) error {
            log.Debug("handling event", logger.String("event_type", string(event.Type)), logger.String("event_id", event.ID))
            err := next(ctx, event)
            if err != nil {
                log.Error("event handler failed", logger.String("event_type", string(event.Type)), logger.Error(err))
            }
            return err
        }
    }
}

func RecoveryMiddleware(log logger.Logger) DispatcherMiddleware {
    return func(next EventHandler) EventHandler {
        return func(ctx context.Context, event *Event) (err error) {
            defer func() {
                if r := recover(); r != nil {
                    log.Error("panic in event handler", logger.String("event_type", string(event.Type)), logger.Any("panic", r))
                    err = fmt.Errorf("panic: %v", r)
                }
            }()
            return next(ctx, event)
        }
    }
}

func TimeoutMiddleware(timeout time.Duration) DispatcherMiddleware {
    return func(next EventHandler) EventHandler {
        return func(ctx context.Context, event *Event) error {
            ctx, cancel := context.WithTimeout(ctx, timeout)
            defer cancel()

            errCh := make(chan error, 1)
            go func() {
                errCh <- next(ctx, event)
            }()

            select {
            case err := <-errCh:
                return err
            case <-ctx.Done():
                return fmt.Errorf("event handler timeout: %w", ctx.Err())
            }
        }
    }
}
