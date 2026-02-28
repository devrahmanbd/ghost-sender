package deliverability

import (
	"context"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

type ReputationManager struct {
	scores    map[string]*ReputationScore
	scoresMu  sync.RWMutex
	config    *ReputationConfig
	logger    logger.Logger
	observers []ReputationObserver
}

type ReputationConfig struct {
	EnableTracking           bool
	ScoreDecayInterval       time.Duration
	ScoreDecayRate           float64
	BounceWeight             float64
	ComplaintWeight          float64
	SuccessWeight            float64
	SuspensionThreshold      float64
	WarningThreshold         float64
	RecoveryThreshold        float64
	MinimumSampleSize        int
	SlidingWindowDuration    time.Duration
	AutoRotateOnLowScore     bool
	AutoSuspendOnCritical    bool
}

type ReputationScore struct {
	AccountID         string
	Email             string
	Score             float64
	TotalSent         int64
	TotalSuccess      int64
	TotalFailed       int64
	TotalBounces      int64
	TotalComplaints   int64
	SuccessRate       float64
	BounceRate        float64
	ComplaintRate     float64
	HealthStatus      HealthStatus
	LastCalculated    time.Time
	LastActivity      time.Time
	History           []*ScoreSnapshot
	Flags             []string
}

type ScoreSnapshot struct {
	Score       float64
	Timestamp   time.Time
	Sent        int64
	Success     int64
	Failed      int64
	Bounces     int64
	Complaints  int64
}

type HealthStatus string

const (
	HealthExcellent HealthStatus = "excellent"
	HealthGood      HealthStatus = "good"
	HealthFair      HealthStatus = "fair"
	HealthPoor      HealthStatus = "poor"
	HealthCritical  HealthStatus = "critical"
)

type ReputationObserver interface {
	OnScoreChanged(accountID string, oldScore, newScore float64)
	OnHealthChanged(accountID string, oldHealth, newHealth HealthStatus)
	OnThresholdReached(accountID string, threshold string, score float64)
}

type ReputationEvent struct {
	AccountID   string
	EventType   ReputationEventType
	Timestamp   time.Time
	Impact      float64
	Details     map[string]interface{}
}

type ReputationEventType string

const (
	EventEmailSent      ReputationEventType = "email_sent"
	EventEmailSuccess   ReputationEventType = "email_success"
	EventEmailFailed    ReputationEventType = "email_failed"
	EventEmailBounced   ReputationEventType = "email_bounced"
	EventComplaint      ReputationEventType = "complaint"
	EventScoreDecay     ReputationEventType = "score_decay"
	EventScoreRecovery  ReputationEventType = "score_recovery"
)

func NewReputationManager(logger logger.Logger) *ReputationManager {
	return &ReputationManager{
		scores:    make(map[string]*ReputationScore),
		config:    DefaultReputationConfig(),
		logger:    logger,
		observers: make([]ReputationObserver, 0),
	}
}

func DefaultReputationConfig() *ReputationConfig {
	return &ReputationConfig{
		EnableTracking:           true,
		ScoreDecayInterval:       24 * time.Hour,
		ScoreDecayRate:           0.05,
		BounceWeight:             -10.0,
		ComplaintWeight:          -15.0,
		SuccessWeight:            1.0,
		SuspensionThreshold:      30.0,
		WarningThreshold:         50.0,
		RecoveryThreshold:        70.0,
		MinimumSampleSize:        10,
		SlidingWindowDuration:    7 * 24 * time.Hour,
		AutoRotateOnLowScore:     true,
		AutoSuspendOnCritical:    true,
	}
}

func (rm *ReputationManager) GetScore(accountID string) *ReputationScore {
	rm.scoresMu.RLock()
	defer rm.scoresMu.RUnlock()

	if score, exists := rm.scores[accountID]; exists {
		return score
	}
	return nil
}

func (rm *ReputationManager) InitializeAccount(account *models.Account) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	if _, exists := rm.scores[account.ID]; !exists {
		rm.scores[account.ID] = &ReputationScore{
			AccountID:      account.ID,
			Email:          account.Email,
			Score:          100.0,
			HealthStatus:   HealthExcellent,
			LastCalculated: time.Now(),
			LastActivity:   time.Now(),
			History:        make([]*ScoreSnapshot, 0),
			Flags:          make([]string, 0),
		}
	}
}

func (rm *ReputationManager) RecordEmailSent(accountID string) error {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	score.TotalSent++
	score.LastActivity = time.Now()

	return nil
}

func (rm *ReputationManager) RecordSuccess(accountID string) error {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	score.TotalSuccess++
	score.LastActivity = time.Now()

	oldScore := score.Score
	score.Score += rm.config.SuccessWeight
	if score.Score > 100.0 {
		score.Score = 100.0
	}

	rm.recalculateRates(score)
	rm.updateHealthStatus(score)

	if oldScore != score.Score {
		rm.notifyScoreChange(accountID, oldScore, score.Score)
	}

	return nil
}

