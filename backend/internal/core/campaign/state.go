package campaign

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"email-campaign-system/internal/models"
)

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrStateNotFound     = errors.New("state not found")
)

type CampaignState struct {
	CampaignID       string
	Status           models.CampaignStatus
	Progress         *Progress
	CurrentRecipient int64
	TotalRecipients  int64
	History          []*StateTransition
	Metadata         map[string]interface{}
	CreatedAt        time.Time
	UpdatedAt        time.Time
	mu               sync.RWMutex
}

type Progress struct {
	Percentage        float64
	ProcessedCount    int64
	SuccessCount      int64
	FailedCount       int64
	SkippedCount      int64
	RemainingCount    int64
	EstimatedComplete time.Time
	CurrentRate       float64
	mu                sync.RWMutex
}

type StateTransition struct {
	From      models.CampaignStatus
	To        models.CampaignStatus
	Timestamp time.Time
	Reason    string
	Metadata  map[string]interface{}
}

type StateMachine struct {
	transitions map[models.CampaignStatus][]models.CampaignStatus
	mu          sync.RWMutex
}

func NewStateMachine() *StateMachine {
    sm := &StateMachine{
        transitions: make(map[models.CampaignStatus][]models.CampaignStatus),
    }

    // created → can start or be scheduled
    sm.transitions[models.CampaignStatusCreated] = []models.CampaignStatus{
        models.CampaignStatusRunning,
        models.CampaignStatusScheduled, // ✅ added
        models.CampaignStatusFailed,
    }

    // scheduled → can be started or cancelled
    sm.transitions[models.CampaignStatusScheduled] = []models.CampaignStatus{
        models.CampaignStatusRunning,   // ✅ added
        models.CampaignStatusStopped,   // ✅ added
        models.CampaignStatusFailed,    // ✅ added
    }

    // running → can pause, stop, complete, or fail
    sm.transitions[models.CampaignStatusRunning] = []models.CampaignStatus{
        models.CampaignStatusPaused,
        models.CampaignStatusStopped,
        models.CampaignStatusCompleted,
        models.CampaignStatusFailed,
    }

    // paused → can resume, stop, or fail
    sm.transitions[models.CampaignStatusPaused] = []models.CampaignStatus{
        models.CampaignStatusRunning,
        models.CampaignStatusStopped,
        models.CampaignStatusFailed,
    }

    // stopped → can be restarted ✅ added
    sm.transitions[models.CampaignStatusStopped] = []models.CampaignStatus{
        models.CampaignStatusRunning,
    }

    // completed → terminal, no transitions
    sm.transitions[models.CampaignStatusCompleted] = []models.CampaignStatus{}

    // failed → can be retried
    sm.transitions[models.CampaignStatusFailed] = []models.CampaignStatus{
        models.CampaignStatusRunning,
    }

    return sm
}


func (sm *StateMachine) CanTransition(from, to models.CampaignStatus) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowedStates, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, state := range allowedStates {
		if state == to {
			return true
		}
	}

	return false
}

func (sm *StateMachine) GetAllowedTransitions(from models.CampaignStatus) []models.CampaignStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowedStates, exists := sm.transitions[from]
	if !exists {
		return []models.CampaignStatus{}
	}

	result := make([]models.CampaignStatus, len(allowedStates))
	copy(result, allowedStates)
	return result
}

func NewCampaignState(campaignID string) *CampaignState {
	now := time.Now()
	return &CampaignState{
		CampaignID:       campaignID,
		Status:           models.CampaignStatusCreated,
		Progress:         NewProgress(),
		CurrentRecipient: 0,
		TotalRecipients:  0,
		History:          make([]*StateTransition, 0),
		Metadata:         make(map[string]interface{}),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func NewProgress() *Progress {
	return &Progress{
		Percentage:        0,
		ProcessedCount:    0,
		SuccessCount:      0,
		FailedCount:       0,
		SkippedCount:      0,
		RemainingCount:    0,
		EstimatedComplete: time.Time{},
		CurrentRate:       0,
	}
}

func (cs *CampaignState) Transition(to models.CampaignStatus, reason string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	from := cs.Status

	transition := &StateTransition{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Reason:    reason,
		Metadata:  make(map[string]interface{}),
	}

	cs.Status = to
	cs.UpdatedAt = time.Now()
	cs.History = append(cs.History, transition)

	return nil
}

func (cs *CampaignState) GetStatus() models.CampaignStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Status
}

func (cs *CampaignState) UpdateProgress(processed, success, failed, skipped, total int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.Progress.mu.Lock()
	defer cs.Progress.mu.Unlock()

	cs.Progress.ProcessedCount = processed
	cs.Progress.SuccessCount = success
	cs.Progress.FailedCount = failed
	cs.Progress.SkippedCount = skipped
	cs.Progress.RemainingCount = total - processed

	if total > 0 {
		cs.Progress.Percentage = float64(processed) / float64(total) * 100
	}

	cs.TotalRecipients = total
	cs.CurrentRecipient = processed
	cs.UpdatedAt = time.Now()
}

func (cs *CampaignState) UpdateRate(rate float64) {
	cs.Progress.mu.Lock()
	defer cs.Progress.mu.Unlock()

	cs.Progress.CurrentRate = rate

	if rate > 0 && cs.Progress.RemainingCount > 0 {
		secondsRemaining := float64(cs.Progress.RemainingCount) / rate
		cs.Progress.EstimatedComplete = time.Now().Add(time.Duration(secondsRemaining) * time.Second)
	}
}

func (cs *CampaignState) GetProgress() *Progress {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cs.Progress.mu.RLock()
	defer cs.Progress.mu.RUnlock()

	return &Progress{
		Percentage:        cs.Progress.Percentage,
		ProcessedCount:    cs.Progress.ProcessedCount,
		SuccessCount:      cs.Progress.SuccessCount,
		FailedCount:       cs.Progress.FailedCount,
		SkippedCount:      cs.Progress.SkippedCount,
		RemainingCount:    cs.Progress.RemainingCount,
		EstimatedComplete: cs.Progress.EstimatedComplete,
		CurrentRate:       cs.Progress.CurrentRate,
	}
}

func (cs *CampaignState) SetMetadata(key string, value interface{}) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Metadata[key] = value
	cs.UpdatedAt = time.Now()
}

