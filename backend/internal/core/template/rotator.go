package template

import (
    "crypto/rand"
    "errors"
    "fmt"
    "math/big"
    "sync"
    "time"

    "email-campaign-system/pkg/logger"
)

type RotationStrategy string

const (
    StrategySequential  RotationStrategy = "sequential"
    StrategyRandom      RotationStrategy = "random"
    StrategyWeighted    RotationStrategy = "weighted"
    StrategyTimeBased   RotationStrategy = "time_based"
    StrategyHealthBased RotationStrategy = "health_based"
    StrategyLeastUsed   RotationStrategy = "least_used"
    StrategyRoundRobin  RotationStrategy = "round_robin"
)

type Rotator struct {
    manager       *TemplateManager
    strategy      RotationStrategy
    currentIndex  int
    lastRotation  time.Time
    stats         *RotationStats
    statsMu       sync.RWMutex
    mu            sync.Mutex
    log           logger.Logger
    config        *RotatorConfig
    weights       map[string]float64
    weightsMu     sync.RWMutex
    timeSlots     map[string][]string
    lastUsedTimes map[string]time.Time
    lastUsedMu    sync.RWMutex
}

type RotatorConfig struct {
    Strategy              RotationStrategy
    EnableHealthFiltering bool
    MinHealthScore        float64
    MaxSpamScore          float64
    SkipUnhealthy         bool
    WeightByHealth        bool
    WeightByUsage         bool
    TimeSlotDuration      time.Duration
    CooldownDuration      time.Duration
}

type RotationStats struct {
    TotalRotations       int64
    SequentialCount      int64
    RandomCount          int64
    WeightedCount        int64
    TimeBasedCount       int64
    HealthBasedCount     int64
    SkippedUnhealthy     int64
    FailedRotations      int64
    AverageRotationTime  time.Duration
    LastRotation         time.Time
    TemplateUsageCount   map[string]int64
    StrategyDistribution map[RotationStrategy]int64
}

func NewRotator(manager *TemplateManager, log logger.Logger, strategy string) *Rotator {
    if strategy == "" {
        strategy = string(StrategySequential)
    }

    config := &RotatorConfig{
        Strategy:              RotationStrategy(strategy),
        EnableHealthFiltering: true,
        MinHealthScore:        50.0,
        MaxSpamScore:          7.0,
        SkipUnhealthy:         true,
        WeightByHealth:        true,
        WeightByUsage:         false,
        TimeSlotDuration:      4 * time.Hour,
        CooldownDuration:      5 * time.Minute,
    }

    rotator := &Rotator{
        manager:       manager,
        strategy:      RotationStrategy(strategy),
        currentIndex:  0,
        lastRotation:  time.Now(),
        log:           log,
        config:        config,
        weights:       make(map[string]float64),
        timeSlots:     make(map[string][]string),
        lastUsedTimes: make(map[string]time.Time),
        stats: &RotationStats{
            TemplateUsageCount:   make(map[string]int64),
            StrategyDistribution: make(map[RotationStrategy]int64),
        },
    }

    rotator.initializeWeights()
    rotator.initializeTimeSlots()

    return rotator
}

func DefaultRotatorConfig() *RotatorConfig {
    return &RotatorConfig{
        Strategy:              StrategySequential,
        EnableHealthFiltering: true,
        MinHealthScore:        50.0,
        MaxSpamScore:          7.0,
        SkipUnhealthy:         true,
        WeightByHealth:        true,
        WeightByUsage:         false,
        TimeSlotDuration:      4 * time.Hour,
        CooldownDuration:      5 * time.Minute,
    }
}

