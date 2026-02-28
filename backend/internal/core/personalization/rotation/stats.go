package rotation

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type StatsCollector struct {
	mu            sync.RWMutex
	kind          Kind
	strategy      Strategy
	values        []string
	valueCounts   map[string]uint64
	totalCalls    uint64
	startTime     time.Time
	lastCallTime  time.Time
	timeSeries    []*TimePoint
	maxTimeSeries int
}

type TimePoint struct {
	Timestamp time.Time
	Value     string
	Index     int
	Duration  time.Duration
}

type ValueStats struct {
	Value       string
	Count       uint64
	Percentage  float64
	FirstUsed   time.Time
	LastUsed    time.Time
	AvgInterval time.Duration
}

type DistributionReport struct {
	Kind            Kind
	Strategy        Strategy
	TotalCalls      uint64
	UniqueValues    int
	StartTime       time.Time
	LastCallTime    time.Time
	Duration        time.Duration
	CallsPerSecond  float64
	ValueStats      []ValueStats
	Distribution    map[string]float64
	ExpectedDistrib map[string]float64
	Variance        float64
}

func NewStatsCollector(kind Kind, strategy Strategy, values []string) *StatsCollector {
	return &StatsCollector{
		kind:          kind,
		strategy:      strategy,
		values:        values,
		valueCounts:   make(map[string]uint64),
		startTime:     time.Now(),
		timeSeries:    make([]*TimePoint, 0, 1000),
		maxTimeSeries: 1000,
	}
}

func (c *StatsCollector) Record(value string, index int, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.valueCounts[value]++
	c.totalCalls++
	c.lastCallTime = now

	if len(c.timeSeries) >= c.maxTimeSeries {
		c.timeSeries = c.timeSeries[1:]
	}

	c.timeSeries = append(c.timeSeries, &TimePoint{
		Timestamp: now,
		Value:     value,
		Index:     index,
		Duration:  duration,
	})
}

func (c *StatsCollector) GetValueStats() []ValueStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make([]ValueStats, 0, len(c.valueCounts))

	for val, count := range c.valueCounts {
		vs := ValueStats{
			Value: val,
			Count: count,
		}

		if c.totalCalls > 0 {
			vs.Percentage = (float64(count) / float64(c.totalCalls)) * 100.0
		}

		var firstUsed, lastUsed time.Time
		var intervals []time.Duration
		var prevTime time.Time

		for _, tp := range c.timeSeries {
			if tp.Value == val {
				if firstUsed.IsZero() {
					firstUsed = tp.Timestamp
				}
				lastUsed = tp.Timestamp

				if !prevTime.IsZero() {
					intervals = append(intervals, tp.Timestamp.Sub(prevTime))
				}
				prevTime = tp.Timestamp
			}
		}

		vs.FirstUsed = firstUsed
		vs.LastUsed = lastUsed

		if len(intervals) > 0 {
			var totalInterval time.Duration
			for _, iv := range intervals {
				totalInterval += iv
			}
			vs.AvgInterval = totalInterval / time.Duration(len(intervals))
		}

		stats = append(stats, vs)
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	return stats
}

func (c *StatsCollector) GetDistribution() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dist := make(map[string]float64)
	if c.totalCalls == 0 {
		return dist
	}

	for val, count := range c.valueCounts {
		dist[val] = (float64(count) / float64(c.totalCalls)) * 100.0
	}

	return dist
}

func (c *StatsCollector) GetReport() DistributionReport {
	c.mu.RLock()
	defer c.mu.RUnlock()

	report := DistributionReport{
		Kind:         c.kind,
		Strategy:     c.strategy,
		TotalCalls:   c.totalCalls,
		UniqueValues: len(c.values),
		StartTime:    c.startTime,
		LastCallTime: c.lastCallTime,
		ValueStats:   c.GetValueStats(),
		Distribution: c.GetDistribution(),
	}

	if !c.startTime.IsZero() && !c.lastCallTime.IsZero() {
		report.Duration = c.lastCallTime.Sub(c.startTime)
		if report.Duration > 0 {
			report.CallsPerSecond = float64(c.totalCalls) / report.Duration.Seconds()
		}
	}

	report.ExpectedDistrib = c.calculateExpectedDistribution()
	report.Variance = c.calculateVariance(report.Distribution, report.ExpectedDistrib)

	return report
}

func (c *StatsCollector) calculateExpectedDistribution() map[string]float64 {
	expected := make(map[string]float64)

	if len(c.values) == 0 {
		return expected
	}

	switch c.strategy {
	case StrategySequential, StrategyRandom:
		pct := 100.0 / float64(len(c.values))
		for _, val := range c.values {
			expected[val] = pct
		}
	case StrategyWeighted:
		for _, val := range c.values {
			expected[val] = 0
		}
	default:
		pct := 100.0 / float64(len(c.values))
		for _, val := range c.values {
			expected[val] = pct
		}
	}

	return expected
}

