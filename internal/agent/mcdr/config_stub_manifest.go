package mcdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent/process"
)

const configStubManifestName = "mcdr-config-stub.json"

type ConfigStubManifest struct {
	SessionID                   string            `json:"session_id"`
	EnvironmentID               string            `json:"environment_id,omitempty"`
	MinecraftVersion            string            `json:"minecraft_version,omitempty"`
	JavaVersion                 string            `json:"java_version,omitempty"`
	MCDRRoot                    string            `json:"mcdr_root"`
	ConfigDir                   string            `json:"config_dir"`
	PluginsDir                  string            `json:"plugins_dir"`
	ServerDir                   string            `json:"server_dir"`
	LogsDir                     string            `json:"logs_dir"`
	ServerWorkDir               string            `json:"server_work_dir"`
	PlannedConfigYMLPath        string            `json:"planned_config_yml_path"`
	PlannedPluginDirPath        string            `json:"planned_plugin_dir_path"`
	PlannedServerPropertiesPath string            `json:"planned_server_properties_path"`
	PlannedEULAPath             string            `json:"planned_eula_path"`
	Status                      ConfigStubStatus  `json:"status"`
	PlannedAt                   time.Time         `json:"planned_at"`
	Notes                       string            `json:"notes"`
	Metadata                    map[string]string `json:"metadata,omitempty"`
}

type ConfigStubInspectionResult struct {
	SessionID                   string
	Exists                      bool
	Path                        string
	Valid                       bool
	Status                      string
	PlannedConfigYMLPath        string
	PlannedServerPropertiesPath string
	PlannedEULAPath             string
	Issues                      []string
	CheckedAt                   time.Time
}

func InspectConfigStubManifest(layout process.MCDRRuntimeLayout) ConfigStubInspectionResult {
	result := ConfigStubInspectionResult{
		SessionID: layout.SessionLayout.SessionID,
		CheckedAt: time.Now().UTC(),
		Issues:    []string{},
	}
	manifestPath := filepath.Join(layout.MCDRRoot, configStubManifestName)
	result.Path = manifestPath
	if !filesystemPathWithin(layout.MCDRRoot, manifestPath) {
		result.Issues = append(result.Issues, "manifest path escapes MCDR root")
		return result
	}
	info, err := os.Lstat(manifestPath)
	if os.IsNotExist(err) {
		result.Status = "missing"
		return result
	}
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("inspect manifest: %v", err))
		return result
	}
	if !info.Mode().IsRegular() {
		result.Issues = append(result.Issues, "manifest is not a regular file")
		return result
	}
	result.Exists = true
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("read manifest: %v", err))
		return result
	}
	var manifest ConfigStubManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("parse JSON: %v", err))
		return result
	}
	if manifest.SessionID != layout.SessionLayout.SessionID {
		result.Issues = append(result.Issues, fmt.Sprintf("session mismatch: manifest=%s layout=%s", manifest.SessionID, layout.SessionLayout.SessionID))
	}
	if err := validateRuntimeRelativePath("mcdr_root", manifest.MCDRRoot); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("mcdr_root: %v", err))
	}
	if err := validateRuntimeRelativePath("config_dir", manifest.ConfigDir); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("config_dir: %v", err))
	}
	if err := validateRuntimeRelativePath("plugins_dir", manifest.PluginsDir); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("plugins_dir: %v", err))
	}
	if err := validateRuntimeRelativePath("server_dir", manifest.ServerDir); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("server_dir: %v", err))
	}
	if err := validateRuntimeRelativePath("planned_config_yml_path", manifest.PlannedConfigYMLPath); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("planned_config_yml_path: %v", err))
	}
	result.Valid = len(result.Issues) == 0
	result.Status = string(manifest.Status)
	result.PlannedConfigYMLPath = manifest.PlannedConfigYMLPath
	result.PlannedServerPropertiesPath = manifest.PlannedServerPropertiesPath
	result.PlannedEULAPath = manifest.PlannedEULAPath
	return result
}

