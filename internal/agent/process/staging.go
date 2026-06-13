package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const stagedArtifactManifestName = "staged-artifacts.json"
const stagedConfigManifestName = "staged-config.json"

type SessionRuntimeStaging struct {
	SessionID               string
	ArtifactsDir            string
	ConfigDir               string
	TmpDir                  string
	StagedArtifactsManifest string
	StagedConfigManifest    string
}

type StagedRuntimeItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Kind          string    `json:"kind"`
	ArtifactID    string    `json:"artifactId,omitempty"`
	StagingPlanID string    `json:"stagingPlanId,omitempty"`
	PayloadHash   string    `json:"payloadHash,omitempty"`
	PayloadSize   int64     `json:"payloadSize,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type StagingManifest struct {
	SessionID string              `json:"sessionId"`
	Kind      string              `json:"kind"`
	Items     []StagedRuntimeItem `json:"items"`
	WrittenAt time.Time           `json:"writtenAt"`
}

func (l SessionRuntimeLayout) Staging() SessionRuntimeStaging {
	return SessionRuntimeStaging{
		SessionID:               l.SessionID,
		ArtifactsDir:            l.ArtifactsDir,
		ConfigDir:               l.ConfigDir,
		TmpDir:                  l.TmpDir,
		StagedArtifactsManifest: filepath.Join(l.ArtifactsDir, stagedArtifactManifestName),
		StagedConfigManifest:    filepath.Join(l.ConfigDir, stagedConfigManifestName),
	}
}

func (s SessionRuntimeStaging) ArtifactPath(name string) (string, error) {
	return stagingPath(s.ArtifactsDir, name)
}

func (s SessionRuntimeStaging) ConfigPath(name string) (string, error) {
	return stagingPath(s.ConfigDir, name)
}

func (s SessionRuntimeStaging) ArtifactEntry(artifactID, name string, at time.Time) (StagedRuntimeItem, error) {
	path, err := s.ArtifactPath(name)
	if err != nil {
		return StagedRuntimeItem{}, err
	}
	return stagedItem("artifact", artifactID, name, path, at)
}

func (s SessionRuntimeStaging) ConfigEntry(configID, name string, at time.Time) (StagedRuntimeItem, error) {
	path, err := s.ConfigPath(name)
	if err != nil {
		return StagedRuntimeItem{}, err
	}
	return stagedItem("config", configID, name, path, at)
}

func (s SessionRuntimeStaging) WriteArtifactManifest(items []StagedRuntimeItem, at time.Time) error {
	return writeStagingManifestAtomic(s.StagedArtifactsManifest, StagingManifest{SessionID: s.SessionID, Kind: "artifacts", Items: items, WrittenAt: normalizeManifestTime(at)})
}

func (s SessionRuntimeStaging) ReadArtifactManifest() (StagingManifest, error) {
	return readStagingManifest(s.StagedArtifactsManifest, s.SessionID, "artifacts")
}

func (s SessionRuntimeStaging) WriteConfigManifest(items []StagedRuntimeItem, at time.Time) error {
	return writeStagingManifestAtomic(s.StagedConfigManifest, StagingManifest{SessionID: s.SessionID, Kind: "config", Items: items, WrittenAt: normalizeManifestTime(at)})
}

func stagingPath(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("staging root is required")
	}
	if err := validateStagingName(name); err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	path := filepath.Join(root, filepath.Clean(name))
	if !pathWithin(root, path) {
		return "", fmt.Errorf("staging path %q escapes staging root", name)
	}
	return path, nil
}

func validateStagingName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("staging name is required")
	}
	if filepath.IsAbs(name) {
		return errors.New("staging name must be relative")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("staging name escapes staging root")
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' || character == '/' || character == '\\' {
			continue
		}
		return fmt.Errorf("staging name %q contains unsupported characters", name)
	}
	return nil
}

func stagedItem(kind, id, name, path string, at time.Time) (StagedRuntimeItem, error) {
	if strings.TrimSpace(id) == "" {
		return StagedRuntimeItem{}, errors.New("staged item id is required")
	}
	return StagedRuntimeItem{ID: id, Name: name, Path: path, Kind: kind, CreatedAt: normalizeManifestTime(at)}, nil
}

func writeStagingManifestAtomic(path string, value StagingManifest) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, runtimeDirectoryPermissions); err != nil {
		return fmt.Errorf("create staging manifest directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".stratum-staging-*.tmp")
	if err != nil {
		return fmt.Errorf("create staging manifest temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode staging manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync staging manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close staging manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace staging manifest: %w", err)
	}
	return nil
}

func readStagingManifest(path, sessionID, kind string) (StagingManifest, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return StagingManifest{SessionID: sessionID, Kind: kind}, nil
	}
	if err != nil {
		return StagingManifest{}, fmt.Errorf("open staging manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest StagingManifest
	if err := decoder.Decode(&manifest); err != nil {
		return StagingManifest{}, fmt.Errorf("decode staging manifest: %w", err)
	}
	if manifest.SessionID != sessionID || manifest.Kind != kind {
		return StagingManifest{}, errors.New("staging manifest identity does not match runtime layout")
	}
	return manifest, nil
}

func normalizeManifestTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}
	return at.UTC()
}