func (c *StatsCollector) calculateVariance(actual, expected map[string]float64) float64 {
	if len(actual) == 0 || len(expected) == 0 {
		return 0
	}

	var sumSquaredDiff float64
	var count int

	for val, exp := range expected {
		act := actual[val]
		diff := act - exp
		sumSquaredDiff += diff * diff
		count++
	}

	if count == 0 {
		return 0
	}

	return sumSquaredDiff / float64(count)
}

func (c *StatsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.valueCounts = make(map[string]uint64)
	c.totalCalls = 0
	c.startTime = time.Now()
	c.lastCallTime = time.Time{}
	c.timeSeries = make([]*TimePoint, 0, c.maxTimeSeries)
}

func (c *StatsCollector) GetTimeSeries(limit int) []*TimePoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 || limit > len(c.timeSeries) {
		limit = len(c.timeSeries)
	}

	start := len(c.timeSeries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*TimePoint, limit)
	copy(result, c.timeSeries[start:])
	return result
}

func (c *StatsCollector) ExportJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	report := c.GetReport()
	return json.MarshalIndent(report, "", "  ")
}

func (c *StatsCollector) Summary() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.totalCalls == 0 {
		return fmt.Sprintf("No rotations recorded for %s", c.kind)
	}

	report := c.GetReport()
	summary := fmt.Sprintf("Rotation Statistics - %s (%s)\n", c.kind, c.strategy)
	summary += fmt.Sprintf("Total Calls: %d\n", report.TotalCalls)
	summary += fmt.Sprintf("Unique Values: %d\n", report.UniqueValues)
	summary += fmt.Sprintf("Duration: %s\n", report.Duration)
	summary += fmt.Sprintf("Calls/Second: %.2f\n", report.CallsPerSecond)
	summary += fmt.Sprintf("Variance: %.2f\n", report.Variance)
	summary += "\nTop Values:\n"

	limit := 10
	if len(report.ValueStats) < limit {
		limit = len(report.ValueStats)
	}

	for i := 0; i < limit; i++ {
		vs := report.ValueStats[i]
		summary += fmt.Sprintf("  %s: %d (%.2f%%)\n", vs.Value, vs.Count, vs.Percentage)
	}

	return summary
}

type AggregatedStats struct {
	Collectors      []*StatsCollector
	TotalCalls      uint64
	TotalValues     int
	OverallDuration time.Duration
	ByKind          map[Kind]*DistributionReport
	ByStrategy      map[Strategy]*DistributionReport
}

func AggregateStats(collectors []*StatsCollector) *AggregatedStats {
	agg := &AggregatedStats{
		Collectors: collectors,
		ByKind:     make(map[Kind]*DistributionReport),
		ByStrategy: make(map[Strategy]*DistributionReport),
	}

	for _, c := range collectors {
		report := c.GetReport()
		agg.TotalCalls += report.TotalCalls
		agg.TotalValues += report.UniqueValues

		if report.Duration > agg.OverallDuration {
			agg.OverallDuration = report.Duration
		}

		if _, exists := agg.ByKind[c.kind]; !exists {
			agg.ByKind[c.kind] = &report
		}

		if _, exists := agg.ByStrategy[c.strategy]; !exists {
			agg.ByStrategy[c.strategy] = &report
		}
	}

	return agg
}

func (a *AggregatedStats) Summary() string {
	summary := "Aggregated Rotation Statistics\n"
	summary += fmt.Sprintf("Total Collectors: %d\n", len(a.Collectors))
	summary += fmt.Sprintf("Total Calls: %d\n", a.TotalCalls)
	summary += fmt.Sprintf("Total Values: %d\n", a.TotalValues)
	summary += fmt.Sprintf("Overall Duration: %s\n", a.OverallDuration)

	summary += "\nBy Kind:\n"
	for kind, report := range a.ByKind {
		summary += fmt.Sprintf("  %s: %d calls\n", kind, report.TotalCalls)
	}

	summary += "\nBy Strategy:\n"
	for strategy, report := range a.ByStrategy {
		summary += fmt.Sprintf("  %s: %d calls\n", strategy, report.TotalCalls)
	}

	return summary
}

func CalculateUniformity(dist map[string]float64, expectedPct float64) float64 {
	if len(dist) == 0 {
		return 0
	}

	var sumDiff float64
	for _, pct := range dist {
		diff := pct - expectedPct
		if diff < 0 {
			diff = -diff
		}
		sumDiff += diff
	}

	return 100.0 - (sumDiff / float64(len(dist)))
}

func CompareDistributions(actual, expected map[string]float64) float64 {
	if len(actual) == 0 || len(expected) == 0 {
		return 0
	}

	var totalDiff float64
	var count int

	for key, exp := range expected {
		act := actual[key]
		diff := act - exp
		if diff < 0 {
			diff = -diff
		}
		totalDiff += diff
		count++
	}

	if count == 0 {
		return 0
	}

	avgDiff := totalDiff / float64(count)
	return 100.0 - avgDiff
}
