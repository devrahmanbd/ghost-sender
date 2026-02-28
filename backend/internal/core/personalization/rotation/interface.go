package rotation

import (
	"context"
	"time"
)

type Strategy string

const (
	StrategySequential Strategy = "sequential"
	StrategyRandom     Strategy = "random"
	StrategyWeighted   Strategy = "weighted"
	StrategyTimeBased  Strategy = "timebased"
)

type Kind string

const (
	KindSenderName  Kind = "sender_name"
	KindSubject     Kind = "subject"
	KindCustomField Kind = "custom_field"
)

type RotateRequest struct {
	Ctx        context.Context
	TenantID   string
	SessionID  string
	CampaignID string
	Kind       Kind
	Key        string
	Now        time.Time
	Index      int
	Meta       map[string]string
}

type RotateResult struct {
	Value    string
	Strategy Strategy
	Index    int
	Weight   float64
	At       time.Time
}

type Rotator interface {
	Kind() Kind
	Strategy() Strategy
	Configure(cfg Config) error
	Next(req RotateRequest) (RotateResult, error)
	Peek(req RotateRequest) (RotateResult, error)
	Reset(scope Scope) error
	Stats() Stats
}

type Scope struct {
	TenantID   string
	SessionID  string
	CampaignID string
	Kind       Kind
	Key        string
}

type Config struct {
	Strategy Strategy
	Values   []string
	Weights  map[string]float64
	TimeMap  map[string][]string
	Seed     int64
	MinLen   int
	MaxLen   int
}

type Stats struct {
	Kind        Kind
	Strategy    Strategy
	TotalValues int
	TotalCalls  uint64
	LastValue   string
	LastIndex   int
	UpdatedAt   time.Time
}
