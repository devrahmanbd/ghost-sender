package campaign

import (
    "context"
    "errors"
    "fmt"
    "sort"
    "sync"
    "time"

    "email-campaign-system/internal/models"
    "email-campaign-system/internal/storage/repository"
    "email-campaign-system/pkg/logger"
)

var (
    ErrScheduleNotFound    = errors.New("schedule not found")
    ErrInvalidScheduleTime = errors.New("invalid schedule time")
    ErrScheduleInPast      = errors.New("schedule time is in the past")
    ErrScheduleExists      = errors.New("schedule already exists")
    ErrSchedulerStopped    = errors.New("scheduler is stopped")
)

type Scheduler struct {
    manager         *Manager
    repo            repository.CampaignRepository
    log             logger.Logger  // ✅ Fixed: Interface, not pointer
    config          SchedulerConfig
    schedules       map[string]*ScheduledCampaign
    mu              sync.RWMutex
    ticker          *time.Ticker
    stopChan        chan struct{}
    running         bool
    timezone        *time.Location
}

type SchedulerConfig struct {
    CheckInterval     time.Duration
    MaxScheduledItems int
    EnableRecurring   bool
    Timezone          string
    LookaheadWindow   time.Duration
    MaxRetries        int
}

type ScheduledCampaign struct {
    ID             string
    CampaignID     string
    Campaign       *models.Campaign
    ScheduledAt    time.Time
    ExecuteAt      time.Time
    Status         ScheduleStatus
    Recurrence     *RecurrenceRule
    RetryCount     int
    LastAttempt    time.Time
    NextExecution  time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
    mu             sync.RWMutex
}

type ScheduleStatus string

const (
    ScheduleStatusPending   ScheduleStatus = "pending"
    ScheduleStatusExecuting ScheduleStatus = "executing"
    ScheduleStatusCompleted ScheduleStatus = "completed"
    ScheduleStatusFailed    ScheduleStatus = "failed"
    ScheduleStatusCancelled ScheduleStatus = "cancelled"
)

type RecurrenceRule struct {
    Type     RecurrenceType
    Interval int
    DaysOfWeek []time.Weekday
    TimeOfDay  time.Time
    EndDate    *time.Time
    MaxOccurrences int
    Occurrences int
}

type RecurrenceType string

const (
    RecurrenceDaily   RecurrenceType = "daily"
    RecurrenceWeekly  RecurrenceType = "weekly"
    RecurrenceMonthly RecurrenceType = "monthly"
    RecurrenceCustom  RecurrenceType = "custom"
)

func NewScheduler(
    manager *Manager,
    repo repository.CampaignRepository,
    log logger.Logger,  // ✅ Fixed: Interface
    config SchedulerConfig,
) (*Scheduler, error) {
    if config.CheckInterval <= 0 {
        config.CheckInterval = 30 * time.Second
    }
    if config.MaxScheduledItems <= 0 {
        config.MaxScheduledItems = 1000
    }
    if config.LookaheadWindow <= 0 {
        config.LookaheadWindow = 5 * time.Minute
    }
    if config.MaxRetries <= 0 {
        config.MaxRetries = 3
    }
    if config.Timezone == "" {
        config.Timezone = "UTC"
    }

    tz, err := time.LoadLocation(config.Timezone)
    if err != nil {
        return nil, fmt.Errorf("invalid timezone: %w", err)
    }

    s := &Scheduler{
        manager:   manager,
        repo:      repo,
        log:       log,
        config:    config,
        schedules: make(map[string]*ScheduledCampaign),
        stopChan:  make(chan struct{}),
        timezone:  tz,
    }

    return s, nil
}

func (s *Scheduler) Start(ctx context.Context) error {
    s.mu.Lock()
    if s.running {
        s.mu.Unlock()
        return errors.New("scheduler already running")
    }
    s.running = true
    s.mu.Unlock()

    if err := s.loadSchedules(ctx); err != nil {
        s.log.Error("failed to load schedules", 
            logger.String("error", err.Error()))
    }

    s.ticker = time.NewTicker(s.config.CheckInterval)

    go s.run(ctx)

    s.log.Info("scheduler started", 
        logger.Duration("check_interval", s.config.CheckInterval))

    return nil
}

