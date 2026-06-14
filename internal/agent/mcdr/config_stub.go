package mcdr

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/process"
)

type ConfigStubStatus string

const (
	ConfigStubStatusPlanned ConfigStubStatus = "planned"
	ConfigStubStatusStub    ConfigStubStatus = "stub"
)

const configStubNotes = "Stratum planning manifest only; this is not MCDR config.yml or Minecraft server.properties, it does not install MCDR, and no MCDR or Minecraft process was started."

// ConfigStub describes intended MCDR configuration paths without writing files
// or starting a runtime.
type ConfigStub struct {
	SessionID            string            `json:"session_id"`
	EnvironmentID        string            `json:"environment_id"`
	MinecraftVersion     string            `json:"minecraft_version"`
	JavaVersion          string            `json:"java_version"`
	MCDRRoot             string            `json:"mcdr_root"`
	ConfigDir            string            `json:"config_dir"`
	PluginsDir           string            `json:"plugins_dir"`
	ServerDir            string            `json:"server_dir"`
	LogsDir              string            `json:"logs_dir"`
	ServerWorkDir        string            `json:"server_work_dir"`
	ConfigFilePath       string            `json:"config_file_path"`
	PluginDirPath        string            `json:"plugin_dir_path"`
	ServerPropertiesPath string            `json:"server_properties_path"`
	EULAPath             string            `json:"eula_path"`
	Notes                string            `json:"notes"`
	Status               ConfigStubStatus  `json:"status"`
	PlannedAt            time.Time         `json:"planned_at"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

// NewConfigStub builds metadata from an existing runtime layout and Environment
// materialization result. It performs no filesystem or process operations.
func NewConfigStub(layout process.MCDRRuntimeLayout, materialization agent.EnvironmentMaterializationResult, plannedAt time.Time) (ConfigStub, error) {
	if layout.SessionLayout.SessionID != materialization.SessionID {
		return ConfigStub{}, errors.New("MCDR config stub session does not match Environment materialization")
	}

	relative := func(field, value string) (string, error) {
		rel, err := filepath.Rel(layout.SessionLayout.SessionRoot, value)
		if err != nil {
			return "", fmt.Errorf("resolve %s relative to session root: %w", field, err)
		}
		rel = filepath.ToSlash(rel)
		if err := validateRuntimeRelativePath(field, rel); err != nil {
			return "", err
		}
		return rel, nil
	}

	mcdrRoot, err := relative("mcdr_root", layout.MCDRRoot)
	if err != nil {
		return ConfigStub{}, err
	}
	configDir, err := relative("config_dir", layout.MCDRConfigDir)
	if err != nil {
		return ConfigStub{}, err
	}
	pluginsDir, err := relative("plugins_dir", layout.MCDRPluginsDir)
	if err != nil {
		return ConfigStub{}, err
	}
	serverDir, err := relative("server_dir", layout.MCDRServerDir)
	if err != nil {
		return ConfigStub{}, err
	}
	logsDir, err := relative("logs_dir", layout.MCDRLogsDir)
	if err != nil {
		return ConfigStub{}, err
	}

	stub := ConfigStub{
		SessionID:            materialization.SessionID,
		EnvironmentID:        materialization.EnvironmentID,
		MinecraftVersion:     materialization.MinecraftVersion,
		JavaVersion:          materialization.JavaVersion,
		MCDRRoot:             mcdrRoot,
		ConfigDir:            configDir,
		PluginsDir:           pluginsDir,
		ServerDir:            serverDir,
		LogsDir:              logsDir,
		ServerWorkDir:        serverDir,
		ConfigFilePath:       path.Join(configDir, "config.yml"),
		PluginDirPath:        pluginsDir,
		ServerPropertiesPath: path.Join(serverDir, "server.properties"),
		EULAPath:             path.Join(serverDir, "eula.txt"),
		Notes:                configStubNotes,
		Status:               ConfigStubStatusPlanned,
		PlannedAt:            plannedAt.UTC(),
		Metadata:             cloneMetadata(materialization.Metadata),
	}
	if err := stub.Validate(); err != nil {
		return ConfigStub{}, err
	}
	return stub, nil
}

func (s ConfigStub) Validate() error {
	if err := validateSafeID("session", s.SessionID); err != nil {
		return err
	}
	if err := validateSafeID("environment", s.EnvironmentID); err != nil {
		return err
	}
	if strings.TrimSpace(s.MinecraftVersion) == "" {
		return errors.New("minecraft version is required")
	}
	if s.Status != ConfigStubStatusPlanned && s.Status != ConfigStubStatusStub {
		return fmt.Errorf("unsupported MCDR config stub status %q", s.Status)
	}

	paths := []struct {
		name  string
		value string
	}{
		{"mcdr_root", s.MCDRRoot},
		{"config_dir", s.ConfigDir},
		{"plugins_dir", s.PluginsDir},
		{"server_dir", s.ServerDir},
		{"logs_dir", s.LogsDir},
		{"server_work_dir", s.ServerWorkDir},
		{"config_file_path", s.ConfigFilePath},
		{"plugin_dir_path", s.PluginDirPath},
		{"server_properties_path", s.ServerPropertiesPath},
		{"eula_path", s.EULAPath},
	}
	for _, candidate := range paths {
		if err := validateRuntimeRelativePath(candidate.name, candidate.value); err != nil {
			return err
		}
	}

	expected := map[string]string{
		"config_dir":             path.Join(s.MCDRRoot, "config"),
		"plugins_dir":            path.Join(s.MCDRRoot, "plugins"),
		"server_dir":             path.Join(s.MCDRRoot, "server"),
		"logs_dir":               path.Join(s.MCDRRoot, "logs"),
		"server_work_dir":        s.ServerDir,
		"config_file_path":       path.Join(s.ConfigDir, "config.yml"),
		"plugin_dir_path":        s.PluginsDir,
		"server_properties_path": path.Join(s.ServerWorkDir, "server.properties"),
		"eula_path":              path.Join(s.ServerWorkDir, "eula.txt"),
	}
	actual := map[string]string{
		"config_dir":             s.ConfigDir,
		"plugins_dir":            s.PluginsDir,
		"server_dir":             s.ServerDir,
		"logs_dir":               s.LogsDir,
		"server_work_dir":        s.ServerWorkDir,
		"config_file_path":       s.ConfigFilePath,
		"plugin_dir_path":        s.PluginDirPath,
		"server_properties_path": s.ServerPropertiesPath,
		"eula_path":              s.EULAPath,
	}
	for field, expectedPath := range expected {
		if actual[field] != expectedPath {
			return fmt.Errorf("%s must be %q", field, expectedPath)
		}
		if !relativePathWithin(s.MCDRRoot, actual[field]) {
			return fmt.Errorf("%s must remain under mcdr_root", field)
		}
	}
	return nil
}

func validateSafeID(kind, value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("%s id is required", kind)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("%s id %q contains unsupported characters", kind, value)
	}
	return nil
}

func validateRuntimeRelativePath(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("%s must use forward-slash runtime-relative paths", field)
	}
	if filepath.IsAbs(value) || path.IsAbs(value) || hasWindowsVolumePrefix(value) {
		return fmt.Errorf("%s must be runtime-relative", field)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s contains an unsafe or non-canonical path", field)
	}
	return nil
}

func relativePathWithin(root, candidate string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
