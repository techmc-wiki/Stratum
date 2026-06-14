package lucy

import "context"

// CapabilityProvider reports which optional Lucy adapter operations are
// available without exposing implementation-specific provider details.
type CapabilityProvider interface {
	Capabilities(context.Context) (Capabilities, error)
}

// EnvironmentPlanner computes declarative environment actions without applying
// them to a runtime workspace.
type EnvironmentPlanner interface {
	PlanEnvironment(context.Context, PlanEnvironmentRequest) (EnvironmentPlan, error)
}

// LockProvider produces a portable lock description without requiring callers
// to know Lucy's lock-file schema.
type LockProvider interface {
	LockEnvironment(context.Context, LockEnvironmentRequest) (EnvironmentLock, error)
}

// StatusProvider compares a desired environment with adapter-observed state.
type StatusProvider interface {
	CheckStatus(context.Context, StatusRequest) (EnvironmentStatus, error)
}

// Adapter is the complete Stratum-facing Lucy integration contract.
type Adapter interface {
	CapabilityProvider
	EnvironmentPlanner
	LockProvider
	StatusProvider
}