func (r *Rotator) Next() (*ManagedTemplate, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    startTime := time.Now()

    templates := r.getEligibleTemplates()
    if len(templates) == 0 {
        r.incrementFailedRotations()
        return nil, ErrNoTemplatesAvailable
    }

    var selected *ManagedTemplate
    var err error

    switch r.strategy {
    case StrategySequential:
        selected = r.nextSequential(templates)
        r.incrementStrategyCount(StrategySequential)
    case StrategyRandom:
        selected = r.nextRandom(templates)
        r.incrementStrategyCount(StrategyRandom)
    case StrategyWeighted:
        selected = r.nextWeighted(templates)
        r.incrementStrategyCount(StrategyWeighted)
    case StrategyTimeBased:
        selected = r.nextTimeBased(templates)
        r.incrementStrategyCount(StrategyTimeBased)
    case StrategyHealthBased:
        selected = r.nextHealthBased(templates)
        r.incrementStrategyCount(StrategyHealthBased)
    case StrategyLeastUsed:
        selected = r.nextLeastUsed(templates)
        r.incrementStrategyCount(StrategyLeastUsed)
    case StrategyRoundRobin:
        selected = r.nextRoundRobin(templates)
        r.incrementStrategyCount(StrategyRoundRobin)
    default:
        selected = r.nextSequential(templates)
        r.incrementStrategyCount(StrategySequential)
    }

    if selected == nil {
        r.incrementFailedRotations()
        return nil, ErrRotationFailed
    }

    duration := time.Since(startTime)
    r.lastRotation = time.Now()
    r.updateLastUsedTime(selected.Template.ID)
    r.updateRotationStats(selected, duration)

    r.log.Debug(
        "template rotated",
        logger.String("template_id", selected.Template.ID),
        logger.String("name", selected.Template.Name),
        logger.String("strategy", string(r.strategy)),
        logger.Duration("duration", duration),
    )

    return selected, err
}

func (r *Rotator) nextSequential(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    if r.currentIndex >= len(templates) {
        r.currentIndex = 0
    }

    selected := templates[r.currentIndex]
    r.currentIndex++

    return selected
}

func (r *Rotator) nextRandom(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    index, err := r.secureRandom(len(templates))
    if err != nil {
        r.log.Error(
            "failed to generate random index",
            logger.Error(err),
        )
        return r.nextSequential(templates)
    }

    return templates[index]
}

func (r *Rotator) nextWeighted(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    r.updateWeights(templates)

    totalWeight := 0.0
    for _, tmpl := range templates {
        r.weightsMu.RLock()
        weight := r.weights[tmpl.Template.ID]
        r.weightsMu.RUnlock()
        totalWeight += weight
    }

    if totalWeight == 0 {
        return r.nextRandom(templates)
    }

    randomValue := r.randomFloat64() * totalWeight
    cumulativeWeight := 0.0

    for _, tmpl := range templates {
        r.weightsMu.RLock()
        weight := r.weights[tmpl.Template.ID]
        r.weightsMu.RUnlock()

        cumulativeWeight += weight
        if randomValue <= cumulativeWeight {
            return tmpl
        }
    }

    return templates[0]
}

func (r *Rotator) nextTimeBased(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    currentSlot := r.getCurrentTimeSlot()

    eligibleIDs := r.timeSlots[currentSlot]
    if len(eligibleIDs) == 0 {
        return r.nextRandom(templates)
    }

    eligible := make([]*ManagedTemplate, 0)
    for _, tmpl := range templates {
        for _, id := range eligibleIDs {
            if tmpl.Template.ID == id {
                eligible = append(eligible, tmpl)
                break
            }
        }
    }

    if len(eligible) == 0 {
        return r.nextRandom(templates)
    }

    return r.nextRandom(eligible)
}

func (r *Rotator) nextHealthBased(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    var bestTemplate *ManagedTemplate
    bestScore := -1.0

    for _, tmpl := range templates {
        tmpl.mu.RLock()
        score := tmpl.Health.Score
        spamScore := tmpl.SpamScore
        tmpl.mu.RUnlock()

        combinedScore := score - (spamScore * 10)

        if combinedScore > bestScore {
            bestScore = combinedScore
            bestTemplate = tmpl
        }
    }

    return bestTemplate
}

func (r *Rotator) nextLeastUsed(templates []*ManagedTemplate) *ManagedTemplate {
    if len(templates) == 0 {
        return nil
    }

    var leastUsed *ManagedTemplate
    var minUsage int64 = -1

    for _, tmpl := range templates {
        r.statsMu.RLock()
        usage := r.stats.TemplateUsageCount[tmpl.Template.ID]
        r.statsMu.RUnlock()

        if minUsage == -1 || usage < minUsage {
            minUsage = usage
            leastUsed = tmpl
        }
    }

    return leastUsed
}