// SerializeConfigStub validates and serializes a planning-only manifest.
func SerializeConfigStub(stub ConfigStub) ([]byte, error) {
	if err := stub.Validate(); err != nil {
		return nil, fmt.Errorf("validate MCDR config stub: %w", err)
	}
	manifest := ConfigStubManifest{
		SessionID:                   stub.SessionID,
		EnvironmentID:               stub.EnvironmentID,
		MinecraftVersion:            stub.MinecraftVersion,
		JavaVersion:                 stub.JavaVersion,
		MCDRRoot:                    stub.MCDRRoot,
		ConfigDir:                   stub.ConfigDir,
		PluginsDir:                  stub.PluginsDir,
		ServerDir:                   stub.ServerDir,
		LogsDir:                     stub.LogsDir,
		ServerWorkDir:               stub.ServerWorkDir,
		PlannedConfigYMLPath:        stub.ConfigFilePath,
		PlannedPluginDirPath:        stub.PluginDirPath,
		PlannedServerPropertiesPath: stub.ServerPropertiesPath,
		PlannedEULAPath:             stub.EULAPath,
		Status:                      ConfigStubStatusPlanned,
		PlannedAt:                   stub.PlannedAt,
		Notes:                       configStubNotes,
		Metadata:                    cloneMetadata(stub.Metadata),
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serialize MCDR config stub manifest: %w", err)
	}
	return append(payload, '\n'), nil
}

// WriteConfigStubManifest atomically writes planning metadata under the
// prepared MCDR root. It does not create the layout or any runtime config.
func WriteConfigStubManifest(layout process.MCDRRuntimeLayout, stub ConfigStub) (string, error) {
	if layout.SessionLayout.SessionID != stub.SessionID {
		return "", errors.New("MCDR config stub session does not match runtime layout")
	}
	if err := validateConfigStubLayout(layout, stub); err != nil {
		return "", err
	}
	manifestPath := filepath.Join(layout.MCDRRoot, configStubManifestName)
	if !filesystemPathWithin(layout.MCDRRoot, manifestPath) {
		return "", errors.New("MCDR config stub manifest path escapes MCDR root")
	}
	if err := validateManifestRoot(layout); err != nil {
		return "", err
	}
	if info, err := os.Lstat(manifestPath); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("MCDR config stub manifest path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect MCDR config stub manifest: %w", err)
	}
	payload, err := SerializeConfigStub(stub)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(layout.MCDRRoot, ".mcdr-config-stub-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary MCDR config stub manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("set MCDR config stub manifest permissions: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write MCDR config stub manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync MCDR config stub manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close MCDR config stub manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, manifestPath); err != nil {
		return "", fmt.Errorf("replace MCDR config stub manifest: %w", err)
	}
	return manifestPath, nil
}

func validateConfigStubLayout(layout process.MCDRRuntimeLayout, stub ConfigStub) error {
	if !filesystemPathWithin(layout.SessionLayout.SessionRoot, layout.MCDRRoot) {
		return errors.New("MCDR root escapes session runtime root")
	}
	relative, err := filepath.Rel(layout.SessionLayout.SessionRoot, layout.MCDRRoot)
	if err != nil {
		return fmt.Errorf("resolve MCDR root relative to session runtime root: %w", err)
	}
	if filepath.ToSlash(relative) != stub.MCDRRoot {
		return errors.New("MCDR config stub root does not match runtime layout")
	}
	return nil
}

func validateManifestRoot(layout process.MCDRRuntimeLayout) error {
	root := filepath.Clean(layout.SessionLayout.RuntimeRoot)
	relative, err := filepath.Rel(root, filepath.Clean(layout.MCDRRoot))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("MCDR root escapes runtime root")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect runtime root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime root must not be a symbolic link")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect MCDR runtime path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("MCDR runtime path contains a symbolic link")
		}
	}
	info, err := os.Stat(layout.MCDRRoot)
	if err != nil {
		return fmt.Errorf("inspect MCDR root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("MCDR root must be a prepared directory")
	}
	return nil
}

func filesystemPathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
