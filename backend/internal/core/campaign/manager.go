package campaign

import (
	"context"
	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/models"
	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/logger"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCampaignNotFound       = errors.New("campaign not found")
	ErrCampaignAlreadyExists  = errors.New("campaign already exists")
	ErrCampaignNotRunning     = errors.New("campaign not running")
	ErrCampaignAlreadyRunning = errors.New("campaign already running")
	ErrInvalidCampaignState   = errors.New("invalid campaign state")
	ErrCampaignCompleted      = errors.New("campaign already completed")
)

type Manager struct {
	campaigns      map[string]*CampaignInstance
	mu             sync.RWMutex
	repo           repository.CampaignRepository
	executor       *Executor
	persistence    *Persistence
	scheduler      *Scheduler
	cleanup        *Cleanup
	stateMachine   *StateMachine
	hub            *websocket.Hub
	log            logger.Logger // ✅ Fixed: Interface, not pointer
	maxConcurrent  int
	shutdownSignal chan struct{}
	wg             sync.WaitGroup
}

type CampaignInstance struct {
	Campaign       *models.Campaign
	State          *CampaignState
	Executor       *ExecutorInstance
	Stats          *CampaignStats
	StartTime      time.Time
	EndTime        time.Time
	LastCheckpoint time.Time
	mu             sync.RWMutex
}

type CampaignStats struct {
	TotalRecipients   int64
	Sent              int64
	Failed            int64
	Pending           int64
	Skipped           int64
	SuccessRate       float64
	Throughput        float64
	AverageSendTime   time.Duration
	EstimatedComplete time.Time
	mu                sync.RWMutex
}

type ManagerConfig struct {
	MaxConcurrentCampaigns int
	CheckpointInterval     time.Duration
	CleanupInterval        time.Duration
	StatsUpdateInterval    time.Duration
	MaxRetries             int
	RetryDelay             time.Duration
}

func NewManager(
    repo repository.CampaignRepository,
    executor *Executor,
    persistence *Persistence,
    scheduler *Scheduler,
    cleanup *Cleanup,
    hub *websocket.Hub,
    log logger.Logger,
    config ManagerConfig,
) *Manager {
    // ✅ Normalize BEFORE constructing m, so m.maxConcurrent gets the right value
    if config.MaxConcurrentCampaigns <= 0 {
        config.MaxConcurrentCampaigns = 5
    }
    if config.CheckpointInterval <= 0 {
        config.CheckpointInterval = 30 * time.Second
    }
    if config.CleanupInterval <= 0 {
        config.CleanupInterval = 1 * time.Minute
    }
    if config.StatsUpdateInterval <= 0 {
        config.StatsUpdateInterval = 5 * time.Second
    }

    m := &Manager{
        campaigns:      make(map[string]*CampaignInstance),
        repo:           repo,
        executor:       executor,
        persistence:    persistence,
        scheduler:      scheduler,
        cleanup:        cleanup,
        stateMachine:   NewStateMachine(),
        hub:            hub,
        log:            log,
        maxConcurrent:  config.MaxConcurrentCampaigns, // ✅ now gets 5
        shutdownSignal: make(chan struct{}),
    }

    if err := m.loadFromDB(context.Background()); err != nil {
        log.Error("failed to load campaigns from DB on startup",
            logger.String("error", err.Error()))
    }

    m.wg.Add(3)
    go m.checkpointLoop(config.CheckpointInterval)
    go m.cleanupLoop(config.CleanupInterval)
    go m.statsUpdateLoop(config.StatsUpdateInterval)

    return m
}

func (m *Manager) UpdateCampaign(ctx context.Context, campaign *models.Campaign) error {
    m.mu.Lock()
    instance, exists := m.campaigns[campaign.ID]
    if !exists {
        m.mu.Unlock()
        return ErrCampaignNotFound
    }
    instance.Campaign = campaign
    m.mu.Unlock()

    if err := m.repo.Update(ctx, m.toRepositoryCampaign(campaign)); err != nil {
        return fmt.Errorf("failed to update campaign: %w", err)
    }

    m.broadcastCampaignEvent(campaign.ID, "updated", nil)
    return nil
}

