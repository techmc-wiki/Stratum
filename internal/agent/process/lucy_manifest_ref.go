package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stratummc/stratum/internal/integration/lucy"
	"github.com/stratummc/stratum/internal/safepath"

	lucystate "github.com/mclucy/lucy/state"
	"gopkg.in/yaml.v3"
)

type referencedLucyManifest struct {
	manifest *lucystate.Manifest
	packages []lucy.PackageRef
	source   string
}

type selectedLucyManifest struct {
	manifest *lucystate.Manifest
	packages []lucy.PackageRef
	metadata map[string]string
}

func selectLucyManifest(ctx context.Context, requestManifestRef string, requestPackages []lucy.PackageRef, defaultManifest *lucystate.Manifest) (selectedLucyManifest, error) {
	packages := append([]lucy.PackageRef(nil), requestPackages...)
	metadata := map[string]string{"lucyManifestSource": "generated-default"}
	if strings.TrimSpace(requestManifestRef) == "" {
		return selectedLucyManifest{manifest: defaultManifest, packages: packages, metadata: metadata}, nil
	}
	referenced, err := loadReferencedLucyManifest(ctx, ".", requestManifestRef)
	if err != nil {
		return selectedLucyManifest{}, err
	}
	packages = append(packages, referenced.packages...)
	metadata["lucyManifestSource"] = "environment-ref"
	metadata["lucyManifestRefResolved"] = referenced.source
	metadata["lucyManifestPackageCount"] = fmt.Sprintf("%d", len(referenced.packages))
	return selectedLucyManifest{manifest: referenced.manifest, packages: packages, metadata: metadata}, nil
}

func loadReferencedLucyManifest(ctx context.Context, workspaceRoot, manifestRef string) (referencedLucyManifest, error) {
	if err := ctx.Err(); err != nil {
		return referencedLucyManifest{}, err
	}
	absRoot, err := findWorkspaceRoot(workspaceRoot)
	if err != nil {
		return referencedLucyManifest{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	cleanRef, err := cleanLucyManifestRef(manifestRef)
	if err != nil {
		return referencedLucyManifest{}, err
	}
	manifestPath := filepath.Join(absRoot, filepath.FromSlash(cleanRef))
	if !safepath.Within(absRoot, manifestPath) {
		return referencedLucyManifest{}, fmt.Errorf("Lucy manifest ref %q escapes workspace root", manifestRef)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return referencedLucyManifest{}, fmt.Errorf("read referenced Lucy manifest %q: %w", manifestRef, err)
	}
	var manifest lucystate.Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return referencedLucyManifest{}, fmt.Errorf("parse referenced Lucy manifest %q: %w", manifestRef, err)
	}
	normalizeReferencedLucyManifest(&manifest)
	if err := lucystate.ValidateManifest(manifest); err != nil {
		return referencedLucyManifest{}, fmt.Errorf("validate referenced Lucy manifest %q: %w", manifestRef, err)
	}
	return referencedLucyManifest{manifest: &manifest, packages: packageRefsFromManifest(&manifest), source: cleanRef}, nil
}

func normalizeReferencedLucyManifest(manifest *lucystate.Manifest) {
	if manifest == nil {
		return
	}
	if manifest.FormatVersion == "1" {
		manifest.FormatVersion = lucystate.SupportedVersion
	}
}

func findWorkspaceRoot(start string) (string, error) {
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	current := absStart
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absStart, nil
		}
		current = parent
	}
}

func cleanLucyManifestRef(manifestRef string) (string, error) {
	trimmed := strings.TrimSpace(filepath.ToSlash(manifestRef))
	if trimmed == "" {
		return "", errors.New("Lucy manifest ref is empty")
	}
	if strings.Contains(trimmed, "\x00") {
		return "", errors.New("Lucy manifest ref contains NUL byte")
	}
	if strings.HasPrefix(trimmed, "/") || filepath.IsAbs(trimmed) || strings.Contains(trimmed, ":") {
		return "", fmt.Errorf("Lucy manifest ref %q must be workspace-relative", manifestRef)
	}
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("Lucy manifest ref %q escapes workspace root", manifestRef)
	}
	return cleaned, nil
}

func writeLucyManifest(ctx context.Context, configDir string, manifest *lucystate.Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create Lucy config directory: %w", err)
	}
	if err := lucy.NewManifestService(configDir).Write(ctx, manifest); err != nil {
		return err
	}
	return nil
}

func packageRefsFromManifest(manifest *lucystate.Manifest) []lucy.PackageRef {
	if manifest == nil || len(manifest.Packages) == 0 {
		return []lucy.PackageRef{}
	}
	packages := make([]lucy.PackageRef, 0, len(manifest.Packages))
	for _, pkg := range manifest.Packages {
		name := manifestPackageName(pkg.ID)
		packages = append(packages, lucy.PackageRef{
			ID:                name,
			Source:            pkg.Source,
			Name:              name,
			VersionConstraint: pkg.Version,
			Loader:            manifestPackageLoader(pkg.ID),
			Required:          manifestPackageRequired(pkg),
			Metadata: map[string]string{
				"role": string(pkg.Role),
				"side": string(pkg.Side),
			},
		})
	}
	return packages
}

func manifestPackageName(id string) string {
	if index := strings.LastIndex(id, "/"); index >= 0 && index+1 < len(id) {
		return id[index+1:]
	}
	return id
}

func manifestPackageLoader(id string) string {
	if index := strings.Index(id, "/"); index > 0 {
		return id[:index]
	}
	return ""
}

func manifestPackageRequired(pkg lucystate.ManifestPackage) bool {
	if pkg.Optional {
		return false
	}
	return pkg.Role == lucystate.RoleRequired || pkg.Role == ""
}
