package lucy

import (
	"context"
	"fmt"

	lucyinstall "github.com/mclucy/lucy/install"
	lucyprobe "github.com/mclucy/lucy/probe"
	lucytypes "github.com/mclucy/lucy/types"
)

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
	lucyReq := lucyinstall.PackageRequest{
		ScopedPackageRef: lucytypes.ScopedPackageRef{
			PackageRef: lucytypes.PackageRef{
				Platform: lucytypes.PlatformId(req.Platform),
				Name:     lucytypes.BarePackageName(req.Name),
			},
			Scope: lucytypes.ParseSource(req.Scope),
		},
		Version: lucytypes.BareVersion(req.Version),
	}
	_, err := lucyinstall.Install(lucyReq, lucyinstall.DefaultOptions())
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
	lucyReqs := make([]lucyinstall.PackageRequest, len(requests))
	for i, req := range requests {
		lucyReqs[i] = lucyinstall.PackageRequest{
			ScopedPackageRef: lucytypes.ScopedPackageRef{
				PackageRef: lucytypes.PackageRef{
					Platform: lucytypes.PlatformId(req.Platform),
					Name:     lucytypes.BarePackageName(req.Name),
				},
				Scope: lucytypes.ParseSource(req.Scope),
			},
			Version: lucytypes.BareVersion(req.Version),
		}
	}
	_, err := lucyinstall.InstallMany(lucyReqs, lucyinstall.DefaultOptions())
	if err != nil {
		return fmt.Errorf("lucy install many: %w", err)
	}
	return nil
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
	info := lucyprobe.ServerInfoAt(s.workDir)
	result := make(map[string]interface{})
	result["game_version"] = string(info.Runtime.GameVersion)
	result["platform"] = string(info.Runtime.DerivedModLoader())
	result["platform_version"] = info.Runtime.DerivedLoaderVersion()
	if info.Environments.Mcdr != nil {
		result["mcdr"] = true
	}
	return result, nil
}

// Invalidate marks the cached server info as stale.
func (s *ProbeService) Invalidate() {
	lucyprobe.InvalidateServerInfo()
}