func (m *Manager) CreateCampaign(ctx context.Context, campaign *models.Campaign) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Printf("DEBUG CreateCampaign START: name=%s\n", campaign.Name)

	if campaign.ID == "" {
		campaign.ID = uuid.New().String()
	}
	fmt.Printf("DEBUG CreateCampaign: id=%s\n", campaign.ID)

	if _, exists := m.campaigns[campaign.ID]; exists {
		fmt.Printf("DEBUG CreateCampaign: already exists\n")
		return ErrCampaignAlreadyExists
	}

	if err := m.validateCampaign(campaign); err != nil {
		fmt.Printf("DEBUG CreateCampaign: validation FAILED: %v\n", err)
		return fmt.Errorf("campaign validation failed: %w", err)
	}
	fmt.Printf("DEBUG CreateCampaign: validation passed\n")

	campaign.Status = models.CampaignStatusCreated
	campaign.CreatedAt = time.Now()
	campaign.UpdatedAt = time.Now()

	repoCampaign := m.toRepositoryCampaign(campaign)
	fmt.Printf("DEBUG CreateCampaign: repoCampaign built, calling repo.Create\n")

	if err := m.repo.Create(ctx, repoCampaign); err != nil {
		fmt.Printf("DEBUG CreateCampaign: repo.Create FAILED: %v\n", err)
		return fmt.Errorf("failed to create campaign: %w", err)
	}
	fmt.Printf("DEBUG CreateCampaign: repo.Create SUCCESS\n")

	instance := &CampaignInstance{
		Campaign: campaign,
		State:    NewCampaignState(campaign.ID),
		Stats:    &CampaignStats{},
	}
	m.campaigns[campaign.ID] = instance

	m.log.Info("campaign created",
		logger.String("campaign_id", campaign.ID),
		logger.String("name", campaign.Name))

	m.broadcastCampaignEvent(campaign.ID, "created", nil)
	return nil
}

func (m *Manager) StartCampaign(ctx context.Context, campaignID string) error {
    m.mu.Lock()
    instance, exists := m.campaigns[campaignID]
    if !exists {
        m.mu.Unlock()
        return ErrCampaignNotFound
    }
    m.mu.Unlock()

    // ✅ Check BEFORE acquiring instance lock — avoids reentrant deadlock
    if m.getRunningCount() >= m.maxConcurrent {
        return fmt.Errorf("maximum concurrent campaigns reached: %d", m.maxConcurrent)
    }

    instance.mu.Lock()
    defer instance.mu.Unlock()

    if !m.stateMachine.CanTransition(instance.State.Status, models.CampaignStatusRunning) {
        return fmt.Errorf("%w: cannot transition from %s to running", ErrInvalidCampaignState, instance.State.Status)
    }

    instance.State.Status = models.CampaignStatusRunning
    instance.Campaign.Status = models.CampaignStatusRunning
    instance.StartTime = time.Now()
    instance.Campaign.StartedAt = &instance.StartTime

    if err := m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign)); err != nil {
        return fmt.Errorf("failed to update campaign: %w", err)
    }

    executorInstance, err := m.executor.Start(ctx, instance.Campaign)
    if err != nil {
        instance.State.Status = models.CampaignStatusFailed
        instance.Campaign.Status = models.CampaignStatusFailed
        m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign))
        return fmt.Errorf("failed to start executor: %w", err)
    }

    instance.Executor = executorInstance

    m.log.Info("campaign started", logger.String("campaign_id", campaignID))

    m.broadcastCampaignEvent(campaignID, "started", map[string]interface{}{
        "start_time": instance.StartTime,
    })

    go m.monitorCampaign(ctx, instance)

    return nil
}

func (m *Manager) PauseCampaign(ctx context.Context, campaignID string) error {
	m.mu.RLock()
	instance, exists := m.campaigns[campaignID]
	m.mu.RUnlock()

	if !exists {
		return ErrCampaignNotFound
	}

	instance.mu.Lock()
	defer instance.mu.Unlock()

	if instance.State.Status != models.CampaignStatusRunning {
		return ErrCampaignNotRunning
	}

	if instance.Executor != nil {
		if err := instance.Executor.Pause(ctx); err != nil {
			return fmt.Errorf("failed to pause executor: %w", err)
		}
	}

	instance.State.Status = models.CampaignStatusPaused
	instance.Campaign.Status = models.CampaignStatusPaused
	instance.Campaign.UpdatedAt = time.Now()

	if err := m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign)); err != nil {
		return fmt.Errorf("failed to update campaign: %w", err)
	}

	if m.persistence != nil {
		if err := m.persistence.SaveState(ctx, campaignID, instance.State); err != nil {
			m.log.Error("failed to save campaign state",
				logger.String("campaign_id", campaignID),
				logger.String("error", err.Error()))
		}
	}
	m.log.Info("campaign paused", logger.String("campaign_id", campaignID))

	m.broadcastCampaignEvent(campaignID, "paused", nil)

	return nil
}

