package mcdrbridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/stratummc/stratum/internal/agent/process"
)

// LaunchCommand describes how MCDR should be launched.
type LaunchCommand struct {
	Executable string   `json:"executable"`
	Argv       []string `json:"argv"`
	WorkingDir string   `json:"working_dir"`
	EnvKeys    []string `json:"env_keys,omitempty"`
}

// StopStrategy describes the MCDR stop mechanism.
type StopStrategy string

const (
	StopStrategyStdin  StopStrategy = "stdin"
	StopStrategySignal StopStrategy = "signal"
	StopStrategyNone   StopStrategy = "none"
)

// StopConfig describes how MCDR should be stopped.
type StopConfig struct {
	Strategy     StopStrategy `json:"strategy"`
	StdinCommand string       `json:"stdin_command,omitempty"`
}

// PreconditionStatus describes whether a launch precondition has been verified.
type PreconditionStatus string

const (
	PreconditionUnknown    PreconditionStatus = "unknown"
	PreconditionRequired   PreconditionStatus = "required"
	PreconditionNotChecked PreconditionStatus = "not_checked"
)

// Preconditions records checks that must be satisfied before launch.
type Preconditions struct {
	PythonAvailable  PreconditionStatus `json:"python_available"`
	MCDRInstalled    PreconditionStatus `json:"mcdr_installed"`
	ServerJarPresent PreconditionStatus `json:"server_jar_present"`
	EULAPresent      PreconditionStatus `json:"eula_present"`
}

// LaunchPlan is the full MCDR launch plan manifest persisted to disk.
type LaunchPlan struct {
	SessionID                   string            `json:"session_id"`
	EnvironmentID               string            `json:"environment_id"`
	MinecraftVersion            string            `json:"minecraft_version"`
	JavaVersion                 string            `json:"java_version"`
	LoaderType                  string            `json:"loader_type"`
	LoaderVersion               string            `json:"loader_version"`
	ServerCore                  string            `json:"server_core"`
	RuntimeProfileID            string            `json:"runtime_profile_id"`
	Status                      string            `json:"status"`
	MCDRRoot                    string            `json:"mcdr_root"`
	ConfigDir                   string            `json:"config_dir"`
	PluginsDir                  string            `json:"plugins_dir"`
	ServerDir                   string            `json:"server_dir"`
	LogsDir                     string            `json:"logs_dir"`
	ServerWorkDir               string            `json:"server_work_dir"`
	PlannedConfigPath           string            `json:"planned_config_path"`
	PlannedServerPropertiesPath string            `json:"planned_server_properties_path"`
	PlannedEULAPath             string            `json:"planned_eula_path"`
	Command                     LaunchCommand     `json:"command"`
	Stop                        StopConfig        `json:"stop"`
	Preconditions               Preconditions     `json:"preconditions"`
	Issues                      []string          `json:"issues,omitempty"`
	PlannedAt                   time.Time         `json:"planned_at"`
	Notes                       string            `json:"notes"`
	Metadata                    map[string]string `json:"metadata,omitempty"`
}

// LaunchPlanStatus is an inspection result for a persisted launch plan.
type LaunchPlanStatus struct {
	SessionID string
	Exists    bool
	Valid     bool
	Status    string
	Path      string
	Issues    []string
	CheckedAt time.Time
	Summary   string
}

func writeLaunchPlanManifest(layout process.MCDRRuntimeLayout, plan LaunchPlan) (string, error) {
	if layout.SessionLayout.SessionID != plan.SessionID {
		return "", fmt.Errorf("MCDR launch plan session does not match runtime layout")
	}

	if !pathWithin(layout.SessionLayout.RuntimeRoot, layout.MCDRRoot) {
		return "", fmt.Errorf("MCDR root escapes runtime root")
	}

	manifestPath := filepath.Join(layout.MCDRRoot, launchPlanManifestName)
	if !pathWithin(layout.MCDRRoot, manifestPath) {
		return "", fmt.Errorf("launch-plan manifest path escapes MCDR root")
	}

	if info, err := os.Lstat(layout.MCDRRoot); err != nil {
		return "", fmt.Errorf("inspect MCDR root: %w", err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("MCDR root must be a prepared directory")
	}

	if info, err := os.Lstat(manifestPath); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("launch-plan manifest path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect launch-plan manifest: %w", err)
	}

	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialize launch plan: %w", err)
	}
	payload = append(payload, '\n')

	tmp, err := os.CreateTemp(layout.MCDRRoot, ".mcdr-launch-plan-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary launch-plan manifest: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("set launch-plan manifest permissions: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write launch-plan manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync launch-plan manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close launch-plan manifest: %w", err)
	}

	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return "", fmt.Errorf("replace launch-plan manifest: %w", err)
	}

	return manifestPath, nil
}

func inspectLaunchPlanManifest(layout process.MCDRRuntimeLayout) LaunchPlanStatus {
	result := LaunchPlanStatus{
		SessionID: layout.SessionLayout.SessionID,
		CheckedAt: time.Now().UTC(),
		Issues:    []string{},
	}

	manifestPath := filepath.Join(layout.MCDRRoot, launchPlanManifestName)
	result.Path = manifestPath

	if !pathWithin(layout.MCDRRoot, manifestPath) {
		result.Issues = append(result.Issues, "launch-plan manifest path escapes MCDR root")
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
		result.Issues = append(result.Issues, "launch-plan manifest is not a regular file")
		return result
	}

	result.Exists = true

	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("read manifest: %v", err))
		return result
	}

	var plan LaunchPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("parse JSON: %v", err))
		return result
	}

	if plan.SessionID != layout.SessionLayout.SessionID {
		result.Issues = append(result.Issues, fmt.Sprintf("session mismatch: manifest=%s layout=%s", plan.SessionID, layout.SessionLayout.SessionID))
	}

	if err := validateRuntimeRelativePath("mcdr_root", plan.MCDRRoot); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("mcdr_root: %v", err))
	}

	result.Valid = len(result.Issues) == 0
	result.Status = plan.Status
	result.Summary = fmt.Sprintf("MCDR launch plan %s (status=%s, valid=%v)", plan.SessionID, plan.Status, result.Valid)
	return result
}
