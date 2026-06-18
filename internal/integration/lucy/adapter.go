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

// PackageInstaller downloads resolved packages into a target runtime directory.
type PackageInstaller interface {
	InstallPackages(context.Context, InstallPackagesRequest) (InstallPackagesResult, error)
}

type InstallPackagesRequest struct {
	Packages  []LockedPackage `json:"packages"`
	TargetDir string          `json:"target_dir"`
	WorkDir   string          `json:"work_dir"`
}

type InstallPackagesResult struct {
	Installed []InstalledPackage `json:"installed"`
	Failed    []FailedPackage    `json:"failed"`
	Status    string             `json:"status"`
	TotalSize int64              `json:"total_size"`
}

type InstalledPackage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
}

type FailedPackage struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// Adapter is the complete Stratum-facing Lucy integration contract.
type Adapter interface {
	CapabilityProvider
	EnvironmentPlanner
	LockProvider
	StatusProvider
	PackageInstaller
}
