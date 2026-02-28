package rotation

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type StrategyPicker struct {
	Strategy Strategy
	Values   []string
	Weights  []float64
	RNG      *rand.Rand
}

func NewStrategyPicker(strategy Strategy, values []string, seed int64) *StrategyPicker {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &StrategyPicker{
		Strategy: strategy,
		Values:   values,
		RNG:      rand.New(rand.NewSource(seed)),
	}
}

func (p *StrategyPicker) Pick(index uint64, meta map[string]string) (int, string, error) {
	if len(p.Values) == 0 {
		return 0, "", fmt.Errorf("no values available")
	}

	switch p.Strategy {
	case StrategySequential:
		idx := int(index % uint64(len(p.Values)))
		return idx, p.Values[idx], nil

	case StrategyRandom:
		idx := p.RNG.Intn(len(p.Values))
		return idx, p.Values[idx], nil

	case StrategyWeighted:
		if len(p.Weights) != len(p.Values) {
			return 0, "", fmt.Errorf("weights length mismatch")
		}
		idx := pickWeighted(p.RNG, p.Weights)
		return idx, p.Values[idx], nil

	case StrategyTimeBased:
		now := time.Now()
		if ts, ok := meta["timestamp"]; ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				now = t
			}
		}
		idx := pickTimeBased(p.RNG, p.Values, now, meta)
		return idx, p.Values[idx], nil

	default:
		return 0, "", fmt.Errorf("unknown strategy: %s", p.Strategy)
	}
}

func ValidateStrategy(s Strategy) error {
	switch s {
	case StrategySequential, StrategyRandom, StrategyWeighted, StrategyTimeBased:
		return nil
	default:
		return fmt.Errorf("invalid strategy: %s", s)
	}
}

func NormalizeWeights(values []string, weights map[string]float64) ([]float64, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("no values to normalize")
	}

	result := make([]float64, len(values))
	
	if len(weights) == 0 {
		for i := range result {
			result[i] = 1.0
		}
		return result, nil
	}

	lut := make(map[string]float64, len(weights))
	for k, v := range weights {
		key := strings.ToLower(strings.TrimSpace(k))
		if key != "" && v >= 0 {
			lut[key] = v
		}
	}

	var total float64
	for i, val := range values {
		key := strings.ToLower(strings.TrimSpace(val))
		w := lut[key]
		if w <= 0 {
			w = 1.0
		}
		result[i] = w
		total += w
	}

	if total <= 0 {
		return nil, fmt.Errorf("total weight must be positive")
	}

	return result, nil
}

func pickWeighted(rng *rand.Rand, weights []float64) int {
	if len(weights) == 0 {
		return 0
	}

	var total float64
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}

	if total <= 0 {
		return rng.Intn(len(weights))
	}

	r := rng.Float64() * total
	var cumulative float64

	for i, w := range weights {
		if w <= 0 {
			continue
		}
		cumulative += w
		if r <= cumulative {
			return i
		}
	}

	return len(weights) - 1
}

func pickTimeBased(rng *rand.Rand, values []string, now time.Time, meta map[string]string) int {
	if len(values) == 0 {
		return 0
	}

	slot := timeSlot(now)
	keywords := getTimeKeywords(slot)

	scored := make([]struct {
		index int
		score int
	}, 0, len(values))

	for i, val := range values {
		lower := strings.ToLower(val)
		score := 0

		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				score += 2
			}
		}

		if contextKey, ok := meta["context"]; ok {
			contextKeywords := strings.Fields(strings.ToLower(contextKey))
			for _, ck := range contextKeywords {
				if len(ck) > 3 && strings.Contains(lower, ck) {
					score++
				}
			}
		}

		if score > 0 {
			scored = append(scored, struct {
				index int
				score int
			}{i, score})
		}
	}

	if len(scored) == 0 {
		return rng.Intn(len(values))
	}

	maxScore := 0
	for _, s := range scored {
		if s.score > maxScore {
			maxScore = s.score
		}
	}

	candidates := make([]int, 0, len(scored))
	for _, s := range scored {
		if s.score >= maxScore {
			candidates = append(candidates, s.index)
		}
	}

	if len(candidates) == 0 {
		return rng.Intn(len(values))
	}

	return candidates[rng.Intn(len(candidates))]
}