func (r *Rotator) nextRoundRobin(templates []*ManagedTemplate) *ManagedTemplate {
    return r.nextSequential(templates)
}

func (r *Rotator) getEligibleTemplates() []*ManagedTemplate {
    filter := &TemplateFilter{
        Status: "active",
    }

    if r.config.EnableHealthFiltering {
        filter.MinHealthScore = r.config.MinHealthScore
        filter.MaxSpamScore = r.config.MaxSpamScore
    }

    templates := r.manager.ListTemplates(filter)

    if r.config.SkipUnhealthy {
        eligible := make([]*ManagedTemplate, 0)
        for _, tmpl := range templates {
            if tmpl.IsHealthy() {
                if r.config.CooldownDuration > 0 {
                    if r.isInCooldown(tmpl.Template.ID) {
                        continue
                    }
                }
                eligible = append(eligible, tmpl)
            } else {
                r.incrementSkippedUnhealthy()
            }
        }
        return eligible
    }

    return templates
}

func (r *Rotator) isInCooldown(templateID string) bool {
    r.lastUsedMu.RLock()
    defer r.lastUsedMu.RUnlock()

    lastUsed, exists := r.lastUsedTimes[templateID]
    if !exists {
        return false
    }

    return time.Since(lastUsed) < r.config.CooldownDuration
}

func (r *Rotator) updateLastUsedTime(templateID string) {
    r.lastUsedMu.Lock()
    defer r.lastUsedMu.Unlock()

    r.lastUsedTimes[templateID] = time.Now()
}

func (r *Rotator) SetStrategy(strategy RotationStrategy) {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.strategy = strategy
    r.currentIndex = 0

    r.log.Info(
        "rotation strategy changed",
        logger.String("strategy", string(strategy)),
    )
}

func (r *Rotator) GetStrategy() RotationStrategy {
    r.mu.Lock()
    defer r.mu.Unlock()

    return r.strategy
}

func (r *Rotator) Reset() {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.currentIndex = 0
    r.lastRotation = time.Now()

    r.log.Info("rotator reset")
}

func (r *Rotator) GetStats() map[string]interface{} {
    r.statsMu.RLock()
    defer r.statsMu.RUnlock()

    usageCount := make(map[string]int64)
    for k, v := range r.stats.TemplateUsageCount {
        usageCount[k] = v
    }

    strategyDist := make(map[string]int64)
    for k, v := range r.stats.StrategyDistribution {
        strategyDist[string(k)] = v
    }

    return map[string]interface{}{
        "total_rotations":       r.stats.TotalRotations,
        "sequential_count":      r.stats.SequentialCount,
        "random_count":          r.stats.RandomCount,
        "weighted_count":        r.stats.WeightedCount,
        "time_based_count":      r.stats.TimeBasedCount,
        "health_based_count":    r.stats.HealthBasedCount,
        "skipped_unhealthy":     r.stats.SkippedUnhealthy,
        "failed_rotations":      r.stats.FailedRotations,
        "average_rotation_time": r.stats.AverageRotationTime,
        "last_rotation":         r.stats.LastRotation,
        "template_usage_count":  usageCount,
        "strategy_distribution": strategyDist,
        "current_strategy":      string(r.strategy),
        "current_index":         r.currentIndex,
    }
}

func (r *Rotator) GetCurrentIndex() int {
    r.mu.Lock()
    defer r.mu.Unlock()

    return r.currentIndex
}

func (r *Rotator) SetCurrentIndex(index int) {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.currentIndex = index
}

func (r *Rotator) initializeWeights() {
    r.weightsMu.Lock()
    defer r.weightsMu.Unlock()

    templates := r.manager.GetActiveTemplates()
    for _, tmpl := range templates {
        r.weights[tmpl.Template.ID] = 1.0
    }
}

