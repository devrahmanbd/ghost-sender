package attachment

import (
    "crypto/rand"
    "errors"
    "math/big"
    "sync"
    "sync/atomic"
    "time"
)

type RotationStrategy string

const (
    RotationSequential RotationStrategy = "sequential"
    RotationRandom     RotationStrategy = "random"
    RotationWeighted   RotationStrategy = "weighted"
    RotationTimeBased  RotationStrategy = "time_based"
)

type FormatRotator struct {
    mu       sync.RWMutex
    strategy RotationStrategy
    formats  []Format
    weights  map[Format]int
    index    int32
    stats    *RotationStats
}

type RotationStats struct {
    mu             sync.RWMutex
    totalRotations uint64
    formatUsage    map[Format]uint64
    lastRotation   time.Time
    startTime      time.Time
}

// ← ADDED: Snapshot type without mutex for safe returning
type RotationStatsSnapshot struct {
    TotalRotations uint64
    FormatUsage    map[Format]uint64
    LastRotation   time.Time
    StartTime      time.Time
}

type RotatorConfig struct {
    Strategy RotationStrategy
    Formats  []Format
    Weights  map[Format]int
    Interval time.Duration
}

func NewFormatRotator(formats []Format) *FormatRotator {
    if len(formats) == 0 {
        formats = []Format{FormatPDF, FormatJPG, FormatPNG, FormatWebP}
    }

    return &FormatRotator{
        strategy: RotationSequential,
        formats:  formats,
        index:    -1,
        stats: &RotationStats{
            formatUsage: make(map[Format]uint64),
            startTime:   time.Now(),
        },
    }
}

func NewFormatRotatorWithConfig(cfg *RotatorConfig) *FormatRotator {
    if cfg == nil {
        return NewFormatRotator(nil)
    }

    if len(cfg.Formats) == 0 {
        cfg.Formats = []Format{FormatPDF, FormatJPG, FormatPNG, FormatWebP}
    }

    if cfg.Strategy == "" {
        cfg.Strategy = RotationSequential
    }

    return &FormatRotator{
        strategy: cfg.Strategy,
        formats:  cfg.Formats,
        weights:  cfg.Weights,
        index:    -1,
        stats: &RotationStats{
            formatUsage: make(map[Format]uint64),
            startTime:   time.Now(),
        },
    }
}

func (fr *FormatRotator) Next() Format {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    var format Format

    switch fr.strategy {
    case RotationRandom:
        format = fr.nextRandom()
    case RotationWeighted:
        format = fr.nextWeighted()
    case RotationTimeBased:
        format = fr.nextTimeBased()
    default:
        format = fr.nextSequential()
    }

    fr.recordUsage(format)
    return format
}

func (fr *FormatRotator) nextSequential() Format {
    idx := atomic.AddInt32(&fr.index, 1)
    return fr.formats[int(idx)%len(fr.formats)]
}

func (fr *FormatRotator) nextRandom() Format {
    n, err := rand.Int(rand.Reader, big.NewInt(int64(len(fr.formats))))
    if err != nil {
        return fr.formats[0]
    }
    return fr.formats[n.Int64()]
}

func (fr *FormatRotator) nextWeighted() Format {
    if len(fr.weights) == 0 {
        return fr.nextSequential()
    }

    totalWeight := 0
    for _, format := range fr.formats {
        if weight, exists := fr.weights[format]; exists {
            totalWeight += weight
        } else {
            totalWeight += 1
        }
    }

    n, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
    if err != nil {
        return fr.formats[0]
    }

    target := int(n.Int64())
    current := 0

    for _, format := range fr.formats {
        weight := 1
        if w, exists := fr.weights[format]; exists {
            weight = w
        }

        current += weight
        if current > target {
            return format
        }
    }

    return fr.formats[0]
}

func (fr *FormatRotator) nextTimeBased() Format {
    hour := time.Now().Hour()

    switch {
    case hour >= 0 && hour < 6:
        return FormatPDF
    case hour >= 6 && hour < 12:
        return FormatJPG
    case hour >= 12 && hour < 18:
        return FormatPNG
    default:
        return FormatWebP
    }
}

func (fr *FormatRotator) recordUsage(format Format) {
    fr.stats.mu.Lock()
    defer fr.stats.mu.Unlock()

    atomic.AddUint64(&fr.stats.totalRotations, 1)
    fr.stats.formatUsage[format]++
    fr.stats.lastRotation = time.Now()
}

func (fr *FormatRotator) SetStrategy(strategy RotationStrategy) error {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    switch strategy {
    case RotationSequential, RotationRandom, RotationWeighted, RotationTimeBased:
        fr.strategy = strategy
        return nil
    default:
        return errors.New("invalid rotation strategy")
    }
}

func (fr *FormatRotator) SetWeights(weights map[Format]int) error {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    for format := range weights {
        found := false
        for _, f := range fr.formats {
            if f == format {
                found = true
                break
            }
        }
        if !found {
            return errors.New("weight specified for unsupported format")
        }
    }

    fr.weights = weights
    return nil
}