func (cs *CampaignState) GetMetadata(key string) (interface{}, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	val, exists := cs.Metadata[key]
	return val, exists
}

func (cs *CampaignState) GetAllMetadata() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make(map[string]interface{}, len(cs.Metadata))
	for k, v := range cs.Metadata {
		result[k] = v
	}
	return result
}

func (cs *CampaignState) GetHistory() []*StateTransition {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	history := make([]*StateTransition, len(cs.History))
	copy(history, cs.History)
	return history
}

func (cs *CampaignState) GetLastTransition() *StateTransition {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.History) == 0 {
		return nil
	}

	return cs.History[len(cs.History)-1]
}

func (cs *CampaignState) IsTerminal() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	return cs.Status == models.CampaignStatusCompleted ||
		cs.Status == models.CampaignStatusStopped ||
		cs.Status == models.CampaignStatusFailed
}

func (cs *CampaignState) IsRunning() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Status == models.CampaignStatusRunning
}

func (cs *CampaignState) IsPaused() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Status == models.CampaignStatusPaused
}

func (cs *CampaignState) Clone() *CampaignState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cs.Progress.mu.RLock()
	defer cs.Progress.mu.RUnlock()

	progress := &Progress{
		Percentage:        cs.Progress.Percentage,
		ProcessedCount:    cs.Progress.ProcessedCount,
		SuccessCount:      cs.Progress.SuccessCount,
		FailedCount:       cs.Progress.FailedCount,
		SkippedCount:      cs.Progress.SkippedCount,
		RemainingCount:    cs.Progress.RemainingCount,
		EstimatedComplete: cs.Progress.EstimatedComplete,
		CurrentRate:       cs.Progress.CurrentRate,
	}

	history := make([]*StateTransition, len(cs.History))
	for i, t := range cs.History {
		metadata := make(map[string]interface{}, len(t.Metadata))
		for k, v := range t.Metadata {
			metadata[k] = v
		}

		history[i] = &StateTransition{
			From:      t.From,
			To:        t.To,
			Timestamp: t.Timestamp,
			Reason:    t.Reason,
			Metadata:  metadata,
		}
	}

	metadata := make(map[string]interface{}, len(cs.Metadata))
	for k, v := range cs.Metadata {
		metadata[k] = v
	}

	return &CampaignState{
		CampaignID:       cs.CampaignID,
		Status:           cs.Status,
		Progress:         progress,
		CurrentRecipient: cs.CurrentRecipient,
		TotalRecipients:  cs.TotalRecipients,
		History:          history,
		Metadata:         metadata,
		CreatedAt:        cs.CreatedAt,
		UpdatedAt:        cs.UpdatedAt,
	}
}

func (cs *CampaignState) MarshalJSON() ([]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cs.Progress.mu.RLock()
	defer cs.Progress.mu.RUnlock()

	type Alias CampaignState
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(cs),
	})
}

func (cs *CampaignState) UnmarshalJSON(data []byte) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	type Alias CampaignState
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(cs),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if cs.Progress == nil {
		cs.Progress = NewProgress()
	}

	if cs.History == nil {
		cs.History = make([]*StateTransition, 0)
	}

	if cs.Metadata == nil {
		cs.Metadata = make(map[string]interface{})
	}

	return nil
}

func (p *Progress) GetSuccessRate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.ProcessedCount == 0 {
		return 0
	}

	return float64(p.SuccessCount) / float64(p.ProcessedCount) * 100
}

func (p *Progress) GetFailureRate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.ProcessedCount == 0 {
		return 0
	}

	return float64(p.FailedCount) / float64(p.ProcessedCount) * 100
}

func (p *Progress) IsComplete() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.RemainingCount == 0 && p.ProcessedCount > 0
}

func (st *StateTransition) SetMetadata(key string, value interface{}) {
	if st.Metadata == nil {
		st.Metadata = make(map[string]interface{})
	}
	st.Metadata[key] = value
}

func (st *StateTransition) GetMetadata(key string) (interface{}, bool) {
	if st.Metadata == nil {
		return nil, false
	}
	val, exists := st.Metadata[key]
	return val, exists
}