func timeSlot(t time.Time) string {
	hour := t.Hour()
	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

func getTimeKeywords(slot string) []string {
	switch slot {
	case "morning":
		return []string{"morning", "start", "begin", "good morning", "breakfast", "early", "wake", "fresh"}
	case "afternoon":
		return []string{"afternoon", "lunch", "midday", "noon", "business", "work", "office"}
	case "evening":
		return []string{"evening", "dinner", "tonight", "good evening", "end", "close", "late"}
	case "night":
		return []string{"night", "tonight", "midnight", "late", "overnight", "good night"}
	default:
		return nil
	}
}

func CalculateDistribution(values []string, weights []float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}

	dist := make(map[string]float64, len(values))

	if len(weights) == 0 || len(weights) != len(values) {
		pct := 100.0 / float64(len(values))
		for _, v := range values {
			dist[v] = pct
		}
		return dist
	}

	var total float64
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}

	if total <= 0 {
		pct := 100.0 / float64(len(values))
		for _, v := range values {
			dist[v] = pct
		}
		return dist
	}

	for i, v := range values {
		if weights[i] > 0 {
			dist[v] = (weights[i] / total) * 100.0
		} else {
			dist[v] = 0.0
		}
	}

	return dist
}

func ShuffleValues(values []string, seed int64) []string {
	if len(values) <= 1 {
		return values
	}

	result := make([]string, len(values))
	copy(result, values)

	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})

	return result
}

func BalanceWeights(weights []float64) []float64 {
	if len(weights) == 0 {
		return nil
	}

	result := make([]float64, len(weights))
	var total float64

	for i, w := range weights {
		if w > 0 {
			result[i] = w
			total += w
		} else {
			result[i] = 0
		}
	}

	if total <= 0 {
		for i := range result {
			result[i] = 1.0
		}
		return result
	}

	for i := range result {
		if result[i] > 0 {
			result[i] = result[i] / total
		}
	}

	return result
}

func FindBestMatch(values []string, query string, fuzzy bool) int {
	if len(values) == 0 || query == "" {
		return -1
	}

	query = strings.ToLower(strings.TrimSpace(query))

	for i, v := range values {
		if strings.ToLower(strings.TrimSpace(v)) == query {
			return i
		}
	}

	if !fuzzy {
		return -1
	}

	bestIdx := -1
	bestScore := 0

	for i, v := range values {
		lower := strings.ToLower(v)
		score := 0

		if strings.Contains(lower, query) {
			score = len(query) * 10
		} else if strings.HasPrefix(lower, query) {
			score = len(query) * 8
		} else {
			words := strings.Fields(query)
			for _, word := range words {
				if len(word) > 2 && strings.Contains(lower, word) {
					score += len(word)
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestScore > 0 {
		return bestIdx
	}

	return -1
}

func CloneValues(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func CloneWeights(weights []float64) []float64 {
	if weights == nil {
		return nil
	}
	result := make([]float64, len(weights))
	copy(result, weights)
	return result
}

func MergeStrategies(primary, fallback Strategy) Strategy {
	if primary != "" && ValidateStrategy(primary) == nil {
		return primary
	}
	if fallback != "" && ValidateStrategy(fallback) == nil {
		return fallback
	}
	return StrategySequential
}

func GetStrategyName(s Strategy) string {
	switch s {
	case StrategySequential:
		return "Sequential"
	case StrategyRandom:
		return "Random"
	case StrategyWeighted:
		return "Weighted"
	case StrategyTimeBased:
		return "Time-Based"
	default:
		return "Unknown"
	}
}

func ParseStrategy(s string) (Strategy, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "sequential", "seq":
		return StrategySequential, nil
	case "random", "rand":
		return StrategyRandom, nil
	case "weighted", "weight":
		return StrategyWeighted, nil
	case "time", "timebased", "time-based", "time_based":
		return StrategyTimeBased, nil
	default:
		return "", fmt.Errorf("unknown strategy: %s", s)
	}
}