func (m *Manager) ResumeCampaign(ctx context.Context, campaignID string) error {
	m.mu.RLock()
	instance, exists := m.campaigns[campaignID]
	m.mu.RUnlock()

	if !exists {
		return ErrCampaignNotFound
	}

	instance.mu.Lock()
	defer instance.mu.Unlock()

	if instance.State.Status != models.CampaignStatusPaused {
		return fmt.Errorf("%w: campaign is not paused", ErrInvalidCampaignState)
	}

	if instance.Executor != nil {
		if err := instance.Executor.Resume(ctx); err != nil {
			return fmt.Errorf("failed to resume executor: %w", err)
		}
	}

	instance.State.Status = models.CampaignStatusRunning
	instance.Campaign.Status = models.CampaignStatusRunning
	instance.Campaign.UpdatedAt = time.Now()

	if err := m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign)); err != nil {
		return fmt.Errorf("failed to update campaign: %w", err)
	}

	m.log.Info("campaign resumed", logger.String("campaign_id", campaignID))

	m.broadcastCampaignEvent(campaignID, "resumed", nil)

	return nil
}
func (m *Manager) StopCampaign(ctx context.Context, campaignID string) error {
	m.mu.RLock()
	instance, exists := m.campaigns[campaignID]
	m.mu.RUnlock()

	if !exists {
		return ErrCampaignNotFound
	}

	instance.mu.Lock()
	defer instance.mu.Unlock()

	if instance.State.Status != models.CampaignStatusRunning && instance.State.Status != models.CampaignStatusPaused {
		return fmt.Errorf("%w: campaign is not running or paused", ErrInvalidCampaignState)
	}

	if instance.Executor != nil {
		if err := instance.Executor.Stop(ctx); err != nil {
			m.log.Error("error stopping executor",
				logger.String("campaign_id", campaignID),
				logger.String("error", err.Error()))
		}
	}

	instance.State.Status = models.CampaignStatusStopped
	instance.Campaign.Status = models.CampaignStatusStopped
	instance.EndTime = time.Now()
	instance.Campaign.CompletedAt = &instance.EndTime
	instance.Campaign.UpdatedAt = time.Now()

	if err := m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign)); err != nil {
		return fmt.Errorf("failed to update campaign: %w", err)
	}

	if err := m.repo.UpdateProgress(ctx, campaignID,
		int(instance.Stats.Sent),
		int(instance.Stats.Failed),
		int(instance.Stats.Pending),
	); err != nil {
		m.log.Error("failed to persist final stats on stop",
			logger.String("campaign_id", campaignID),
			logger.String("error", err.Error()))
	}

	if m.persistence != nil {
		if err := m.persistence.SaveState(ctx, campaignID, instance.State); err != nil {
			m.log.Error("failed to save campaign state",
				logger.String("campaign_id", campaignID),
				logger.String("error", err.Error()))
		}
	}

	m.log.Info("campaign stopped", logger.String("campaign_id", campaignID))

	m.broadcastCampaignEvent(campaignID, "stopped", map[string]interface{}{
		"end_time": instance.EndTime,
		"stats":    instance.Stats,
	})

	return nil
}


func (m *Manager) GetCampaign(ctx context.Context, campaignID string) (*models.Campaign, error) {
	m.mu.RLock()
	instance, exists := m.campaigns[campaignID]
	m.mu.RUnlock()

	if !exists {
		fmt.Printf("DEBUG GetCampaign: map miss id=%s len(campaigns)=%d — trying DB\n",
			campaignID, len(m.campaigns))
		rc, err := m.repo.GetByID(ctx, campaignID)
		if err != nil {
			return nil, ErrCampaignNotFound
		}
		return m.fromRepositoryCampaign(rc), nil
	}

	instance.mu.RLock()
	defer instance.mu.RUnlock()
	return instance.Campaign, nil
}


