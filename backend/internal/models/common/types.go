package common

type RotationStrategy string

const (
	RotationStrategySequential  RotationStrategy = "sequential"
	RotationStrategyRandom      RotationStrategy = "random"
	RotationStrategyWeighted    RotationStrategy = "weighted"
	RotationStrategyTimeBased   RotationStrategy = "time_based"
	RotationStrategyHealthBased RotationStrategy = "health_based"
	RotationStrategyRoundRobin  RotationStrategy = "round_robin"
)

func (r RotationStrategy) IsValid() bool {
	switch r {
	case RotationStrategySequential, RotationStrategyRandom, RotationStrategyWeighted,
		RotationStrategyTimeBased, RotationStrategyHealthBased, RotationStrategyRoundRobin:
		return true
	}
	return false
}

type EmailStatus string

const (
	EmailStatusPending   EmailStatus = "pending"
	EmailStatusQueued    EmailStatus = "queued"
	EmailStatusSending   EmailStatus = "sending"
	EmailStatusSent      EmailStatus = "sent"
	EmailStatusDelivered EmailStatus = "delivered"
	EmailStatusFailed    EmailStatus = "failed"
	EmailStatusBounced   EmailStatus = "bounced"
	EmailStatusRetrying  EmailStatus = "retrying"
)

func (s EmailStatus) IsValid() bool {
	switch s {
	case EmailStatusPending, EmailStatusQueued, EmailStatusSending, EmailStatusSent,
		EmailStatusDelivered, EmailStatusFailed, EmailStatusBounced, EmailStatusRetrying:
		return true
	}
	return false
}

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

type Status string

const (
	StatusActive    Status = "active"
	StatusInactive  Status = "inactive"
	StatusSuspended Status = "suspended"
	StatusDisabled  Status = "disabled"
	StatusPending   Status = "pending"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusSuspended, StatusDisabled, StatusPending:
		return true
	}
	return false
}
