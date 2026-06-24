package lucy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lucyinstall "github.com/mclucy/lucy/install"
	lucystate "github.com/mclucy/lucy/state"
	lucytypes "github.com/mclucy/lucy/types"
)

type LucyProjectBackend struct {
	workDir string
}

var _ EmbeddedBackend = (*LucyProjectBackend)(nil)

func NewLucyProjectBackend(workDir string) *LucyProjectBackend {
	return &LucyProjectBackend{workDir: workDir}
}

func (b *LucyProjectBackend) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		SupportsPlan:     true,
		SupportsLock:     true,
		SupportsStatus:   true,
		SupportedSources: []string{"modrinth", "curseforge", "github-release"},
		SupportedLoaders: []string{"fabric", "forge", "quilt", "liteloader"},
		Metadata:         map[string]string{"backend": "lucy-project"},
	}, nil
}

func (b *LucyProjectBackend) Plan(ctx context.Context, spec EnvironmentSpec) (EnvironmentPlan, error) {
	if err := ctx.Err(); err != nil {
		return EnvironmentPlan{}, err
	}
	manifest := ManifestToLucyManifest(spec)
	plan := EnvironmentPlan{
		Actions:  []PlanAction{},
		Warnings: []string{},
		Errors:   []string{},
		Metadata: map[string]string{"backend": "lucy-project"},
	}
	existing, err := NewManifestService(b.workDir).Read(ctx)
	if err != nil {
		plan.RequiresLockUpdate = true
		plan.Warnings = append(plan.Warnings, "lucy manifest is not readable; lock update required")
		return plan, nil
	}
	plan.RequiresLockUpdate = !sameManifestIntent(existing, manifest)
	return plan, nil
}

func (b *LucyProjectBackend) Lock(ctx context.Context, spec EnvironmentSpec) (EnvironmentLock, error) {
	manifest := ManifestToLucyManifest(spec)
	if err := NewManifestService(b.workDir).Write(ctx, manifest); err != nil {
		return EnvironmentLock{}, err
	}
	if len(spec.Packages) == 0 {
		lock := emptyLucyLockForManifest(manifest)
		if err := NewLockService(b.workDir).Write(ctx, lock); err != nil {
			return EnvironmentLock{}, err
		}
		return LucyLockToEnvironmentLock(lock), nil
	}
	requests := make([]PackageRequest, 0, len(spec.Packages))
	for _, pkg := range spec.Packages {
		requests = append(requests, PackageRequest{
			Platform: packagePlatform(pkg),
			Name:     pkg.Name,
			Scope:    pkg.Source,
			Version:  packageVersion(pkg),
		})
	}
	lucyRequests := lucyInstallRequests(requests)
	var installResult *lucyinstall.Result
	err := NewInstallService(b.workDir).withWorkDir(ctx, func() error {
		var installErr error
		installResult, installErr = lucyinstall.InstallMany(ctx, lucyRequests, lucyinstall.DefaultOptions())
		return installErr
	})
	if err != nil {
		return EnvironmentLock{}, err
	}
	lock := lucyLockFromInstallResult(b.workDir, manifest, installResult)
	if err := NewLockService(b.workDir).Write(ctx, lock); err != nil {
		return EnvironmentLock{}, err
	}
	return LucyLockToEnvironmentLock(lock), nil
}

func (b *LucyProjectBackend) Status(ctx context.Context, spec EnvironmentSpec, lock *EnvironmentLock) (EnvironmentStatus, error) {
	if err := ctx.Err(); err != nil {
		return EnvironmentStatus{}, err
	}
	info, err := NewProbeService(b.workDir).ServerInfo()
	if err != nil {
		return EnvironmentStatus{}, err
	}
	status := EnvironmentStatus{OK: true, Missing: []string{}, Drifted: []string{}, Warnings: []string{}, Errors: []string{}, Metadata: map[string]string{}}
	for key, value := range info {
		status.Metadata[key] = fmt.Sprint(value)
	}
	if lock != nil && len(lock.Packages) > 0 {
		result, err := NewProbeService(b.workDir).VerifyLockIntegrity(ctx, filepath.Join(b.workDir, "lucy-lock.yaml"), filepath.Join(b.workDir, "mods"))
		if err != nil {
			status.OK = false
			status.Errors = append(status.Errors, err.Error())
			return status, nil
		}
		status.OK = result.OK
		status.Missing = append(status.Missing, result.Missing...)
		status.Drifted = append(status.Drifted, result.Corrupt...)
		status.Errors = append(status.Errors, result.Errors...)
	}
	_ = spec
	return status, nil
}