func (m *Manager) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error) {
	fmt.Printf("DEBUG GetCampaignStats: campaignID=%s len(m.campaigns)=%d\n",
		campaignID, len(m.campaigns))

	m.mu.RLock()
	instance, exists := m.campaigns[campaignID]
	m.mu.RUnlock()

	if !exists {
		fmt.Printf("DEBUG GetCampaignStats: map miss — trying DB\n")
		rc, err := m.repo.GetByID(ctx, campaignID)
		if err != nil {
			return nil, ErrCampaignNotFound
		}
		return &CampaignStats{
			TotalRecipients: int64(rc.TotalRecipients),
			Sent:            int64(rc.SentCount),
			Failed:          int64(rc.FailedCount),
			Pending:         int64(rc.PendingCount),
			SuccessRate:     rc.SuccessRate,
			Throughput:      rc.Throughput,
		}, nil
	}

	instance.Stats.mu.RLock()
	defer instance.Stats.mu.RUnlock()

	return &CampaignStats{
		TotalRecipients:   instance.Stats.TotalRecipients,
		Sent:              instance.Stats.Sent,
		Failed:            instance.Stats.Failed,
		Pending:           instance.Stats.Pending,
		Skipped:           instance.Stats.Skipped,
		SuccessRate:       instance.Stats.SuccessRate,
		Throughput:        instance.Stats.Throughput,
		AverageSendTime:   instance.Stats.AverageSendTime,
		EstimatedComplete: instance.Stats.EstimatedComplete,
	}, nil
}

func (m *Manager) ListCampaigns() []*models.Campaign {
	m.mu.RLock()
	defer m.mu.RUnlock()

	campaigns := make([]*models.Campaign, 0, len(m.campaigns))
	for _, instance := range m.campaigns {
		instance.mu.RLock()
		campaigns = append(campaigns, instance.Campaign)
		instance.mu.RUnlock()
	}

	return campaigns
}

func (m *Manager) DeleteCampaign(ctx context.Context, campaignID string) error {
	m.mu.Lock()
	instance, exists := m.campaigns[campaignID]
	if !exists {
		m.mu.Unlock()
		return ErrCampaignNotFound
	}
	delete(m.campaigns, campaignID)
	m.mu.Unlock()

	instance.mu.Lock()
	if instance.State.Status == models.CampaignStatusRunning {
		instance.mu.Unlock()
		return fmt.Errorf("%w: cannot delete running campaign", ErrInvalidCampaignState)
	}
	instance.mu.Unlock()

	if err := m.repo.Delete(ctx, campaignID); err != nil {
		return fmt.Errorf("failed to delete campaign: %w", err)
	}

	if m.persistence != nil {
		if err := m.persistence.DeleteState(ctx, campaignID); err != nil {
			m.log.Error("failed to delete campaign state",
				logger.String("campaign_id", campaignID),
				logger.String("error", err.Error()))
		}
	}

	m.log.Info("campaign deleted", logger.String("campaign_id", campaignID))

	m.broadcastCampaignEvent(campaignID, "deleted", nil)

	return nil
}
func (m *Manager) monitorCampaign(ctx context.Context, instance *CampaignInstance) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-m.shutdownSignal:
            return
        case <-ticker.C:
            instance.mu.RLock()
            status := instance.State.Status
            executor := instance.Executor // ✅ capture under lock
            instance.mu.RUnlock()

            if status != models.CampaignStatusRunning {
                return
            }

            // ✅ Guard before calling any method on executor
            if executor == nil {
                m.log.Warn("monitorCampaign: executor nil, stopping monitor",
                    logger.String("campaign_id", instance.Campaign.ID))
                return
            }

            m.updateCampaignStats(instance)

            if executor.IsComplete() {
                m.completeCampaign(ctx, instance)
                return
            }
        }
    }
}