func (rm *ReputationManager) RecordFailure(accountID string, reason string) error {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	score.TotalFailed++
	score.LastActivity = time.Now()

	oldScore := score.Score
	score.Score -= 2.0
	if score.Score < 0 {
		score.Score = 0
	}

	rm.recalculateRates(score)
	rm.updateHealthStatus(score)

	if oldScore != score.Score {
		rm.notifyScoreChange(accountID, oldScore, score.Score)
	}

	return nil
}

func (rm *ReputationManager) RecordBounce(accountID string, bounceType string) error {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	score.TotalBounces++
	score.LastActivity = time.Now()

	oldScore := score.Score
	oldHealth := score.HealthStatus

	weight := rm.config.BounceWeight
	if bounceType == "hard" {
		weight *= 1.5
	}

	score.Score += weight
	if score.Score < 0 {
		score.Score = 0
	}

	rm.recalculateRates(score)
	rm.updateHealthStatus(score)

	if oldScore != score.Score {
		rm.notifyScoreChange(accountID, oldScore, score.Score)
	}

	if oldHealth != score.HealthStatus {
		rm.notifyHealthChange(accountID, oldHealth, score.HealthStatus)
	}

	if score.Score <= rm.config.SuspensionThreshold {
		rm.notifyThresholdReached(accountID, "suspension", score.Score)
	}

	return nil
}

func (rm *ReputationManager) RecordComplaint(accountID string) error {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	score.TotalComplaints++
	score.LastActivity = time.Now()

	oldScore := score.Score
	oldHealth := score.HealthStatus

	score.Score += rm.config.ComplaintWeight
	if score.Score < 0 {
		score.Score = 0
	}

	rm.recalculateRates(score)
	rm.updateHealthStatus(score)

	if oldScore != score.Score {
		rm.notifyScoreChange(accountID, oldScore, score.Score)
	}

	if oldHealth != score.HealthStatus {
		rm.notifyHealthChange(accountID, oldHealth, score.HealthStatus)
	}

	if score.Score <= rm.config.SuspensionThreshold {
		rm.notifyThresholdReached(accountID, "suspension", score.Score)
	}

	return nil
}

func (rm *ReputationManager) getOrCreateScore(accountID string) *ReputationScore {
	if score, exists := rm.scores[accountID]; exists {
		return score
	}

	score := &ReputationScore{
		AccountID:      accountID,
		Score:          100.0,
		HealthStatus:   HealthExcellent,
		LastCalculated: time.Now(),
		LastActivity:   time.Now(),
		History:        make([]*ScoreSnapshot, 0),
		Flags:          make([]string, 0),
	}
	rm.scores[accountID] = score
	return score
}

func (rm *ReputationManager) recalculateRates(score *ReputationScore) {
	if score.TotalSent == 0 {
		score.SuccessRate = 0
		score.BounceRate = 0
		score.ComplaintRate = 0
		return
	}

	score.SuccessRate = float64(score.TotalSuccess) / float64(score.TotalSent) * 100
	score.BounceRate = float64(score.TotalBounces) / float64(score.TotalSent) * 100
	score.ComplaintRate = float64(score.TotalComplaints) / float64(score.TotalSent) * 100
}

func (rm *ReputationManager) updateHealthStatus(score *ReputationScore) {
	oldHealth := score.HealthStatus

	if score.Score >= 85 {
		score.HealthStatus = HealthExcellent
	} else if score.Score >= 70 {
		score.HealthStatus = HealthGood
	} else if score.Score >= 50 {
		score.HealthStatus = HealthFair
	} else if score.Score >= 30 {
		score.HealthStatus = HealthPoor
	} else {
		score.HealthStatus = HealthCritical
	}

	if oldHealth != score.HealthStatus {
		rm.notifyHealthChange(score.AccountID, oldHealth, score.HealthStatus)
	}
}

func (rm *ReputationManager) TakeSnapshot(accountID string) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score, exists := rm.scores[accountID]
	if !exists {
		return
	}

	snapshot := &ScoreSnapshot{
		Score:      score.Score,
		Timestamp:  time.Now(),
		Sent:       score.TotalSent,
		Success:    score.TotalSuccess,
		Failed:     score.TotalFailed,
		Bounces:    score.TotalBounces,
		Complaints: score.TotalComplaints,
	}

	score.History = append(score.History, snapshot)

	maxHistory := 100
	if len(score.History) > maxHistory {
		score.History = score.History[len(score.History)-maxHistory:]
	}
}

func (rm *ReputationManager) ApplyDecay(ctx context.Context) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	for accountID, score := range rm.scores {
		if time.Since(score.LastActivity) < rm.config.ScoreDecayInterval {
			continue
		}

		oldScore := score.Score

		if score.Score < 100 {
			recovery := rm.config.ScoreDecayRate * 10
			score.Score += recovery
			if score.Score > 100 {
				score.Score = 100
			}
		}

		rm.updateHealthStatus(score)

		if oldScore != score.Score {
			rm.notifyScoreChange(accountID, oldScore, score.Score)
		}
	}
}