func (fr *FormatRotator) Reset() {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    atomic.StoreInt32(&fr.index, -1)
    fr.stats = &RotationStats{
        formatUsage: make(map[Format]uint64),
        startTime:   time.Now(),
    }
}

func (fr *FormatRotator) Stats() RotationStatsSnapshot {  // ← FIXED: Return snapshot type
    fr.stats.mu.RLock()
    defer fr.stats.mu.RUnlock()

    snapshot := RotationStatsSnapshot{
        TotalRotations: atomic.LoadUint64(&fr.stats.totalRotations),
        FormatUsage:    make(map[Format]uint64),
        LastRotation:   fr.stats.lastRotation,
        StartTime:      fr.stats.startTime,
    }

    for format, count := range fr.stats.formatUsage {
        snapshot.FormatUsage[format] = count
    }

    return snapshot
}

func (fr *FormatRotator) GetFormats() []Format {
    fr.mu.RLock()
    defer fr.mu.RUnlock()

    formats := make([]Format, len(fr.formats))
    copy(formats, fr.formats)
    return formats
}

func (fr *FormatRotator) GetStrategy() RotationStrategy {
    fr.mu.RLock()
    defer fr.mu.RUnlock()
    return fr.strategy
}

type TemplateRotator struct {
    mu        sync.RWMutex
    templates []string
    strategy  RotationStrategy
    index     int32
    stats     *RotationStats
}

func NewTemplateRotator(templates []string, strategy RotationStrategy) *TemplateRotator {
    if strategy == "" {
        strategy = RotationSequential
    }

    return &TemplateRotator{
        templates: templates,
        strategy:  strategy,
        index:     -1,
        stats: &RotationStats{
            formatUsage: make(map[Format]uint64),
            startTime:   time.Now(),
        },
    }
}

func (tr *TemplateRotator) Next() string {
    tr.mu.Lock()
    defer tr.mu.Unlock()

    if len(tr.templates) == 0 {
        return ""
    }

    var template string

    switch tr.strategy {
    case RotationRandom:
        template = tr.nextRandom()
    default:
        template = tr.nextSequential()
    }

    atomic.AddUint64(&tr.stats.totalRotations, 1)
    tr.stats.lastRotation = time.Now()

    return template
}

func (tr *TemplateRotator) nextSequential() string {
    idx := atomic.AddInt32(&tr.index, 1)
    return tr.templates[int(idx)%len(tr.templates)]
}

func (tr *TemplateRotator) nextRandom() string {
    n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tr.templates))))
    if err != nil {
        return tr.templates[0]
    }
    return tr.templates[n.Int64()]
}

func (tr *TemplateRotator) Reset() {
    tr.mu.Lock()
    defer tr.mu.Unlock()

    atomic.StoreInt32(&tr.index, -1)
    tr.stats = &RotationStats{
        formatUsage: make(map[Format]uint64),
        startTime:   time.Now(),
    }
}

func (tr *TemplateRotator) Stats() RotationStatsSnapshot {  // ← FIXED: Return snapshot type
    tr.stats.mu.RLock()
    defer tr.stats.mu.RUnlock()

    return RotationStatsSnapshot{
        TotalRotations: atomic.LoadUint64(&tr.stats.totalRotations),
        LastRotation:   tr.stats.lastRotation,
        StartTime:      tr.stats.startTime,
    }
}

func (tr *TemplateRotator) Count() int {
    tr.mu.RLock()
    defer tr.mu.RUnlock()
    return len(tr.templates)
}

type CombinedRotator struct {
    formatRotator   *FormatRotator
    templateRotator *TemplateRotator
}

func NewCombinedRotator(formats []Format, templates []string, strategy RotationStrategy) *CombinedRotator {
    return &CombinedRotator{
        formatRotator:   NewFormatRotatorWithConfig(&RotatorConfig{Strategy: strategy, Formats: formats}),
        templateRotator: NewTemplateRotator(templates, strategy),
    }
}

func (cr *CombinedRotator) Next() (string, Format) {
    template := cr.templateRotator.Next()
    format := cr.formatRotator.Next()
    return template, format
}

func (cr *CombinedRotator) NextFormat() Format {
    return cr.formatRotator.Next()
}

func (cr *CombinedRotator) NextTemplate() string {
    return cr.templateRotator.Next()
}

func (cr *CombinedRotator) Reset() {
    cr.formatRotator.Reset()
    cr.templateRotator.Reset()
}

func (cr *CombinedRotator) FormatStats() RotationStatsSnapshot {  // ← FIXED: Return snapshot type
    return cr.formatRotator.Stats()
}

func (cr *CombinedRotator) TemplateStats() RotationStatsSnapshot {  // ← FIXED: Return snapshot type
    return cr.templateRotator.Stats()
}

func ValidateRotationStrategy(strategy string) bool {
    switch RotationStrategy(strategy) {
    case RotationSequential, RotationRandom, RotationWeighted, RotationTimeBased:
        return true
    default:
        return false
    }
}

func ParseRotationStrategy(s string) (RotationStrategy, error) {
    strategy := RotationStrategy(s)
    if ValidateRotationStrategy(s) {
        return strategy, nil
    }
    return "", errors.New("invalid rotation strategy")
}
