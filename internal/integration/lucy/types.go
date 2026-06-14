package lucy

import "time"

// ActionType identifies a planned filesystem or dependency action. Actions are
// descriptive only; this package does not execute them.
type ActionType string

const (
	ActionDownload    ActionType = "download"
	ActionCopy        ActionType = "copy"
	ActionVerify      ActionType = "verify"
	ActionRemove      ActionType = "remove"
	ActionLink        ActionType = "link"
	ActionWriteConfig ActionType = "write_config"
)

// EnvironmentSpec describes the desired environment using Stratum-owned
// values rather than Lucy manifest or provider types.
type EnvironmentSpec struct {
	EnvironmentID    string             `json:"environment_id"`
	MinecraftVersion string             `json:"minecraft_version"`
	JavaVersion      string             `json:"java_version,omitempty"`
	LoaderType       string             `json:"loader_type"`
	LoaderVersion    string             `json:"loader_version,omitempty"`
	ServerCore       string             `json:"server_core"`
	CarpetRequired   bool               `json:"carpet_required"`
	MCDRRequired     bool               `json:"mcdr_required"`
	RuntimeProfileID string             `json:"runtime_profile_id,omitempty"`
	Packages         []PackageRef       `json:"packages"`
	LocalArtifacts   []LocalArtifactRef `json:"local_artifacts"`
	Metadata         map[string]string  `json:"metadata"`
}

// PackageRef is a provider-neutral dependency request.
type PackageRef struct {
	ID                string            `json:"id"`
	Source            string            `json:"source"`
	Name              string            `json:"name"`
	VersionConstraint string            `json:"version_constraint,omitempty"`
	MinecraftVersion  string            `json:"minecraft_version,omitempty"`
	Loader            string            `json:"loader,omitempty"`
	Required          bool              `json:"required"`
	Metadata          map[string]string `json:"metadata"`
}

// LocalArtifactRef describes a verified Stratum Artifact payload without
// exposing Controller repository or blob-store types.
type LocalArtifactRef struct {
	ArtifactID       string            `json:"artifact_id"`
	PayloadAlgorithm string            `json:"payload_algorithm"`
	PayloadHash      string            `json:"payload_hash"`
	PayloadSize      int64             `json:"payload_size"`
	ArtifactType     string            `json:"artifact_type"`
	RuntimeName      string            `json:"runtime_name"`
	Metadata         map[string]string `json:"metadata"`
}

// PlanEnvironmentRequest asks an adapter to describe changes for a desired
// environment without applying them.
type PlanEnvironmentRequest struct {
	Spec EnvironmentSpec `json:"spec"`
}

// LockEnvironmentRequest asks an adapter to produce resolved, portable lock
// metadata for a desired environment.
type LockEnvironmentRequest struct {
	Spec EnvironmentSpec `json:"spec"`
}

// StatusRequest asks an adapter to compare a desired environment and optional
// lock with adapter-observed state.
type StatusRequest struct {
	Spec EnvironmentSpec  `json:"spec"`
	Lock *EnvironmentLock `json:"lock,omitempty"`
}

// Capabilities describes supported adapter operations and data sources.
type Capabilities struct {
	SupportsPlan     bool              `json:"supports_plan"`
	SupportsLock     bool              `json:"supports_lock"`
	SupportsStatus   bool              `json:"supports_status"`
	SupportedSources []string          `json:"supported_sources"`
	SupportedLoaders []string          `json:"supported_loaders"`
	Metadata         map[string]string `json:"metadata"`
}

// EnvironmentPlan is a non-executing description of proposed environment
// changes and validation findings.
type EnvironmentPlan struct {
	Actions            []PlanAction      `json:"actions"`
	Warnings           []string          `json:"warnings"`
	Errors             []string          `json:"errors"`
	RequiresLockUpdate bool              `json:"requires_lock_update"`
	Metadata           map[string]string `json:"metadata"`
}

// PlanAction describes one proposed operation without granting permission to
// execute it.
type PlanAction struct {
	ActionType ActionType        `json:"action_type"`
	PackageID  string            `json:"package_id,omitempty"`
	ArtifactID string            `json:"artifact_id,omitempty"`
	Source     string            `json:"source,omitempty"`
	Target     string            `json:"target,omitempty"`
	Hash       string            `json:"hash,omitempty"`
	Size       int64             `json:"size,omitempty"`
	Metadata   map[string]string `json:"metadata"`
}

// EnvironmentLock records provider-neutral resolved package and Artifact
// identities. It is not a Lucy lock-file schema.
type EnvironmentLock struct {
	LockID           string            `json:"lock_id"`
	LockHash         string            `json:"lock_hash"`
	GeneratedAt      time.Time         `json:"generated_at"`
	Packages         []LockedPackage   `json:"packages"`
	Artifacts        []LockedArtifact  `json:"artifacts"`
	ProviderMetadata map[string]string `json:"provider_metadata"`
}

// LockedPackage records one resolved package in an EnvironmentLock.
type LockedPackage struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Hash     string            `json:"hash,omitempty"`
	Size     int64             `json:"size,omitempty"`
	Metadata map[string]string `json:"metadata"`
}

// LockedArtifact records one resolved Stratum Artifact in an EnvironmentLock.
type LockedArtifact struct {
	ArtifactID       string            `json:"artifact_id"`
	PayloadAlgorithm string            `json:"payload_algorithm"`
	PayloadHash      string            `json:"payload_hash"`
	PayloadSize      int64             `json:"payload_size"`
	RuntimeName      string            `json:"runtime_name"`
	Metadata         map[string]string `json:"metadata"`
}

// EnvironmentStatus summarizes missing or drifted environment components.
type EnvironmentStatus struct {
	OK       bool              `json:"ok"`
	Missing  []string          `json:"missing"`
	Drifted  []string          `json:"drifted"`
	Warnings []string          `json:"warnings"`
	Errors   []string          `json:"errors"`
	Metadata map[string]string `json:"metadata"`
}