func (b *LucyProjectBackend) Install(ctx context.Context, req InstallPackagesRequest) (InstallPackagesResult, error) {
	installed, failed, err := NewInstallService(req.WorkDir).InstallPackages(ctx, PackageRequestsFromLocked(req.Packages), req.TargetDir)
	if err != nil {
		return InstallPackagesResult{}, err
	}
	status := "ok"
	if len(failed) > 0 {
		status = "partial"
		if len(installed) == 0 {
			status = "failed"
		}
	}
	totalSize := int64(0)
	for _, pkg := range installed {
		totalSize += pkg.Size
	}
	return InstallPackagesResult{Installed: installed, Failed: failed, Status: status, TotalSize: totalSize}, nil
}

func (b *LucyProjectBackend) VerifyIntegrity(ctx context.Context, req IntegrityRequest) (IntegrityResult, error) {
	return NewProbeService(b.workDir).VerifyIntegrityFromLock(ctx, req.LockPath, req.ModsDir)
}

func ManifestToLucyManifest(spec EnvironmentSpec) *lucystate.Manifest {
	manifest := lucystate.ManifestDefaults()
	manifest.Environment.GameVersion = spec.MinecraftVersion
	manifest.Environment.ServerCore = spec.ServerCore
	manifest.Environment.ModdingPlatform = spec.LoaderType
	manifest.Environment.ModdingPlatformVersion = spec.LoaderVersion
	manifest.Environment.Mcdr = spec.MCDRRequired
	manifest.Packages = make([]lucystate.ManifestPackage, 0, len(spec.Packages))
	for _, pkg := range spec.Packages {
		manifest.Packages = append(manifest.Packages, lucystate.ManifestPackage{
			ID:       packageID(pkg),
			Version:  packageVersion(pkg),
			Source:   pkg.Source,
			Role:     lucystate.RoleRequired,
			Side:     lucystate.SideServer,
			Optional: !pkg.Required,
		})
	}
	return &manifest
}

func LucyLockToEnvironmentLock(lock *lucystate.Lock) EnvironmentLock {
	if lock == nil {
		return EnvironmentLock{Packages: []LockedPackage{}, Artifacts: []LockedArtifact{}, ProviderMetadata: map[string]string{}}
	}
	generatedAt, _ := time.Parse(time.RFC3339, lock.GeneratedAt)
	lockHash, _ := Hash(lock)
	result := EnvironmentLock{
		LockID:           "lucy-lock",
		LockHash:         lockHash,
		GeneratedAt:      generatedAt,
		Packages:         make([]LockedPackage, 0, len(lock.Packages)),
		Artifacts:        []LockedArtifact{},
		ProviderMetadata: map[string]string{"provider": "lucy", "manifest_fingerprint": lock.ManifestFingerprint},
	}
	for _, pkg := range lock.Packages {
		result.Packages = append(result.Packages, LockedPackage{
			ID:      pkg.ID,
			Source:  pkg.Source,
			Name:    packageNameFromID(pkg.ID),
			Version: pkg.Version,
			Hash:    pkg.Hash,
			Size:    0,
			Metadata: map[string]string{
				"filename":       pkg.Filename,
				"hash_algorithm": pkg.HashAlgorithm,
				"install_path":   pkg.InstallPath,
			},
		})
	}
	return result
}

func sameManifestIntent(left, right *lucystate.Manifest) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftData, leftErr := lucystate.SerializeManifest(left)
	rightData, rightErr := lucystate.SerializeManifest(right)
	return leftErr == nil && rightErr == nil && string(leftData) == string(rightData)
}