func (m *Manager) completeCampaign(ctx context.Context, instance *CampaignInstance) {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	instance.State.Status = models.CampaignStatusCompleted
	instance.Campaign.Status = models.CampaignStatusCompleted
	instance.EndTime = time.Now()
	instance.Campaign.CompletedAt = &instance.EndTime
	instance.Campaign.UpdatedAt = time.Now()

	if err := m.repo.Update(ctx, m.toRepositoryCampaign(instance.Campaign)); err != nil {
		m.log.Error("failed to update completed campaign",
			logger.String("campaign_id", instance.Campaign.ID),
			logger.String("error", err.Error()))
	}

	if err := m.repo.UpdateProgress(ctx, instance.Campaign.ID,
		int(instance.Stats.Sent),
		int(instance.Stats.Failed),
		int(instance.Stats.Pending),
	); err != nil {
		m.log.Error("failed to persist final stats on complete",
			logger.String("campaign_id", instance.Campaign.ID),
			logger.String("error", err.Error()))
	}

	if m.persistence != nil {
		if err := m.persistence.SaveState(ctx, instance.Campaign.ID, instance.State); err != nil {
			m.log.Error("failed to save completed campaign state",
				logger.String("campaign_id", instance.Campaign.ID),
				logger.String("error", err.Error()))
		}
	}

	m.log.Info("campaign completed",
		logger.String("campaign_id", instance.Campaign.ID),
		logger.Int64("sent", instance.Stats.Sent),
		logger.Int64("failed", instance.Stats.Failed),
		logger.Duration("duration", instance.EndTime.Sub(instance.StartTime)),
	)

	m.broadcastCampaignEvent(instance.Campaign.ID, "completed", map[string]interface{}{
		"end_time": instance.EndTime,
		"stats":    instance.Stats,
	})
}


func (m *Manager) checkpointLoop(interval time.Duration) {
	defer m.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.shutdownSignal:
			return
		case <-ticker.C:
			m.saveCheckpoints()
		}
	}
}

func (m *Manager) saveCheckpoints() {
	m.mu.RLock()
	campaigns := make([]*CampaignInstance, 0, len(m.campaigns))
	for _, instance := range m.campaigns {
		instance.mu.RLock()
		if instance.State.Status == models.CampaignStatusRunning {
			campaigns = append(campaigns, instance)
		}
		instance.mu.RUnlock()
	}
	m.mu.RUnlock()

	for _, instance := range campaigns {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if m.persistence != nil {
			if err := m.persistence.SaveState(ctx, instance.Campaign.ID, instance.State); err != nil {
				m.log.Error("checkpoint save failed",
					logger.String("campaign_id", instance.Campaign.ID),
					logger.String("error", err.Error()))
			} else {
				instance.mu.Lock()
				instance.LastCheckpoint = time.Now()
				instance.mu.Unlock()
			}
		}
		cancel()
	}
}

func (m *Manager) cleanupLoop(interval time.Duration) {
	defer m.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.shutdownSignal:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if m.cleanup != nil {
				m.cleanup.performCleanup(ctx)
			}
			cancel()
		}
	}
}

func (m *Manager) statsUpdateLoop(interval time.Duration) {
    defer m.wg.Done()
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-m.shutdownSignal:
            return
        case <-ticker.C:
            m.mu.RLock()
            for _, instance := range m.campaigns {
                instance.mu.RLock()
                isRunning := instance.State.Status == models.CampaignStatusRunning
                hasExecutor := instance.Executor != nil // ✅ guard
                instance.mu.RUnlock()

                if isRunning && hasExecutor {
                    go m.updateCampaignStats(instance)
                }
            }
            m.mu.RUnlock()
        }
    }
}

func (m *Manager) updateCampaignStats(instance *CampaignInstance) {
    // ✅ Double-check inside the goroutine — Executor could be nil if Start failed
    instance.mu.RLock()
    executor := instance.Executor
    instance.mu.RUnlock()

    if executor == nil {
        m.log.Warn("updateCampaignStats: executor is nil, skipping",
            logger.String("campaign_id", instance.Campaign.ID))
        return
    }

    stats := executor.GetStats()

    instance.Stats.mu.Lock()
    instance.Stats.TotalRecipients = stats.Total
    instance.Stats.Sent = stats.Sent
    instance.Stats.Failed = stats.Failed
    instance.Stats.Pending = stats.Pending
    instance.Stats.Skipped = stats.Skipped

    if stats.Total > 0 {
        instance.Stats.SuccessRate = float64(stats.Sent) / float64(stats.Total) * 100
    }

    elapsed := time.Since(instance.StartTime).Seconds()
    if elapsed > 0 {
        instance.Stats.Throughput = float64(stats.Sent) / elapsed
    }

    if stats.Sent > 0 {
        instance.Stats.AverageSendTime = time.Duration(elapsed/float64(stats.Sent)) * time.Second
    }

    remaining := stats.Total - stats.Sent - stats.Failed - stats.Skipped
    if instance.Stats.Throughput > 0 && remaining > 0 {
        eta := time.Duration(float64(remaining)/instance.Stats.Throughput) * time.Second
        instance.Stats.EstimatedComplete = time.Now().Add(eta)
    }
    instance.Stats.mu.Unlock()

    m.broadcastCampaignEvent(instance.Campaign.ID, "stats_update", map[string]interface{}{
        "stats": instance.Stats,
    })
}