func (rm *ReputationManager) ShouldRotate(accountID string) bool {
	if !rm.config.AutoRotateOnLowScore {
		return false
	}

	score := rm.GetScore(accountID)
	if score == nil {
		return false
	}

	return score.Score < rm.config.WarningThreshold
}

func (rm *ReputationManager) ShouldSuspend(accountID string) bool {
	if !rm.config.AutoSuspendOnCritical {
		return false
	}

	score := rm.GetScore(accountID)
	if score == nil {
		return false
	}

	return score.Score <= rm.config.SuspensionThreshold
}

func (rm *ReputationManager) GetAllScores() map[string]*ReputationScore {
	rm.scoresMu.RLock()
	defer rm.scoresMu.RUnlock()

	result := make(map[string]*ReputationScore)
	for id, score := range rm.scores {
		result[id] = score
	}
	return result
}

func (rm *ReputationManager) GetScoresByHealth(health HealthStatus) []*ReputationScore {
	rm.scoresMu.RLock()
	defer rm.scoresMu.RUnlock()

	result := make([]*ReputationScore, 0)
	for _, score := range rm.scores {
		if score.HealthStatus == health {
			result = append(result, score)
		}
	}
	return result
}

func (rm *ReputationManager) ResetScore(accountID string) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	if score, exists := rm.scores[accountID]; exists {
		oldScore := score.Score
		score.Score = 100.0
		score.TotalSent = 0
		score.TotalSuccess = 0
		score.TotalFailed = 0
		score.TotalBounces = 0
		score.TotalComplaints = 0
		score.SuccessRate = 0
		score.BounceRate = 0
		score.ComplaintRate = 0
		score.HealthStatus = HealthExcellent
		score.LastCalculated = time.Now()
		score.Flags = make([]string, 0)

		rm.notifyScoreChange(accountID, oldScore, score.Score)
	}
}

func (rm *ReputationManager) AddFlag(accountID string, flag string) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score := rm.getOrCreateScore(accountID)
	for _, f := range score.Flags {
		if f == flag {
			return
		}
	}
	score.Flags = append(score.Flags, flag)
}

func (rm *ReputationManager) RemoveFlag(accountID string, flag string) {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	score, exists := rm.scores[accountID]
	if !exists {
		return
	}

	newFlags := make([]string, 0)
	for _, f := range score.Flags {
		if f != flag {
			newFlags = append(newFlags, f)
		}
	}
	score.Flags = newFlags
}

func (rm *ReputationManager) GetHistory(accountID string, limit int) []*ScoreSnapshot {
	rm.scoresMu.RLock()
	defer rm.scoresMu.RUnlock()

	score, exists := rm.scores[accountID]
	if !exists {
		return []*ScoreSnapshot{}
	}

	history := score.History
	if limit > 0 && len(history) > limit {
		history = history[len(history)-limit:]
	}

	return history
}

func (rm *ReputationManager) RegisterObserver(observer ReputationObserver) {
	rm.observers = append(rm.observers, observer)
}

func (rm *ReputationManager) notifyScoreChange(accountID string, oldScore, newScore float64) {
	for _, observer := range rm.observers {
		observer.OnScoreChanged(accountID, oldScore, newScore)
	}
}

func (rm *ReputationManager) notifyHealthChange(accountID string, oldHealth, newHealth HealthStatus) {
	for _, observer := range rm.observers {
		observer.OnHealthChanged(accountID, oldHealth, newHealth)
	}
}

func (rm *ReputationManager) notifyThresholdReached(accountID string, threshold string, score float64) {
	for _, observer := range rm.observers {
		observer.OnThresholdReached(accountID, threshold, score)
	}
}

func (rm *ReputationManager) SetConfig(config *ReputationConfig) {
	rm.config = config
}

func (rm *ReputationManager) GetConfig() *ReputationConfig {
	return rm.config
}

func (rm *ReputationManager) ExportStats() map[string]interface{} {
	rm.scoresMu.RLock()
	defer rm.scoresMu.RUnlock()

	stats := map[string]interface{}{
		"total_accounts": len(rm.scores),
		"by_health":      make(map[string]int),
		"average_score":  0.0,
	}

	healthCounts := make(map[HealthStatus]int)
	totalScore := 0.0

	for _, score := range rm.scores {
		healthCounts[score.HealthStatus]++
		totalScore += score.Score
	}

	if len(rm.scores) > 0 {
		stats["average_score"] = totalScore / float64(len(rm.scores))
	}

	stats["by_health"] = map[string]int{
		string(HealthExcellent): healthCounts[HealthExcellent],
		string(HealthGood):      healthCounts[HealthGood],
		string(HealthFair):      healthCounts[HealthFair],
		string(HealthPoor):      healthCounts[HealthPoor],
		string(HealthCritical):  healthCounts[HealthCritical],
	}

	return stats
}

func (rm *ReputationManager) Clear() {
	rm.scoresMu.Lock()
	defer rm.scoresMu.Unlock()

	rm.scores = make(map[string]*ReputationScore)
}

