package lucy

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Validate checks the structural and path-safety rules for an environment
// request without resolving packages or inspecting external sources.
func (spec EnvironmentSpec) Validate() error {
	if err := validateSafeID("environment id", spec.EnvironmentID); err != nil {
		return err
	}
	if strings.TrimSpace(spec.MinecraftVersion) == "" {
		return errors.New("minecraft version is required")
	}
	if strings.TrimSpace(spec.LoaderType) == "" {
		return errors.New("loader type is required")
	}
	if strings.TrimSpace(spec.ServerCore) == "" {
		return errors.New("server core is required")
	}
	for index, item := range spec.Packages {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("package %d: %w", index, err)
		}
	}
	for index, item := range spec.LocalArtifacts {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("local artifact %d: %w", index, err)
		}
	}
	return nil
}

// Validate checks a provider-neutral package reference.
func (ref PackageRef) Validate() error {
	if err := validateSafeID("package id", ref.ID); err != nil {
		return err
	}
	if strings.TrimSpace(ref.Source) == "" {
		return errors.New("package source is required")
	}
	if strings.TrimSpace(ref.Name) == "" {
		return errors.New("package name is required")
	}
	return nil
}

// Validate checks Artifact payload pairing, sizes, and runtime path safety.
func (ref LocalArtifactRef) Validate() error {
	if err := validateSafeID("artifact id", ref.ArtifactID); err != nil {
		return err
	}
	algorithmSet := strings.TrimSpace(ref.PayloadAlgorithm) != ""
	hashSet := strings.TrimSpace(ref.PayloadHash) != ""
	if algorithmSet != hashSet {
		return errors.New("payload algorithm and hash must be provided together")
	}
	if ref.PayloadSize < 0 {
		return errors.New("payload size must be non-negative")
	}
	if strings.TrimSpace(ref.ArtifactType) == "" {
		return errors.New("artifact type is required")
	}
	if ref.RuntimeName != "" {
		if err := validateRelativePath("runtime name", ref.RuntimeName); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks a planning request and its desired EnvironmentSpec.
func (req PlanEnvironmentRequest) Validate() error {
	return req.Spec.Validate()
}

// Validate checks a locking request and its desired EnvironmentSpec.
func (req LockEnvironmentRequest) Validate() error {
	return req.Spec.Validate()
}

// Validate checks a status request, including its optional lock.
func (req StatusRequest) Validate() error {
	if err := req.Spec.Validate(); err != nil {
		return err
	}
	if req.Lock != nil {
		if err := req.Lock.Validate(); err != nil {
			return fmt.Errorf("lock: %w", err)
		}
	}
	return nil
}

// Validate rejects empty advertised source and loader names.
func (capabilities Capabilities) Validate() error {
	if err := validateNonEmptyEntries("supported source", capabilities.SupportedSources); err != nil {
		return err
	}
	return validateNonEmptyEntries("supported loader", capabilities.SupportedLoaders)
}

// Validate checks every action in a non-executing environment plan.
func (plan EnvironmentPlan) Validate() error {
	for index, action := range plan.Actions {
		if err := action.Validate(); err != nil {
			return fmt.Errorf("action %d: %w", index, err)
		}
	}
	return nil
}

// Validate checks action type, path safety, identifiers, and hash/size
// consistency without executing the action.
func (action PlanAction) Validate() error {
	switch action.ActionType {
	case ActionDownload, ActionCopy, ActionVerify, ActionRemove, ActionLink, ActionWriteConfig:
	default:
		return fmt.Errorf("unsupported action type %q", action.ActionType)
	}
	if action.PackageID != "" {
		if err := validateSafeID("package id", action.PackageID); err != nil {
			return err
		}
	}
	if action.ArtifactID != "" {
		if err := validateSafeID("artifact id", action.ArtifactID); err != nil {
			return err
		}
	}
	if action.Target != "" {
		if err := validateRelativePath("action target", action.Target); err != nil {
			return err
		}
	}
	if action.Size < 0 {
		return errors.New("action size must be non-negative")
	}
	if action.Size > 0 && strings.TrimSpace(action.Hash) == "" {
		return errors.New("action hash is required when size is positive")
	}
	return nil
}

// Validate checks a lock and its nested package and Artifact records. The zero
// value is valid for adapters that do not support locking.
func (lock EnvironmentLock) Validate() error {
	if lock.isZero() {
		return nil
	}
	if err := validateSafeID("lock id", lock.LockID); err != nil {
		return err
	}
	if lock.LockHash != "" && strings.TrimSpace(lock.LockHash) == "" {
		return errors.New("lock hash must not be blank")
	}
	if !lock.GeneratedAt.IsZero() && (lock.GeneratedAt.Year() < 1 || lock.GeneratedAt.Year() > 9999) {
		return errors.New("lock generated time is outside JSON timestamp range")
	}
	for index, item := range lock.Packages {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("locked package %d: %w", index, err)
		}
	}
	for index, item := range lock.Artifacts {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("locked artifact %d: %w", index, err)
		}
	}
	return nil
}