func (m *Manager) validateCampaign(campaign *models.Campaign) error {
	if campaign.Name == "" {
		return errors.New("campaign name is required")
	}
	if len(campaign.TemplateIDs) == 0 {
		return errors.New("at least one template is required")
	}
	if len(campaign.AccountIDs) == 0 {
		return errors.New("at least one account is required")
	}
	// REMOVED recipient validation - recipients loaded lazily from file during execution
	return nil
}
// ✅ Always hold m.mu.RLock when iterating m.campaigns
func (m *Manager) getRunningCount() int {
    m.mu.RLock()
    defer m.mu.RUnlock()

    count := 0
    for _, inst := range m.campaigns {
        inst.mu.RLock()
        if inst.State.Status == models.CampaignStatusRunning {
            count++
        }
        inst.mu.RUnlock()
    }
    return count
}


func (m *Manager) broadcastCampaignEvent(campaignID, event string, data map[string]interface{}) {
	if m.hub == nil {
		return
	}

	message := map[string]interface{}{
		"type":        "campaign_event",
		"campaign_id": campaignID,
		"event":       event,
		"timestamp":   time.Now(),
	}

	if data != nil {
		message["data"] = data
	}

	// ✅ Fixed: Convert to websocket.Message
	dataJSON, _ := json.Marshal(message)
	m.hub.Broadcast(&websocket.Message{
		Type: "campaign_event",
		Data: json.RawMessage(dataJSON),
	})
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.log.Info("shutting down campaign manager")

	close(m.shutdownSignal)

	m.mu.RLock()
	runningCampaigns := make([]*CampaignInstance, 0)
	for _, instance := range m.campaigns {
		instance.mu.RLock()
		if instance.State.Status == models.CampaignStatusRunning {
			runningCampaigns = append(runningCampaigns, instance)
		}
		instance.mu.RUnlock()
	}
	m.mu.RUnlock()

	for _, instance := range runningCampaigns {
		if err := m.PauseCampaign(ctx, instance.Campaign.ID); err != nil {
			m.log.Error("failed to pause campaign during shutdown",
				logger.String("campaign_id", instance.Campaign.ID),
				logger.String("error", err.Error()))
		}
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.log.Info("campaign manager shutdown completed")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) toRepositoryCampaign(campaign *models.Campaign) *repository.Campaign {
	fmt.Printf("🟢 DEBUG toRepositoryCampaign: START id=%s\n", campaign.ID)

	now := time.Now()
	createdAt := campaign.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := campaign.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	state := "idle"

	priorityMap := map[models.CampaignPriority]int{
		models.CampaignPriorityLow:      1,
		models.CampaignPriorityNormal:   2,
		models.CampaignPriorityHigh:     3,
		models.CampaignPriorityCritical: 4,
	}
	priority, ok := priorityMap[campaign.Priority]
	if !ok {
		priority = 2
	}

	configMap := make(map[string]interface{})
	if configBytes, err := json.Marshal(campaign.Config); err == nil {
		json.Unmarshal(configBytes, &configMap)
	}

	status := string(campaign.Status)
	if status == "" {
		status = "created"
	}

	// nil-safe Schedule access — THIS was the panic
	var scheduledAt *time.Time
	if campaign.Schedule != nil {
		scheduledAt = campaign.Schedule.ScheduledStartTime
	}

	fmt.Printf("🟢 DEBUG toRepositoryCampaign: status=%s state=%s priority=%d templateIDs=%v accountIDs=%v scheduledAt=%v\n",
		status, state, priority, campaign.TemplateIDs, campaign.AccountIDs, scheduledAt)

	repoCamp := &repository.Campaign{
		ID:              campaign.ID,
		Name:            campaign.Name,
		Description:     campaign.Description,
		Status:          status,
		State:           state,
		Priority:        priority,
		ScheduledAt:     scheduledAt, // safe now
		TotalRecipients: int(campaign.TotalRecipients),
		Config:          configMap,
		TemplateIDs:     campaign.TemplateIDs,
		AccountIDs:      campaign.AccountIDs,
		RecipientListID: nil,
		ProxyIDs:        []string{},
		Tags:            []string{},
		Metadata:        map[string]interface{}{},
		MaxRetries:      3,
		CreatedBy:       campaign.CreatedBy,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		StartedAt:       campaign.StartedAt,
		CompletedAt:     campaign.CompletedAt,
	}

	fmt.Printf("🟢 DEBUG toRepositoryCampaign: SUCCESS id=%s\n", repoCamp.ID)
	return repoCamp
}
func (m *Manager) loadFromDB(ctx context.Context) error {
    fmt.Printf("DEBUG loadFromDB: START — calling repo.List\n")
    rows, _, err := m.repo.List(ctx, &repository.CampaignFilter{IncludeArchived: false})
    if err != nil {
        fmt.Printf("DEBUG loadFromDB: repo.List FAILED: %v\n", err)
        return fmt.Errorf("loadFromDB: %w", err)
    }
    fmt.Printf("DEBUG loadFromDB: repo.List returned %d rows\n", len(rows))

    for _, rc := range rows {
        c := m.fromRepositoryCampaign(rc)
        state := NewCampaignState(c.ID)
        state.Status = c.Status

        // ✅ Campaigns marked "running" in DB means the server crashed mid-run.
        // Reset to "failed" so statsUpdateLoop never calls GetStats on a nil Executor.
        if c.Status == models.CampaignStatusRunning {
            c.Status = models.CampaignStatusFailed
            state.Status = models.CampaignStatusFailed
            fmt.Printf("DEBUG loadFromDB: reset stale running→failed for id=%s\n", c.ID)

            // Persist the reset so DB is consistent
            if err := m.repo.Update(ctx, m.toRepositoryCampaign(c)); err != nil {
                fmt.Printf("DEBUG loadFromDB: failed to persist reset for id=%s: %v\n", c.ID, err)
            }
        }

        stats := &CampaignStats{
            TotalRecipients: int64(rc.TotalRecipients),
            Sent:            int64(rc.SentCount),
            Failed:          int64(rc.FailedCount),
            Pending:         int64(rc.PendingCount),
            SuccessRate:     rc.SuccessRate,
            Throughput:      rc.Throughput,
        }

        m.campaigns[c.ID] = &CampaignInstance{
            Campaign: c,
            State:    state,
            Stats:    stats,
            // Executor intentionally nil — only set when campaign is actually started
        }
    }

    fmt.Printf("DEBUG loadFromDB: loaded %d campaigns into memory\n", len(m.campaigns))
    return nil
}


// fromRepositoryCampaign converts a flat repository.Campaign row to *models.Campaign.
func (m *Manager) fromRepositoryCampaign(rc *repository.Campaign) *models.Campaign {
	c := &models.Campaign{
		ID:              rc.ID,
		Name:            rc.Name,
		Description:     rc.Description,
		Status:          models.CampaignStatus(rc.Status),
		TotalRecipients: int64(rc.TotalRecipients),
		TemplateIDs:     rc.TemplateIDs,
		AccountIDs:      rc.AccountIDs,
		CreatedBy:       rc.CreatedBy,
		CreatedAt:       rc.CreatedAt,
		UpdatedAt:       rc.UpdatedAt,
		StartedAt:       rc.StartedAt,
		CompletedAt:     rc.CompletedAt,
	}
	if rc.ScheduledAt != nil {
		c.Schedule = &models.CampaignSchedule{
			ScheduledStartTime: rc.ScheduledAt,
		}
	}
	if len(rc.Config) > 0 {
		configBytes, _ := json.Marshal(rc.Config)
		json.Unmarshal(configBytes, &c.Config) //nolint:errcheck — best-effort restore
	}
	return c
}
