package rotation

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCustomFieldNotFound   = errors.New("rotation: custom field not found")
	ErrCustomFieldNoValues   = errors.New("rotation: custom field has no values")
	ErrCustomFieldBadWeights = errors.New("rotation: custom field invalid weights")
)

type CustomFieldRotator struct {
	mu     sync.RWMutex
	fields map[string]*fieldState
}

type fieldState struct {
	mu        sync.Mutex
	name      string
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

func NewCustomFieldRotator() *CustomFieldRotator {
	return &CustomFieldRotator{
		fields: make(map[string]*fieldState),
	}
}

func (r *CustomFieldRotator) Kind() Kind { return KindCustomField }

func (r *CustomFieldRotator) Strategy() Strategy {
	return StrategySequential
}

func (r *CustomFieldRotator) Configure(cfg Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fieldName := fmt.Sprintf("FIELD_%d", time.Now().UnixNano())
	fieldName = normalizeFieldName(fieldName)

	strat := cfg.Strategy
	if strat == "" {
		strat = StrategySequential
	}
	switch strat {
	case StrategySequential, StrategyRandom, StrategyWeighted:
	default:
		return errors.New("rotation: custom field unsupported strategy")
	}

	vals := sanitizeCustomFieldValues(cfg.Values)
	if len(vals) == 0 {
		return ErrCustomFieldNoValues
	}

	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	fs := &fieldState{
		name:     fieldName,
		strategy: strat,
		values:   vals,
		seed:     seed,
		rng:      rand.New(rand.NewSource(seed)),
	}

	if strat == StrategyWeighted {
		ws, err := normalizeCustomFieldWeights(vals, cfg.Weights)
		if err != nil {
			return err
		}
		fs.weights = ws
	}

	atomic.StoreUint64(&fs.nextIndex, 0)
	atomic.StoreUint64(&fs.totalCalls, 0)
	fs.updatedAt = time.Now()

	r.fields[fieldName] = fs
	return nil
}

func (r *CustomFieldRotator) ConfigureField(fieldName string, values []string, strategy Strategy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if fieldName == "" {
		return errors.New("rotation: custom field name required")
	}
	fieldName = normalizeFieldName(fieldName)

	strat := strategy
	if strat == "" {
		strat = StrategySequential
	}
	switch strat {
	case StrategySequential, StrategyRandom, StrategyWeighted:
	default:
		return errors.New("rotation: custom field unsupported strategy")
	}

	vals := sanitizeCustomFieldValues(values)
	if len(vals) == 0 {
		return ErrCustomFieldNoValues
	}

	seed := time.Now().UnixNano()

	fs := &fieldState{
		name:     fieldName,
		strategy: strat,
		values:   vals,
		seed:     seed,
		rng:      rand.New(rand.NewSource(seed)),
	}

	atomic.StoreUint64(&fs.nextIndex, 0)
	atomic.StoreUint64(&fs.totalCalls, 0)
	fs.updatedAt = time.Now()

	r.fields[fieldName] = fs
	return nil
}

func (r *CustomFieldRotator) Next(req RotateRequest) (RotateResult, error) {
	fieldName := req.Meta["field_name"]
	if fieldName == "" {
		fieldName = req.Key
	}
	if fieldName == "" {
		return RotateResult{}, errors.New("rotation: field name required")
	}
	fieldName = normalizeFieldName(fieldName)

	r.mu.RLock()
	fs := r.fields[fieldName]
	r.mu.RUnlock()

	if fs == nil {
		return RotateResult{}, ErrCustomFieldNotFound
	}

	return fs.next(req)
}

func (r *CustomFieldRotator) Peek(req RotateRequest) (RotateResult, error) {
	fieldName := req.Meta["field_name"]
	if fieldName == "" {
		fieldName = req.Key
	}
	if fieldName == "" {
		return RotateResult{}, errors.New("rotation: field name required")
	}
	fieldName = normalizeFieldName(fieldName)

	r.mu.RLock()
	fs := r.fields[fieldName]
	r.mu.RUnlock()

	if fs == nil {
		return RotateResult{}, ErrCustomFieldNotFound
	}

	return fs.peek(req)
}

func (r *CustomFieldRotator) Reset(scope Scope) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if scope.Key != "" {
		fieldName := normalizeFieldName(scope.Key)
		fs := r.fields[fieldName]
		if fs == nil {
			return ErrCustomFieldNotFound
		}
		return fs.reset()
	}

	for _, fs := range r.fields {
		_ = fs.reset()
	}
	return nil
}