func (s *Scheduler) Stop(ctx context.Context) error {
    s.mu.Lock()
    if !s.running {
        s.mu.Unlock()
        return nil
    }
    s.running = false
    s.mu.Unlock()

    close(s.stopChan)

    if s.ticker != nil {
        s.ticker.Stop()
    }

    if err := s.persistAllSchedules(ctx); err != nil {
        s.log.Error("failed to persist schedules on shutdown", 
            logger.String("error", err.Error()))
    }

    s.log.Info("scheduler stopped")

    return nil
}

func (s *Scheduler) ScheduleCampaign(ctx context.Context, campaign *models.Campaign, executeAt time.Time, recurrence *RecurrenceRule) (*ScheduledCampaign, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if !s.running {
        return nil, ErrSchedulerStopped
    }

    if err := s.validateScheduleTime(executeAt); err != nil {
        return nil, err
    }

    if len(s.schedules) >= s.config.MaxScheduledItems {
        return nil, fmt.Errorf("maximum scheduled items reached: %d", s.config.MaxScheduledItems)
    }

    scheduleID := fmt.Sprintf("schedule-%s-%d", campaign.ID, time.Now().Unix())

    if _, exists := s.schedules[scheduleID]; exists {
        return nil, ErrScheduleExists
    }

    scheduled := &ScheduledCampaign{
        ID:            scheduleID,
        CampaignID:    campaign.ID,
        Campaign:      campaign,
        ScheduledAt:   time.Now(),
        ExecuteAt:     executeAt.In(s.timezone),
        Status:        ScheduleStatusPending,
        Recurrence:    recurrence,
        NextExecution: executeAt.In(s.timezone),
        CreatedAt:     time.Now(),
        UpdatedAt:     time.Now(),
    }

    s.schedules[scheduleID] = scheduled

    if err := s.persistSchedule(ctx, scheduled); err != nil {
        delete(s.schedules, scheduleID)
        return nil, fmt.Errorf("failed to persist schedule: %w", err)
    }

    s.log.Info("campaign scheduled",
        logger.String("schedule_id", scheduleID),
        logger.String("campaign_id", campaign.ID),
        logger.String("execute_at", executeAt.Format(time.RFC3339)),
    )

    return scheduled, nil
}

func (s *Scheduler) CancelSchedule(ctx context.Context, scheduleID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    scheduled, exists := s.schedules[scheduleID]
    if !exists {
        return ErrScheduleNotFound
    }

    scheduled.mu.Lock()
    scheduled.Status = ScheduleStatusCancelled
    scheduled.UpdatedAt = time.Now()
    scheduled.mu.Unlock()

    delete(s.schedules, scheduleID)

    s.log.Info("schedule cancelled", 
        logger.String("schedule_id", scheduleID))

    return nil
}

func (s *Scheduler) UpdateSchedule(ctx context.Context, scheduleID string, newExecuteAt time.Time) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    scheduled, exists := s.schedules[scheduleID]
    if !exists {
        return ErrScheduleNotFound
    }

    if err := s.validateScheduleTime(newExecuteAt); err != nil {
        return err
    }

    scheduled.mu.Lock()
    scheduled.ExecuteAt = newExecuteAt.In(s.timezone)
    scheduled.NextExecution = newExecuteAt.In(s.timezone)
    scheduled.UpdatedAt = time.Now()
    scheduled.mu.Unlock()

    if err := s.persistSchedule(ctx, scheduled); err != nil {
        return fmt.Errorf("failed to persist updated schedule: %w", err)
    }

    s.log.Info("schedule updated",
        logger.String("schedule_id", scheduleID),
        logger.String("new_execute_at", newExecuteAt.Format(time.RFC3339)),
    )

    return nil
}

