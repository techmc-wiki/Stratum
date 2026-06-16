package mcdrbridge

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/stratummc/stratum/internal/agent/process"
)

const launchPlanManifestName = "mcdr-launch-plan.json"
const launchPlanNotes = "Stratum MCDR launch plan; planning only, no MCDR or Minecraft process was started."

// Status constants for LaunchPlan.
const (
	StatusPlanned       = "planned"
	StatusMissingLayout = "missing_layout"
	StatusInvalid       = "invalid"
	StatusUnsupported   = "unsupported"
)

// ValidStatuses lists every recognised LaunchPlan status.
var ValidStatuses = []string{StatusPlanned, StatusMissingLayout, StatusInvalid, StatusUnsupported}

// MCDRBridge defines the planning contract for an MCDR child runtime.
// It builds, validates, and inspects launch plans without executing MCDR
// or Minecraft.
type MCDRBridge interface {
	BuildLaunchPlan(ctx context.Context, req BuildLaunchPlanRequest) (*LaunchPlan, error)
	ValidateLaunchPlan(ctx context.Context, plan LaunchPlan) error
	InspectLaunchPlan(ctx context.Context, sessionID string) (LaunchPlanStatus, error)
}

// BuildLaunchPlanRequest carries the information needed to construct
// a MCDR launch plan.
type BuildLaunchPlanRequest struct {
	SessionID        string
	RuntimeRoot      string
	SessionRoot      string
	WorkDir          string
	ConfigDir        string
	LogsDir          string
	EnvironmentID    string
	MinecraftVersion string
	JavaVersion      string
	LoaderType       string
	LoaderVersion    string
	ServerCore       string
	RuntimeProfileID string
	MCDRRequired     bool
	CarpetRequired   bool
	Metadata         map[string]string
}

// defaultBridge implements MCDRBridge backed by the process layout.
type defaultBridge struct {
	runtimeRoot string
}

// NewBridge returns an MCDRBridge that stores launch plans under the
// session-local runtime root.
func NewBridge(runtimeRoot string) MCDRBridge {
	return &defaultBridge{runtimeRoot: runtimeRoot}
}

// BuildLaunchPlan derives session and MCDR runtime layouts from the
// bridge's runtime root and request SessionID, constructs a LaunchPlan
// from the request, validates it, ensures MCDR directories exist, and
// writes the launch-plan manifest.
func (b *defaultBridge) BuildLaunchPlan(ctx context.Context, req BuildLaunchPlanRequest) (*LaunchPlan, error) {
	sessionLayout, err := process.NewSessionRuntimeLayout(b.runtimeRoot, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("build session layout: %w", err)
	}

	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		return nil, fmt.Errorf("build MCDR layout: %w", err)
	}

	plan := buildLaunchPlan(req, mcdrLayout)

	if err := b.ValidateLaunchPlan(ctx, *plan); err != nil {
		return nil, fmt.Errorf("validate launch plan: %w", err)
	}

	if err := mcdrLayout.Create(); err != nil {
		return nil, fmt.Errorf("create MCDR directories: %w", err)
	}

	if _, err := writeLaunchPlanManifest(mcdrLayout, *plan); err != nil {
		return nil, fmt.Errorf("write launch-plan manifest: %w", err)
	}

	return plan, nil
}

func buildLaunchPlan(req BuildLaunchPlanRequest, mcdrLayout process.MCDRRuntimeLayout) *LaunchPlan {
	mcdrRoot := filepath.ToSlash(mustRelative(mcdrLayout.SessionLayout.SessionRoot, mcdrLayout.MCDRRoot))
	configDir := filepath.ToSlash(mustRelative(mcdrLayout.SessionLayout.SessionRoot, mcdrLayout.MCDRConfigDir))
	pluginsDir := filepath.ToSlash(mustRelative(mcdrLayout.SessionLayout.SessionRoot, mcdrLayout.MCDRPluginsDir))
	serverDir := filepath.ToSlash(mustRelative(mcdrLayout.SessionLayout.SessionRoot, mcdrLayout.MCDRServerDir))
	logsDir := filepath.ToSlash(mustRelative(mcdrLayout.SessionLayout.SessionRoot, mcdrLayout.MCDRLogsDir))

	return &LaunchPlan{
		SessionID:                   req.SessionID,
		EnvironmentID:               req.EnvironmentID,
		MinecraftVersion:            req.MinecraftVersion,
		JavaVersion:                 req.JavaVersion,
		LoaderType:                  req.LoaderType,
		LoaderVersion:               req.LoaderVersion,
		ServerCore:                  req.ServerCore,
		RuntimeProfileID:            req.RuntimeProfileID,
		Status:                      StatusPlanned,
		MCDRRoot:                    mcdrRoot,
		ConfigDir:                   configDir,
		PluginsDir:                  pluginsDir,
		ServerDir:                   serverDir,
		LogsDir:                     logsDir,
		ServerWorkDir:               serverDir,
		PlannedConfigPath:           configDir + "/config.yml",
		PlannedServerPropertiesPath: serverDir + "/server.properties",
		PlannedEULAPath:             serverDir + "/eula.txt",
		Command: LaunchCommand{
			Executable: "python",
			Argv:       []string{"-m", "mcdreforged"},
			WorkingDir: mcdrRoot,
			EnvKeys:    []string{},
		},
		Stop: StopConfig{
			Strategy: StopStrategyStdin,
		},
		Preconditions: Preconditions{
			PythonAvailable:  PreconditionNotChecked,
			MCDRInstalled:    PreconditionNotChecked,
			ServerJarPresent: PreconditionNotChecked,
			EULAPresent:      PreconditionNotChecked,
		},
		PlannedAt: time.Now().UTC(),
		Notes:     launchPlanNotes,
		Metadata:  cloneStringMap(req.Metadata),
	}
}