func (lock EnvironmentLock) isZero() bool {
	return lock.LockID == "" && lock.LockHash == "" && lock.GeneratedAt.IsZero() &&
		len(lock.Packages) == 0 && len(lock.Artifacts) == 0 && len(lock.ProviderMetadata) == 0
}

// Validate checks a resolved package record.
func (item LockedPackage) Validate() error {
	if err := validatePackageID("locked package id", item.ID); err != nil {
		return err
	}
	if strings.TrimSpace(item.Source) == "" {
		return errors.New("locked package source is required")
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("locked package name is required")
	}
	if strings.TrimSpace(item.Version) == "" {
		return errors.New("locked package version is required")
	}
	if item.Size < 0 {
		return errors.New("locked package size must be non-negative")
	}
	return nil
}

// Validate checks a resolved Artifact record.
func (item LockedArtifact) Validate() error {
	return LocalArtifactRef{
		ArtifactID:       item.ArtifactID,
		PayloadAlgorithm: item.PayloadAlgorithm,
		PayloadHash:      item.PayloadHash,
		PayloadSize:      item.PayloadSize,
		ArtifactType:     "locked-artifact",
		RuntimeName:      item.RuntimeName,
	}.Validate()
}

// Validate accepts both healthy and unhealthy diagnostic status responses.
func (EnvironmentStatus) Validate() error {
	return nil
}

func (req InstallPackagesRequest) Validate() error {
	if strings.TrimSpace(req.TargetDir) == "" {
		return errors.New("target dir is required")
	}
	if strings.TrimSpace(req.WorkDir) == "" {
		return errors.New("work dir is required")
	}
	for index, item := range req.Packages {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("package %d: %w", index, err)
		}
	}
	return nil
}

func (result InstallPackagesResult) Validate() error {
	switch result.Status {
	case "ok", "partial", "failed", "not_capable":
	case "":
		return errors.New("install status is required")
	default:
		return fmt.Errorf("unsupported install status %q", result.Status)
	}
	if result.TotalSize < 0 {
		return errors.New("install total size must be non-negative")
	}
	for index, item := range result.Installed {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("installed package %d: %w", index, err)
		}
	}
	for index, item := range result.Failed {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("failed package %d: %w", index, err)
		}
	}
	return nil
}

func (item InstalledPackage) Validate() error {
	if err := validatePackageID("installed package id", item.ID); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("installed package name is required")
	}
	if strings.TrimSpace(item.Version) == "" {
		return errors.New("installed package version is required")
	}
	if strings.TrimSpace(item.Path) == "" {
		return errors.New("installed package path is required")
	}
	if item.Size < 0 {
		return errors.New("installed package size must be non-negative")
	}
	return nil
}

func (item FailedPackage) Validate() error {
	if err := validatePackageID("failed package id", item.ID); err != nil {
		return err
	}
	if strings.TrimSpace(item.Error) == "" {
		return errors.New("failed package error is required")
	}
	return nil
}

func (req IntegrityRequest) Validate() error {
	if strings.TrimSpace(req.LockPath) == "" {
		return errors.New("lock path is required")
	}
	if strings.TrimSpace(req.ModsDir) == "" {
		return errors.New("mods dir is required")
	}
	return nil
}

func (result IntegrityResult) Validate() error {
	switch result.Status {
	case "ok", "missing_files", "hash_mismatch", "not_checked":
	case "":
		return errors.New("integrity status is required")
	default:
		return fmt.Errorf("unsupported integrity status %q", result.Status)
	}
	if result.Checked < 0 {
		return errors.New("integrity checked count must be non-negative")
	}
	return nil
}

func validateSafeID(label, value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("%s is required", label)
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("%s %q contains unsupported characters", label, value)
	}
	return nil
}

func validatePackageID(label, value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("%s is required", label)
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 {
		return fmt.Errorf("%s %q contains unsupported characters", label, value)
	}
	for _, part := range parts {
		if err := validateSafeID(label, part); err != nil {
			return err
		}
	}
	return nil
}

func validateRelativePath(label, value string) error {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if normalized == "" || path.IsAbs(normalized) || filepath.IsAbs(filepath.FromSlash(normalized)) || hasWindowsVolumePrefix(normalized) {
		return fmt.Errorf("%s must be relative", label)
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s escapes its root", label)
	}
	return nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') ||
		(value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func validateNonEmptyEntries(label string, values []string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s %d must not be empty", label, index)
		}
	}
	return nil
}
