package rotation

import (
	"errors"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrSenderNameNoValues     = errors.New("rotation: sender_name has no values")
	ErrSenderNameBadWeights   = errors.New("rotation: sender_name invalid weights")
	ErrSenderNameBadTimeMap   = errors.New("rotation: sender_name invalid time map")
	ErrSenderNameBadStrategy  = errors.New("rotation: sender_name invalid strategy")
)

type SenderNameRotator struct {
	mu        sync.Mutex
	strategy  Strategy
	values    []string
	weights   []float64
	seed      int64
	rng       *rand.Rand
	nextIndex uint64

	totalCalls uint64
	lastValue  string
	lastIndex  int
	updatedAt  time.Time
}

func NewSenderNameRotator() *SenderNameRotator {
	return &SenderNameRotator{
		strategy: StrategySequential,
		seed:     time.Now().UnixNano(),
	}
}

func (r *SenderNameRotator) Kind() Kind { return KindSenderName }

func (r *SenderNameRotator) Strategy() Strategy {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.strategy
}

func (r *SenderNameRotator) Configure(cfg Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	strat := cfg.Strategy
	if strat == "" {
		strat = StrategySequential
	}
	switch strat {
	case StrategySequential, StrategyRandom, StrategyWeighted, StrategyTimeBased:
	default:
		return ErrSenderNameBadStrategy
	}

	vals := sanitizeValues(cfg.Values)
	if len(vals) == 0 {
		return ErrSenderNameNoValues
	}

	r.strategy = strat
	r.values = vals
	r.seed = cfg.Seed
	if r.seed == 0 {
		r.seed = time.Now().UnixNano()
	}
	r.rng = rand.New(rand.NewSource(r.seed))

	r.weights = nil
	if strat == StrategyWeighted {
		ws, err := normalizeWeights(vals, cfg.Weights)
		if err != nil {
			return err
		}
		r.weights = ws
	}

	atomic.StoreUint64(&r.nextIndex, 0)
	atomic.StoreUint64(&r.totalCalls, 0)
	r.lastValue = ""
	r.lastIndex = -1
	r.updatedAt = time.Now()
	return nil
}

func (r *SenderNameRotator) Next(req RotateRequest) (RotateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.values) == 0 {
		return RotateResult{}, ErrSenderNameNoValues
	}

	idx, val, w, err := r.pickLocked(req, true)
	if err != nil {
		return RotateResult{}, err
	}

	r.lastValue = val
	r.lastIndex = idx
	r.updatedAt = req.Now
	atomic.AddUint64(&r.totalCalls, 1)

	return RotateResult{
		Value:    val,
		Strategy: r.strategy,
		Index:    idx,
		Weight:   w,
		At:       req.Now,
	}, nil
}

func (r *SenderNameRotator) Peek(req RotateRequest) (RotateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.values) == 0 {
		return RotateResult{}, ErrSenderNameNoValues
	}

	idx, val, w, err := r.pickLocked(req, false)
	if err != nil {
		return RotateResult{}, err
	}

	return RotateResult{
		Value:    val,
		Strategy: r.strategy,
		Index:    idx,
		Weight:   w,
		At:       req.Now,
	}, nil
}

func (r *SenderNameRotator) Reset(scope Scope) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	atomic.StoreUint64(&r.nextIndex, 0)
	atomic.StoreUint64(&r.totalCalls, 0)
	r.lastValue = ""
	r.lastIndex = -1
	r.updatedAt = time.Now()
	return nil
}

func (r *SenderNameRotator) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()

	return Stats{
		Kind:        KindSenderName,
		Strategy:    r.strategy,
		TotalValues: len(r.values),
		TotalCalls:  atomic.LoadUint64(&r.totalCalls),
		LastValue:   r.lastValue,
		LastIndex:   r.lastIndex,
		UpdatedAt:   r.updatedAt,
	}
}