func (s *Scheduler) GetSchedule(scheduleID string) (*ScheduledCampaign, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    scheduled, exists := s.schedules[scheduleID]
    if !exists {
        return nil, ErrScheduleNotFound
    }

    return scheduled, nil
}

func (s *Scheduler) ListScheduled() []*ScheduledCampaign {
    s.mu.RLock()
    defer s.mu.RUnlock()

    schedules := make([]*ScheduledCampaign, 0, len(s.schedules))
    for _, scheduled := range s.schedules {
        schedules = append(schedules, scheduled)
    }

    sort.Slice(schedules, func(i, j int) bool {
        return schedules[i].ExecuteAt.Before(schedules[j].ExecuteAt)
    })

    return schedules
}

func (s *Scheduler) GetNextScheduled(count int) []*ScheduledCampaign {
    s.mu.RLock()
    defer s.mu.RUnlock()

    now := time.Now()
    upcoming := make([]*ScheduledCampaign, 0)

    for _, scheduled := range s.schedules {
        scheduled.mu.RLock()
        if scheduled.Status == ScheduleStatusPending && scheduled.NextExecution.After(now) {
            upcoming = append(upcoming, scheduled)
        }
        scheduled.mu.RUnlock()
    }

    sort.Slice(upcoming, func(i, j int) bool {
        return upcoming[i].NextExecution.Before(upcoming[j].NextExecution)
    })

    if len(upcoming) > count {
        upcoming = upcoming[:count]
    }

    return upcoming
}

func (s *Scheduler) run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-s.stopChan:
            return
        case <-s.ticker.C:
            s.processSchedules(ctx)
        }
    }
}

func (s *Scheduler) processSchedules(ctx context.Context) {
    s.mu.RLock()
    now := time.Now()
    lookahead := now.Add(s.config.LookaheadWindow)

    toExecute := make([]*ScheduledCampaign, 0)
    for _, scheduled := range s.schedules {
        scheduled.mu.RLock()
        if scheduled.Status == ScheduleStatusPending &&
            scheduled.NextExecution.After(now) &&
            scheduled.NextExecution.Before(lookahead) {
            toExecute = append(toExecute, scheduled)
        }
        scheduled.mu.RUnlock()
    }
    s.mu.RUnlock()

    for _, scheduled := range toExecute {
        timeUntilExecution := time.Until(scheduled.NextExecution)
        if timeUntilExecution > 0 {
            time.Sleep(timeUntilExecution)
        }

        go s.triggerCampaign(ctx, scheduled)
    }
}

func (s *Scheduler) triggerCampaign(ctx context.Context, scheduled *ScheduledCampaign) {
    scheduled.mu.Lock()
    scheduled.Status = ScheduleStatusExecuting
    scheduled.LastAttempt = time.Now()
    scheduled.UpdatedAt = time.Now()
    scheduled.mu.Unlock()

    s.log.Info("triggering scheduled campaign",
        logger.String("schedule_id", scheduled.ID),
        logger.String("campaign_id", scheduled.CampaignID),
    )

    err := s.manager.StartCampaign(ctx, scheduled.CampaignID)

    scheduled.mu.Lock()
    if err != nil {
        s.log.Error("failed to trigger scheduled campaign",
            logger.String("schedule_id", scheduled.ID),
            logger.String("campaign_id", scheduled.CampaignID),
            logger.String("error", err.Error()),
        )

        scheduled.RetryCount++
        if scheduled.RetryCount >= s.config.MaxRetries {
            scheduled.Status = ScheduleStatusFailed
        } else {
            scheduled.Status = ScheduleStatusPending
            scheduled.NextExecution = time.Now().Add(time.Duration(scheduled.RetryCount) * time.Minute)
        }
    } else {
        if scheduled.Recurrence != nil && s.config.EnableRecurring {
            nextExecution := s.calculateNextExecution(scheduled)
            if nextExecution != nil {
                scheduled.Status = ScheduleStatusPending
                scheduled.NextExecution = *nextExecution
                scheduled.RetryCount = 0
                scheduled.Recurrence.Occurrences++
            } else {
                scheduled.Status = ScheduleStatusCompleted
            }
        } else {
            scheduled.Status = ScheduleStatusCompleted
        }
    }
    scheduled.UpdatedAt = time.Now()
    scheduled.mu.Unlock()

    if scheduled.Status == ScheduleStatusCompleted || scheduled.Status == ScheduleStatusFailed {
        s.mu.Lock()
        delete(s.schedules, scheduled.ID)
        s.mu.Unlock()
    }

    if err := s.persistSchedule(ctx, scheduled); err != nil {
        s.log.Error("failed to persist schedule after trigger", 
            logger.String("schedule_id", scheduled.ID), 
            logger.String("error", err.Error()))
    }
}

