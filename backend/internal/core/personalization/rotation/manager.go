package rotation

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrRotatorNotFound     = errors.New("rotation: rotator not found")
	ErrInvalidRotateRequest = errors.New("rotation: invalid rotate request")
)

type Manager struct {
	mu       sync.RWMutex
	rotators map[Kind]map[string]Rotator
}

func NewManager() *Manager {
	return &Manager{
		rotators: make(map[Kind]map[string]Rotator),
	}
}

func (m *Manager) Register(key string, r Rotator) error {
	if r == nil {
		return errors.New("rotation: nil rotator")
	}
	if key == "" {
		return errors.New("rotation: empty key")
	}
	kind := r.Kind()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.rotators[kind] == nil {
		m.rotators[kind] = make(map[string]Rotator)
	}
	m.rotators[kind][key] = r
	return nil
}

func (m *Manager) Configure(kind Kind, key string, cfg Config) error {
	r, err := m.Get(kind, key)
	if err != nil {
		return err
	}
	return r.Configure(cfg)
}

func (m *Manager) Get(kind Kind, key string) (Rotator, error) {
	if key == "" {
		return nil, errors.New("rotation: empty key")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	byKind := m.rotators[kind]
	if byKind == nil {
		return nil, ErrRotatorNotFound
	}
	r := byKind[key]
	if r == nil {
		return nil, ErrRotatorNotFound
	}
	return r, nil
}

func (m *Manager) Next(req RotateRequest) (RotateResult, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return RotateResult{}, err
	}
	r, err := m.Get(req.Kind, req.Key)
	if err != nil {
		return RotateResult{}, err
	}
	return r.Next(req)
}

func (m *Manager) Peek(req RotateRequest) (RotateResult, error) {
	req, err := normalizeRequest(req)
	if err != nil {
		return RotateResult{}, err
	}
	r, err := m.Get(req.Kind, req.Key)
	if err != nil {
		return RotateResult{}, err
	}
	return r.Peek(req)
}

func (m *Manager) Reset(scope Scope) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if scope.Kind != "" && scope.Key != "" {
		r := m.rotators[scope.Kind][scope.Key]
		if r == nil {
			return ErrRotatorNotFound
		}
		return r.Reset(scope)
	}

	if scope.Kind != "" {
		byKind := m.rotators[scope.Kind]
		if byKind == nil {
			return nil
		}
		for _, r := range byKind {
			_ = r.Reset(scope)
		}
		return nil
	}

	for _, byKind := range m.rotators {
		for _, r := range byKind {
			_ = r.Reset(scope)
		}
	}
	return nil
}

func (m *Manager) Stats() map[Kind]map[string]Stats {
	out := make(map[Kind]map[string]Stats)

	m.mu.RLock()
	defer m.mu.RUnlock()

	for kind, byKey := range m.rotators {
		if byKey == nil {
			continue
		}
		dst := make(map[string]Stats, len(byKey))
		for key, r := range byKey {
			if r == nil {
				continue
			}
			dst[key] = r.Stats()
		}
		out[kind] = dst
	}
	return out
}

func normalizeRequest(req RotateRequest) (RotateRequest, error) {
	if req.Ctx == nil {
		req.Ctx = context.Background()
	}
	if req.Kind == "" {
		return RotateRequest{}, ErrInvalidRotateRequest
	}
	if req.Key == "" {
		return RotateRequest{}, ErrInvalidRotateRequest
	}
	if req.Now.IsZero() {
		req.Now = time.Now()
	}
	if req.Meta == nil {
		req.Meta = map[string]string{}
	}
	return req, nil
}