func (r *SenderNameRotator) pickLocked(req RotateRequest, advance bool) (int, string, float64, error) {
	switch r.strategy {
	case StrategySequential:
		n := uint64(len(r.values))
		cur := atomic.LoadUint64(&r.nextIndex)
		var idx int
		if advance {
			idx = int(atomic.AddUint64(&r.nextIndex, 1)-1) % int(n)
		} else {
			idx = int(cur % n)
		}
		return idx, r.values[idx], 0, nil

	case StrategyRandom:
		idx := r.rng.Intn(len(r.values))
		return idx, r.values[idx], 0, nil

	case StrategyWeighted:
		if len(r.weights) != len(r.values) {
			return 0, "", 0, ErrSenderNameBadWeights
		}
		idx := weightedIndex(r.rng, r.weights)
		w := r.weights[idx]
		return idx, r.values[idx], w, nil

	case StrategyTimeBased:
		slot := timeSlot(req.Now)
		candidates := valuesForSlot(r.values, slot)
		if len(candidates) == 0 {
			idx := r.rng.Intn(len(r.values))
			return idx, r.values[idx], 0, nil
		}
		val := candidates[r.rng.Intn(len(candidates))]
		idx := indexOf(r.values, val)
		return idx, val, 0, nil

	default:
		return 0, "", 0, ErrSenderNameBadStrategy
	}
}

func sanitizeValues(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, v := range in {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeWeights(values []string, weights map[string]float64) ([]float64, error) {
	if len(values) == 0 {
		return nil, ErrSenderNameNoValues
	}
	if len(weights) == 0 {
		ws := make([]float64, len(values))
		for i := range ws {
			ws[i] = 1
		}
		return ws, nil
	}

	lut := make(map[string]float64, len(weights))
	for k, v := range weights {
		kk := strings.ToLower(strings.TrimSpace(k))
		if kk == "" {
			continue
		}
		if v < 0 {
			return nil, ErrSenderNameBadWeights
		}
		lut[kk] = v
	}

	ws := make([]float64, len(values))
	var sum float64
	for i, v := range values {
		w := lut[strings.ToLower(v)]
		if w <= 0 {
			w = 0
		}
		ws[i] = w
		sum += w
	}
	if sum <= 0 {
		return nil, ErrSenderNameBadWeights
	}
	return ws, nil
}

func weightedIndex(rng *rand.Rand, weights []float64) int {
	var total float64
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return rng.Intn(len(weights))
	}
	x := rng.Float64() * total
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		if x < w {
			return i
		}
		x -= w
	}
	return len(weights) - 1
}


func valuesForSlot(values []string, slot string) []string {
	type scored struct {
		v     string
		score int
	}
	kw := slotKeywords(slot)
	if len(kw) == 0 {
		cp := append([]string(nil), values...)
		sort.Strings(cp)
		return cp
	}

	sc := make([]scored, 0, len(values))
	for _, v := range values {
		l := strings.ToLower(v)
		s := 0
		for _, k := range kw {
			if strings.Contains(l, k) {
				s++
			}
		}
		if s > 0 {
			sc = append(sc, scored{v: v, score: s})
		}
	}
	sort.Slice(sc, func(i, j int) bool {
		if sc[i].score == sc[j].score {
			return sc[i].v < sc[j].v
		}
		return sc[i].score > sc[j].score
	})
	out := make([]string, len(sc))
	for i := range sc {
		out[i] = sc[i].v
	}
	return out
}

func slotKeywords(slot string) []string {
	switch slot {
	case "morning":
		return []string{"morning", "fresh", "early", "start", "begin"}
	case "afternoon":
		return []string{"business", "professional", "corporate", "office"}
	case "evening":
		return []string{"support", "service", "help", "assist", "care"}
	case "night":
		return []string{"24/7", "247", "always", "anytime", "round", "clock"}
	default:
		return nil
	}
}

func indexOf(values []string, v string) int {
	for i := range values {
		if values[i] == v {
			return i
		}
	}
	return -1
}
