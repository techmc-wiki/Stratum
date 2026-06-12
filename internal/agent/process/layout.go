package process

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeDirectoryPermissions = 0o750

type SessionRuntimeLayout struct {
	RuntimeRoot    string
	SessionID      string
	SessionRoot    string
	WorkDir        string
	LogsDir        string
	ConfigDir      string
	ArtifactsDir   string
	CheckpointsDir string
	TmpDir         string
}

func NewSessionRuntimeLayout(runtimeRoot, sessionID string) (SessionRuntimeLayout, error) {
	root, err := filepath.Abs(strings.TrimSpace(runtimeRoot))
	if err != nil {
		return SessionRuntimeLayout{}, fmt.Errorf("resolve runtime root: %w", err)
	}
	root = filepath.Clean(root)
	if err := validateSessionRuntimeID(sessionID); err != nil {
		return SessionRuntimeLayout{}, err
	}
	sessionRoot := filepath.Join(root, "sessions", sessionID)
	layout := SessionRuntimeLayout{
		RuntimeRoot:    root,
		SessionID:      sessionID,
		SessionRoot:    sessionRoot,
		WorkDir:        filepath.Join(sessionRoot, "work"),
		LogsDir:        filepath.Join(sessionRoot, "logs"),
		ConfigDir:      filepath.Join(sessionRoot, "config"),
		ArtifactsDir:   filepath.Join(sessionRoot, "artifacts"),
		CheckpointsDir: filepath.Join(sessionRoot, "checkpoints"),
		TmpDir:         filepath.Join(sessionRoot, "tmp"),
	}
	for _, path := range []string{layout.SessionRoot, layout.WorkDir, layout.LogsDir, layout.ConfigDir, layout.ArtifactsDir, layout.CheckpointsDir, layout.TmpDir} {
		if !pathWithin(root, path) {
			return SessionRuntimeLayout{}, fmt.Errorf("session runtime path %q escapes runtime root", path)
		}
	}
	return layout, nil
}

func (l SessionRuntimeLayout) Create() error {
	for _, path := range []string{l.SessionRoot, l.WorkDir, l.LogsDir, l.ConfigDir, l.ArtifactsDir, l.CheckpointsDir, l.TmpDir} {
		if err := os.MkdirAll(path, runtimeDirectoryPermissions); err != nil {
			return fmt.Errorf("create session runtime directory %q: %w", path, err)
		}
	}
	return nil
}

func validateSessionRuntimeID(id string) error {
	if id == "" || id == "." || id == ".." {
		return errors.New("session runtime id is required")
	}
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("session runtime id %q contains unsupported characters", id)
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