// ValidateLaunchPlan checks the plan struct fields without accessing the
// filesystem.
func (b *defaultBridge) ValidateLaunchPlan(ctx context.Context, plan LaunchPlan) error {
	var issues []string

	if err := validateSessionID(plan.SessionID); err != nil {
		issues = append(issues, "session_id: "+err.Error())
	}

	pathFields := []struct {
		name  string
		value string
	}{
		{"mcdr_root", plan.MCDRRoot},
		{"config_dir", plan.ConfigDir},
		{"plugins_dir", plan.PluginsDir},
		{"server_dir", plan.ServerDir},
		{"logs_dir", plan.LogsDir},
		{"server_work_dir", plan.ServerWorkDir},
		{"planned_config_path", plan.PlannedConfigPath},
		{"planned_server_properties_path", plan.PlannedServerPropertiesPath},
		{"planned_eula_path", plan.PlannedEULAPath},
		{"command.working_dir", plan.Command.WorkingDir},
	}
	for _, f := range pathFields {
		if err := validateRuntimeRelativePath(f.name, f.value); err != nil {
			issues = append(issues, f.name+": "+err.Error())
		}
	}

	for i, arg := range plan.Command.Argv {
		if stringsContains(arg, "\n") || stringsContains(arg, "\r") {
			issues = append(issues, fmt.Sprintf("command.argv[%d]: must not contain newlines", i))
		}
	}
	if plan.Command.Executable == "" {
		issues = append(issues, "command.executable: required")
	}
	if len(plan.Command.Argv) == 0 {
		issues = append(issues, "command.argv: at least one argument required")
	}

	switch plan.Stop.Strategy {
	case StopStrategyStdin, StopStrategySignal, StopStrategyNone:
	default:
		issues = append(issues, fmt.Sprintf("stop.strategy: unsupported value %q", plan.Stop.Strategy))
	}

	for _, pc := range []struct {
		name  string
		value PreconditionStatus
	}{
		{"preconditions.python_available", plan.Preconditions.PythonAvailable},
		{"preconditions.mcdr_installed", plan.Preconditions.MCDRInstalled},
		{"preconditions.server_jar_present", plan.Preconditions.ServerJarPresent},
		{"preconditions.eula_present", plan.Preconditions.EULAPresent},
	} {
		switch pc.value {
		case PreconditionUnknown, PreconditionRequired, PreconditionNotChecked:
		default:
			issues = append(issues, fmt.Sprintf("%s: unsupported value %q", pc.name, pc.value))
		}
	}

	validStatus := false
	for _, s := range ValidStatuses {
		if plan.Status == s {
			validStatus = true
			break
		}
	}
	if !validStatus {
		issues = append(issues, fmt.Sprintf("status: unsupported value %q", plan.Status))
	}

	if len(issues) > 0 {
		return fmt.Errorf("MCDR launch plan validation: %v", issues)
	}
	return nil
}

// InspectLaunchPlan reads the persisted launch-plan manifest and returns
// a status report. It never modifies the manifest.
func (b *defaultBridge) InspectLaunchPlan(ctx context.Context, sessionID string) (LaunchPlanStatus, error) {
	sessionLayout, err := process.NewSessionRuntimeLayout(b.runtimeRoot, sessionID)
	if err != nil {
		return LaunchPlanStatus{}, fmt.Errorf("build session layout: %w", err)
	}

	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		return LaunchPlanStatus{}, fmt.Errorf("build MCDR layout: %w", err)
	}

	return inspectLaunchPlanManifest(mcdrLayout), nil
}

func mustRelative(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}