func (r *Rotator) updateWeights(templates []*ManagedTemplate) {
    r.weightsMu.Lock()
    defer r.weightsMu.Unlock()

    for _, tmpl := range templates {
        weight := 1.0

        if r.config.WeightByHealth {
            tmpl.mu.RLock()
            healthScore := tmpl.Health.Score
            spamScore := tmpl.SpamScore
            tmpl.mu.RUnlock()

            weight = healthScore / 100.0
            weight *= (10.0 - spamScore) / 10.0
        }

        if r.config.WeightByUsage {
            r.statsMu.RLock()
            usage := r.stats.TemplateUsageCount[tmpl.Template.ID]
            totalRotations := r.stats.TotalRotations
            r.statsMu.RUnlock()

            if usage > 0 && totalRotations > 0 {
                avgUsage := float64(totalRotations) / float64(len(templates))
                if avgUsage > 0 {
                    usageRatio := float64(usage) / avgUsage
                    weight *= (2.0 - usageRatio)
                }
            }
        }

        if weight < 0.1 {
            weight = 0.1
        }

        r.weights[tmpl.Template.ID] = weight
    }
}

func (r *Rotator) initializeTimeSlots() {
    slots := []string{"morning", "afternoon", "evening", "night"}
    for _, slot := range slots {
        r.timeSlots[slot] = make([]string, 0)
    }

    templates := r.manager.GetActiveTemplates()
    for _, tmpl := range templates {
        for _, slot := range slots {
            r.timeSlots[slot] = append(r.timeSlots[slot], tmpl.Template.ID)
        }
    }
}

func (r *Rotator) getCurrentTimeSlot() string {
    hour := time.Now().Hour()

    if hour >= 6 && hour < 12 {
        return "morning"
    } else if hour >= 12 && hour < 17 {
        return "afternoon"
    } else if hour >= 17 && hour < 21 {
        return "evening"
    }
    return "night"
}

func (r *Rotator) updateRotationStats(tmpl *ManagedTemplate, duration time.Duration) {
    r.statsMu.Lock()
    defer r.statsMu.Unlock()

    r.stats.TotalRotations++
    r.stats.TemplateUsageCount[tmpl.Template.ID]++
    r.stats.LastRotation = time.Now()

    if r.stats.AverageRotationTime == 0 {
        r.stats.AverageRotationTime = duration
    } else {
        r.stats.AverageRotationTime = (r.stats.AverageRotationTime + duration) / 2
    }
}

func (r *Rotator) incrementStrategyCount(strategy RotationStrategy) {
    r.statsMu.Lock()
    defer r.statsMu.Unlock()

    r.stats.StrategyDistribution[strategy]++

    switch strategy {
    case StrategySequential:
        r.stats.SequentialCount++
    case StrategyRandom:
        r.stats.RandomCount++
    case StrategyWeighted:
        r.stats.WeightedCount++
    case StrategyTimeBased:
        r.stats.TimeBasedCount++
    case StrategyHealthBased:
        r.stats.HealthBasedCount++
    }
}

func (r *Rotator) incrementSkippedUnhealthy() {
    r.statsMu.Lock()
    defer r.statsMu.Unlock()

    r.stats.SkippedUnhealthy++
}

func (r *Rotator) incrementFailedRotations() {
    r.statsMu.Lock()
    defer r.statsMu.Unlock()

    r.stats.FailedRotations++
}

func (r *Rotator) secureRandom(max int) (int, error) {
    if max <= 0 {
        return 0, errors.New("max must be greater than 0")
    }

    nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
    if err != nil {
        return 0, err
    }

    return int(nBig.Int64()), nil
}

func (r *Rotator) randomFloat64() float64 {
    max := big.NewInt(1_000_000)
    nBig, err := rand.Int(rand.Reader, max)
    if err != nil {
        return 0.5
    }

    return float64(nBig.Int64()) / 1_000_000.0
}

func (r *Rotator) SetConfig(config *RotatorConfig) {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.config = config
    r.strategy = config.Strategy

    r.log.Info(
        "rotator config updated",
        logger.String("strategy", string(config.Strategy)),
        logger.Bool("health_filtering", config.EnableHealthFiltering),
    )
}

func (r *Rotator) GetConfig() *RotatorConfig {
    r.mu.Lock()
    defer r.mu.Unlock()

    configCopy := *r.config
    return &configCopy
}

