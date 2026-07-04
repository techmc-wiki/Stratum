package lucy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	lucyinstall "github.com/mclucy/lucy/install"
	lucytypes "github.com/mclucy/lucy/types"
	lucyworkspace "github.com/mclucy/lucy/workspace"
)

var lucyInstallCwdMu sync.Mutex

// InstallService provides operations for Lucy package installation.
type InstallService struct {
	workDir string
}

// NewInstallService creates an InstallService for the given work directory.
func NewInstallService(workDir string) *InstallService {
	return &InstallService{workDir: workDir}
}

// PackageRequest wraps Lucy's package installation request.
type PackageRequest struct {
	Platform string
	Name     string
	Scope    string
	Version  string
}

// Install installs a single package.
func (s *InstallService) Install(ctx context.Context, req PackageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lucyReq := lucytypes.PackageRequest{
		FullPackageRef: lucytypes.FullPackageRef{
			PackageRef: lucytypes.PackageRef{
				Eco:  lucytypes.Ecosystem(req.Platform),
				Name: lucytypes.BarePackageName(req.Name),
			},
			Scope:   lucytypes.ParseSource(req.Scope),
			Version: lucytypes.BareVersion(req.Version),
		},
	}
	err := s.withWorkDir(ctx, func() error {
		_, err := lucyinstall.Install(ctx, lucyReq, lucyinstall.DefaultOptions())
		return err
	})
	if err != nil {
		return fmt.Errorf("lucy install: %w", err)
	}
	return nil
}

// InstallMany installs multiple packages in a single transaction.
func (s *InstallService) InstallMany(ctx context.Context, requests []PackageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lucyReqs := make([]lucytypes.PackageRequest, len(requests))
	for i, req := range requests {
		lucyReqs[i] = lucytypes.PackageRequest{
			FullPackageRef: lucytypes.FullPackageRef{
				PackageRef: lucytypes.PackageRef{
					Eco:  lucytypes.Ecosystem(req.Platform),
					Name: lucytypes.BarePackageName(req.Name),
				},
				Scope:   lucytypes.ParseSource(req.Scope),
				Version: lucytypes.BareVersion(req.Version),
			},
		}
	}
	err := s.withWorkDir(ctx, func() error {
		_, err := lucyinstall.InstallMany(ctx, lucyReqs, lucyinstall.DefaultOptions())
		return err
	})
	if err != nil {
		return fmt.Errorf("lucy install many: %w", err)
	}
	return nil
}

func (s *InstallService) InstallPackages(ctx context.Context, requests []PackageRequest, targetDir string) ([]InstalledPackage, []FailedPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create target dir: %w", err)
	}
	if err := s.InstallMany(ctx, requests); err != nil {
		return nil, nil, err
	}
	installed := make([]InstalledPackage, 0, len(requests))
	failed := make([]FailedPackage, 0)
	for _, req := range requests {
		id := req.Platform + "/" + req.Name
		if req.Platform == "" {
			id = req.Name
		}
		path := filepath.Join(targetDir, req.Name+"-"+req.Version+".jar")
		info, err := os.Stat(path)
		if err != nil {
			failed = append(failed, FailedPackage{ID: id, Error: err.Error()})
			continue
		}
		installed = append(installed, InstalledPackage{
			ID:      id,
			Name:    req.Name,
			Version: req.Version,
			Path:    path,
			Size:    info.Size(),
		})
	}
	return installed, failed, nil
}

func (s *InstallService) withWorkDir(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.workDir == "" {
		return fn()
	}
	lucyInstallCwdMu.Lock()
	defer lucyInstallCwdMu.Unlock()
	original, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	if err := os.Chdir(s.workDir); err != nil {
		return fmt.Errorf("change lucy work dir: %w", err)
	}
	defer func() { _ = os.Chdir(original) }()
	return fn()
}