func emptyLucyLockForManifest(manifest *lucystate.Manifest) *lucystate.Lock {
	lock := lucystate.NewLock()
	lock.ManifestFingerprint = manifestFingerprint(manifest)
	if manifest != nil {
		lock.GameVersion = manifest.Environment.GameVersion
		lock.Platform = manifest.Environment.ModdingPlatform
		lock.PlatformVersion = manifest.Environment.ModdingPlatformVersion
	}
	if lock.Platform == "" {
		lock.Platform = "none"
	}
	if lock.PlatformVersion == "" {
		lock.PlatformVersion = "unknown"
	}
	return &lock
}

func manifestFingerprint(manifest *lucystate.Manifest) string {
	data, err := lucystate.SerializeManifest(manifest)
	if err != nil {
		return "sha256:absent"
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func packageID(pkg PackageRef) string {
	if strings.Contains(pkg.ID, "/") {
		return pkg.ID
	}
	platform := packagePlatform(pkg)
	if platform == "" {
		return pkg.ID
	}
	return platform + "/" + pkg.Name
}

func packagePlatform(pkg PackageRef) string {
	if pkg.Loader != "" {
		return pkg.Loader
	}
	if index := strings.Index(pkg.ID, "/"); index > 0 {
		return pkg.ID[:index]
	}
	return ""
}

func packageVersion(pkg PackageRef) string {
	if pkg.VersionConstraint != "" {
		return pkg.VersionConstraint
	}
	return "compatible"
}

func lucyInstallRequests(requests []PackageRequest) []lucytypes.PackageRequest {
	converted := make([]lucytypes.PackageRequest, 0, len(requests))
	for _, req := range requests {
		converted = append(converted, lucytypes.PackageRequest{
			FullPackageRef: lucytypes.FullPackageRef{
				PackageRef: lucytypes.PackageRef{
					Platform: lucytypes.PlatformId(req.Platform),
					Name:     lucytypes.BarePackageName(req.Name),
				},
				Scope:   lucytypes.ParseSource(req.Scope),
				Version: lucytypes.BareVersion(req.Version),
			},
		})
	}
	return converted
}

func lucyLockFromInstallResult(workDir string, manifest *lucystate.Manifest, result *lucyinstall.Result) *lucystate.Lock {
	lock := emptyLucyLockForManifest(manifest)
	if result == nil {
		return lock
	}
	lock.Packages = make([]lucystate.LockedPackage, 0, len(result.Installed))
	for _, pkg := range result.Installed {
		provenance := result.Provenance[pkg.Id.StringBase()]
		requester := "root"
		if len(provenance) > 0 {
			requester = provenance[len(provenance)-1]
		}
		filename := ""
		installPath := ""
		if pkg.Path != "" {
			filename = filepath.Base(pkg.Path)
			installPath = relativeLucyInstallPath(workDir, pkg.Path)
		}
		source := "direct"
		url := pkg.FileUrl
		hash := pkg.Hash
		hashAlgorithm := pkg.HashAlgorithm
		if hash == "" {
			hash = "unknown"
		}
		if hashAlgorithm == "" {
			hashAlgorithm = "sha1"
		}
		if pkg.Filename != "" {
			filename = pkg.Filename
		}
		if len(provenance) == 0 {
			provenance = []string{"root"}
		}
		lock.Packages = append(lock.Packages, lucystate.LockedPackage{
			ID:            pkg.Id.StringBase(),
			Version:       pkg.Id.Version.String(),
			Source:        source,
			URL:           url,
			Filename:      filename,
			Hash:          hash,
			HashAlgorithm: hashAlgorithm,
			InstallPath:   installPath,
			Side:          string(lucystate.SideBoth),
			Provenance:    append([]string(nil), provenance...),
			Requester:     requester,
		})
	}
	lock.Packages = lucystate.CanonicalLockedPackages(lock.Packages)
	return lock
}

func relativeLucyInstallPath(workDir, installPath string) string {
	if installPath == "" {
		return ""
	}
	if rel, err := filepath.Rel(workDir, installPath); err == nil {
		return filepath.ToSlash(rel)
	}
	if rel, err := filepath.Rel(os.TempDir(), installPath); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(installPath)
}