func (r *Rotator) ResetStats() {
    r.statsMu.Lock()
    defer r.statsMu.Unlock()

    r.stats = &RotationStats{
        TemplateUsageCount:   make(map[string]int64),
        StrategyDistribution: make(map[RotationStrategy]int64),
    }

    r.log.Info("rotation stats reset")
}

func (r *Rotator) GetTemplateUsage(templateID string) int64 {
    r.statsMu.RLock()
    defer r.statsMu.RUnlock()

    return r.stats.TemplateUsageCount[templateID]
}

func (r *Rotator) GetTotalRotations() int64 {
    r.statsMu.RLock()
    defer r.statsMu.RUnlock()

    return r.stats.TotalRotations
}

func (r *Rotator) GetLastRotationTime() time.Time {
    r.statsMu.RLock()
    defer r.statsMu.RUnlock()

    return r.stats.LastRotation
}

func (r *Rotator) IsTemplateEligible(templateID string) bool {
    templates := r.getEligibleTemplates()

    for _, tmpl := range templates {
        if tmpl.Template.ID == templateID {
            return true
        }
    }

    return false
}

func (r *Rotator) AddTimeSlotMapping(slot string, templateIDs []string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.timeSlots[slot] = templateIDs

    r.log.Debug(
        "time slot mapping added",
        logger.String("slot", slot),
        logger.Int("template_count", len(templateIDs)),
    )
}

func (r *Rotator) GetTimeSlotMapping(slot string) []string {
    r.mu.Lock()
    defer r.mu.Unlock()

    if ids, exists := r.timeSlots[slot]; exists {
        result := make([]string, len(ids))
        copy(result, ids)
        return result
    }

    return []string{}
}

func (r *Rotator) SetWeight(templateID string, weight float64) {
    r.weightsMu.Lock()
    defer r.weightsMu.Unlock()

    if weight < 0 {
        weight = 0
    }
    if weight > 10 {
        weight = 10
    }

    r.weights[templateID] = weight

    r.log.Debug(
        "template weight updated",
        logger.String("template_id", templateID),
        logger.Float64("weight", weight),
    )
}

func (r *Rotator) GetWeight(templateID string) float64 {
    r.weightsMu.RLock()
    defer r.weightsMu.RUnlock()

    if weight, exists := r.weights[templateID]; exists {
        return weight
    }

    return 1.0
}

func (r *Rotator) ExportStats() *RotationStats {
    r.statsMu.RLock()
    defer r.statsMu.RUnlock()

    stats := &RotationStats{
        TotalRotations:       r.stats.TotalRotations,
        SequentialCount:      r.stats.SequentialCount,
        RandomCount:          r.stats.RandomCount,
        WeightedCount:        r.stats.WeightedCount,
        TimeBasedCount:       r.stats.TimeBasedCount,
        HealthBasedCount:     r.stats.HealthBasedCount,
        SkippedUnhealthy:     r.stats.SkippedUnhealthy,
        FailedRotations:      r.stats.FailedRotations,
        AverageRotationTime:  r.stats.AverageRotationTime,
        LastRotation:         r.stats.LastRotation,
        TemplateUsageCount:   make(map[string]int64),
        StrategyDistribution: make(map[RotationStrategy]int64),
    }

    for k, v := range r.stats.TemplateUsageCount {
        stats.TemplateUsageCount[k] = v
    }

    for k, v := range r.stats.StrategyDistribution {
        stats.StrategyDistribution[k] = v
    }

    return stats
}

func (r *Rotator) ValidateStrategy(strategy string) error {
    validStrategies := []RotationStrategy{
        StrategySequential,
        StrategyRandom,
        StrategyWeighted,
        StrategyTimeBased,
        StrategyHealthBased,
        StrategyLeastUsed,
        StrategyRoundRobin,
    }

    for _, valid := range validStrategies {
        if RotationStrategy(strategy) == valid {
            return nil
        }
    }

    return fmt.Errorf("invalid rotation strategy: %s", strategy)
}

func (r *Rotator) GetAvailableStrategies() []string {
    return []string{
        string(StrategySequential),
        string(StrategyRandom),
        string(StrategyWeighted),
        string(StrategyTimeBased),
        string(StrategyHealthBased),
        string(StrategyLeastUsed),
        string(StrategyRoundRobin),
    }
}
