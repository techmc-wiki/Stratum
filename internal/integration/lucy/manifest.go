package lucy

import (
	"context"
	"fmt"

	lucystate "github.com/mclucy/lucy/state"
)

// ManifestService provides operations for Lucy manifest management.
type ManifestService struct {
	workDir string
}

// NewManifestService creates a ManifestService for the given work directory.
func NewManifestService(workDir string) *ManifestService {
	return &ManifestService{workDir: workDir}
}

// Read reads the Lucy manifest from the work directory.
func (s *ManifestService) Read(ctx context.Context) (*lucystate.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	svc := lucystate.NewProjectStateService(s.workDir)
	if err := svc.Load(ctx); err != nil {
		return nil, fmt.Errorf("load lucy state: %w", err)
	}
	return svc.Manifest(), nil
}

// Write writes a Lucy manifest to the work directory.
func (s *ManifestService) Write(ctx context.Context, manifest *lucystate.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	svc := lucystate.NewProjectStateService(s.workDir)
	if err := svc.Save(ctx, manifest, nil); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	return nil
}

// CreateDefault creates a default manifest for the given environment parameters.
func CreateDefault(gameVersion, platform, platformVersion string, mcdr bool) *lucystate.Manifest {
	manifest := lucystate.ManifestDefaults()
	manifest.Environment.GameVersion = gameVersion
	manifest.Environment.ModdingPlatform = platform
	manifest.Environment.ModdingPlatformVersion = platformVersion
	manifest.Environment.Mcdr = mcdr
	return &manifest
}