func PackageRequestsFromLocked(packages []LockedPackage) []PackageRequest {
	requests := make([]PackageRequest, 0, len(packages))
	for _, pkg := range packages {
		platform := packagePlatformFromID(pkg.ID)
		name := pkg.Name
		if name == "" {
			name = packageNameFromID(pkg.ID)
		}
		requests = append(requests, PackageRequest{
			Platform: platform,
			Name:     name,
			Scope:    pkg.Source,
			Version:  pkg.Version,
		})
	}
	return requests
}

// ProbeService provides operations for Lucy server probing.
type ProbeService struct {
	workDir string
}

// NewProbeService creates a ProbeService for the given work directory.
func NewProbeService(workDir string) *ProbeService {
	return &ProbeService{workDir: workDir}
}

// ServerInfo returns the current server environment information.
func (s *ProbeService) ServerInfo() (map[string]interface{}, error) {
	info := lucyworkspace.NewAt(s.workDir)
	ws := lucyworkspace.Workspace{
		Runtime:  info.Runtime,
		Topology: info.Topology,
	}
	result := make(map[string]interface{})
	result["game_version"] = string(info.Runtime.GameVersion)
	result["platform"] = string(ws.DerivedModLoader())
	result["platform_version"] = ws.DerivedLoaderVersion()
	if info.Environments.Mcdr != nil {
		result["mcdr"] = true
	}
	return result, nil
}

// Invalidate marks the cached server info as stale.
func (s *ProbeService) Invalidate() {
	lucyworkspace.Invalidate()
}

// LockIntegrityResult reports whether all locked packages are present and intact.
type LockIntegrityResult struct {
	OK      bool     `json:"ok"`
	Missing []string `json:"missing"`
	Corrupt []string `json:"corrupt"`
	Checked int      `json:"checked"`
	Errors  []string `json:"errors,omitempty"`
}

func (s *ProbeService) VerifyLockIntegrity(ctx context.Context, lockPath string, modsDir string) (LockIntegrityResult, error) {
	if err := ctx.Err(); err != nil {
		return LockIntegrityResult{}, err
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return LockIntegrityResult{}, fmt.Errorf("read lock: %w", err)
	}
	if _, err := fileSHA256(lockPath); err != nil {
		return LockIntegrityResult{}, fmt.Errorf("hash lock: %w", err)
	}
	var lock EnvironmentLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockIntegrityResult{}, fmt.Errorf("decode lock: %w", err)
	}
	result := LockIntegrityResult{
		Missing: []string{},
		Corrupt: []string{},
		Errors:  []string{},
	}
	for _, pkg := range lock.Packages {
		result.Checked++
		filename := pkg.Metadata["filename"]
		if filename == "" {
			filename = expectedPackageFilename(pkg)
		}
		path := filepath.Join(modsDir, filename)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				result.Missing = append(result.Missing, pkg.ID)
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", pkg.ID, err))
			continue
		}
		algorithm := pkg.Metadata["hash_algorithm"]
		actual, err := fileHashByAlgorithm(path, algorithm)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", pkg.ID, err))
			continue
		}
		if normalizeHash(pkg.Hash) != "" && actual != normalizeHash(pkg.Hash) {
			result.Corrupt = append(result.Corrupt, pkg.ID)
		}
	}
	result.OK = len(result.Missing) == 0 && len(result.Corrupt) == 0 && len(result.Errors) == 0
	return result, nil
}

func (s *ProbeService) VerifyIntegrityFromLock(ctx context.Context, lockPath, modsDir string) (IntegrityResult, error) {
	result, err := s.VerifyLockIntegrity(ctx, lockPath, modsDir)
	if err != nil {
		return IntegrityResult{}, err
	}
	status := "ok"
	if len(result.Missing) > 0 {
		status = "missing_files"
	} else if len(result.Corrupt) > 0 {
		status = "hash_mismatch"
	} else if len(result.Errors) > 0 {
		status = "error"
	}
	return IntegrityResult{
		OK:      result.OK,
		Status:  status,
		Missing: result.Missing,
		Corrupt: result.Corrupt,
		Checked: result.Checked,
		Errors:  result.Errors,
	}, nil
}
