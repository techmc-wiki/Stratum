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

type MCDRRuntimeLayout struct {
	SessionLayout    SessionRuntimeLayout
	MCDRRoot         string
	MCDRConfigDir    string
	MCDRPluginsDir   string
	MCDRServerDir    string
	MCDRLogsDir      string
	MCDRTmpDir       string
	MCDRManifestPath string
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

func (l SessionRuntimeLayout) MCDR() (MCDRRuntimeLayout, error) {
	mcdrRoot := filepath.Join(l.WorkDir, "mcdr")
	layout := MCDRRuntimeLayout{
		SessionLayout:    l,
		MCDRRoot:         mcdrRoot,
		MCDRConfigDir:    filepath.Join(mcdrRoot, "config"),
		MCDRPluginsDir:   filepath.Join(mcdrRoot, "plugins"),
		MCDRServerDir:    filepath.Join(mcdrRoot, "server"),
		MCDRLogsDir:      filepath.Join(mcdrRoot, "logs"),
		MCDRTmpDir:       filepath.Join(mcdrRoot, "tmp"),
		MCDRManifestPath: filepath.Join(mcdrRoot, "mcdr-layout.json"),
	}
	for _, path := range []string{layout.MCDRRoot, layout.MCDRConfigDir, layout.MCDRPluginsDir, layout.MCDRServerDir, layout.MCDRLogsDir, layout.MCDRTmpDir} {
		if !pathWithin(l.RuntimeRoot, path) {
			return MCDRRuntimeLayout{}, fmt.Errorf("MCDR runtime path %q escapes runtime root", path)
		}
	}
	return layout, nil
}

func (m MCDRRuntimeLayout) Create() error {
	for _, path := range []string{m.MCDRRoot, m.MCDRConfigDir, m.MCDRPluginsDir, m.MCDRServerDir, m.MCDRLogsDir, m.MCDRTmpDir} {
		if err := os.MkdirAll(path, runtimeDirectoryPermissions); err != nil {
			return fmt.Errorf("create MCDR runtime directory %q: %w", path, err)
		}
	}
	return nil
}

type MCDRLayoutManifest struct {
	SessionID  string    `json:"session_id"`
	CreatedAt  time.Time `json:"created_at"`
	MCDRRoot   string    `json:"mcdr_root"`
	ConfigDir  string    `json:"config_dir"`
	PluginsDir string    `json:"plugins_dir"`
	ServerDir  string    `json:"server_dir"`
	LogsDir    string    `json:"logs_dir"`
	TmpDir     string    `json:"tmp_dir"`
	Status     string    `json:"status"`
	Notes      string    `json:"notes"`
}

func (m MCDRRuntimeLayout) WriteManifest() error {
	manifest := MCDRLayoutManifest{
		SessionID:  m.SessionLayout.SessionID,
		CreatedAt:  time.Now().UTC(),
		MCDRRoot:   m.MCDRRoot,
		ConfigDir:  m.MCDRConfigDir,
		PluginsDir: m.MCDRPluginsDir,
		ServerDir:  m.MCDRServerDir,
		LogsDir:    m.MCDRLogsDir,
		TmpDir:     m.MCDRTmpDir,
		Status:     "prepared",
		Notes:      "MCDR runtime directories are prepared only; MCDR and Minecraft are not started.",
	}
	tmp, err := os.CreateTemp(m.MCDRRoot, ".mcdr-layout-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	if err := os.Rename(tmpPath, m.MCDRManifestPath); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
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