func (r *CustomFieldRotator) Stats() Stats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var totalCalls uint64

	for _, fs := range r.fields {
		fs.mu.Lock()
		calls := atomic.LoadUint64(&fs.totalCalls)
		totalCalls += calls
		fs.mu.Unlock()
	}

	return Stats{
		Kind:       KindCustomField,
		Strategy:   StrategySequential,
		TotalCalls: totalCalls,
	}
}

func (r *CustomFieldRotator) GetFieldValue(fieldName string) (string, error) {
	fieldName = normalizeFieldName(fieldName)

	r.mu.RLock()
	fs := r.fields[fieldName]
	r.mu.RUnlock()

	if fs == nil {
		return "", ErrCustomFieldNotFound
	}

	req := RotateRequest{
		Kind: KindCustomField,
		Key:  fieldName,
		Now:  time.Now(),
		Meta: map[string]string{"field_name": fieldName},
	}

	result, err := fs.next(req)
	if err != nil {
		return "", err
	}
	return result.Value, nil
}

func (r *CustomFieldRotator) HasField(fieldName string) bool {
	fieldName = normalizeFieldName(fieldName)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fields[fieldName] != nil
}

func (r *CustomFieldRotator) ListFields() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.fields))
	for name := range r.fields {
		names = append(names, name)
	}
	return names
}

func (fs *fieldState) next(req RotateRequest) (RotateResult, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(fs.values) == 0 {
		return RotateResult{}, ErrCustomFieldNoValues
	}

	idx, val, w, err := fs.pickLocked(true)
	if err != nil {
		return RotateResult{}, err
	}

	fs.lastValue = val
	fs.lastIndex = idx
	fs.updatedAt = req.Now
	atomic.AddUint64(&fs.totalCalls, 1)

	return RotateResult{
		Value:    val,
		Strategy: fs.strategy,
		Index:    idx,
		Weight:   w,
		At:       req.Now,
	}, nil
}

func (fs *fieldState) peek(req RotateRequest) (RotateResult, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(fs.values) == 0 {
		return RotateResult{}, ErrCustomFieldNoValues
	}

	idx, val, w, err := fs.pickLocked(false)
	if err != nil {
		return RotateResult{}, err
	}

	return RotateResult{
		Value:    val,
		Strategy: fs.strategy,
		Index:    idx,
		Weight:   w,
		At:       req.Now,
	}, nil
}

func (fs *fieldState) reset() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	atomic.StoreUint64(&fs.nextIndex, 0)
	atomic.StoreUint64(&fs.totalCalls, 0)
	fs.lastValue = ""
	fs.lastIndex = -1
	fs.updatedAt = time.Now()
	return nil
}

func (fs *fieldState) pickLocked(advance bool) (int, string, float64, error) {
	switch fs.strategy {
	case StrategySequential:
		n := uint64(len(fs.values))
		cur := atomic.LoadUint64(&fs.nextIndex)
		var idx int
		if advance {
			idx = int(atomic.AddUint64(&fs.nextIndex, 1)-1) % int(n)
		} else {
			idx = int(cur % n)
		}
		return idx, fs.values[idx], 0, nil

	case StrategyRandom:
		idx := fs.rng.Intn(len(fs.values))
		return idx, fs.values[idx], 0, nil

	case StrategyWeighted:
		if len(fs.weights) != len(fs.values) {
			return 0, "", 0, ErrCustomFieldBadWeights
		}
		idx := weightedCustomFieldIndex(fs.rng, fs.weights)
		w := fs.weights[idx]
		return idx, fs.values[idx], w, nil

	default:
		return 0, "", 0, fmt.Errorf("rotation: unsupported strategy %s", fs.strategy)
	}
}

func normalizeFieldName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToUpper(name)
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func sanitizeCustomFieldValues(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func normalizeCustomFieldWeights(values []string, weights map[string]float64) ([]float64, error) {
	if len(values) == 0 {
		return nil, ErrCustomFieldNoValues
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
			return nil, ErrCustomFieldBadWeights
		}
		lut[kk] = v
	}

	ws := make([]float64, len(values))
	var sum float64
	for i, v := range values {
		w := lut[strings.ToLower(v)]
		if w <= 0 {
			w = 1
		}
		ws[i] = w
		sum += w
	}
	if sum <= 0 {
		return nil, ErrCustomFieldBadWeights
	}
	return ws, nil
}

func weightedCustomFieldIndex(rng *rand.Rand, weights []float64) int {
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