func (s *Scheduler) calculateNextExecution(scheduled *ScheduledCampaign) *time.Time {
    if scheduled.Recurrence == nil {
        return nil
    }

    rule := scheduled.Recurrence

    if rule.MaxOccurrences > 0 && rule.Occurrences >= rule.MaxOccurrences {
        return nil
    }

    if rule.EndDate != nil && time.Now().After(*rule.EndDate) {
        return nil
    }

    var next time.Time
    current := scheduled.NextExecution

    switch rule.Type {
    case RecurrenceDaily:
        next = current.AddDate(0, 0, rule.Interval)
    case RecurrenceWeekly:
        next = current.AddDate(0, 0, 7*rule.Interval)
    case RecurrenceMonthly:
        next = current.AddDate(0, rule.Interval, 0)
    default:
        return nil
    }

    if rule.TimeOfDay.Hour() != 0 || rule.TimeOfDay.Minute() != 0 {
        next = time.Date(next.Year(), next.Month(), next.Day(),
            rule.TimeOfDay.Hour(), rule.TimeOfDay.Minute(), 0, 0, s.timezone)
    }

    return &next
}

func (s *Scheduler) validateScheduleTime(t time.Time) error {
    now := time.Now()

    if t.Before(now) {
        return ErrScheduleInPast
    }

    if t.Sub(now) > 365*24*time.Hour {
        return fmt.Errorf("schedule time too far in the future")
    }

    return nil
}

func (s *Scheduler) persistSchedule(ctx context.Context, scheduled *ScheduledCampaign) error {
    return nil
}

func (s *Scheduler) loadSchedules(ctx context.Context) error {
    return nil
}

func (s *Scheduler) persistAllSchedules(ctx context.Context) error {
    s.mu.RLock()
    defer s.mu.RUnlock()

    for _, scheduled := range s.schedules {
        if err := s.persistSchedule(ctx, scheduled); err != nil {
            s.log.Error("failed to persist schedule", 
                logger.String("schedule_id", scheduled.ID), 
                logger.String("error", err.Error()))
        }
    }

    return nil
}

func (sc *ScheduledCampaign) IsExpired() bool {
    sc.mu.RLock()
    defer sc.mu.RUnlock()

    if sc.Status == ScheduleStatusCompleted || sc.Status == ScheduleStatusCancelled {
        return true
    }

    if sc.Recurrence != nil {
        if sc.Recurrence.EndDate != nil && time.Now().After(*sc.Recurrence.EndDate) {
            return true
        }
        if sc.Recurrence.MaxOccurrences > 0 && sc.Recurrence.Occurrences >= sc.Recurrence.MaxOccurrences {
            return true
        }
    }

    return false
}

func (sc *ScheduledCampaign) CanExecute() bool {
    sc.mu.RLock()
    defer sc.mu.RUnlock()

    return sc.Status == ScheduleStatusPending && time.Now().After(sc.NextExecution)
}

func (rr *RecurrenceRule) IsValid() bool {
    if rr.Interval <= 0 {
        return false
    }

    if rr.MaxOccurrences < 0 {
        return false
    }

    if rr.EndDate != nil && rr.EndDate.Before(time.Now()) {
        return false
    }

    return true
}
