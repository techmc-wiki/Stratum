package mcdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusCrashed    Status = "crashed"
)

type RuntimeState struct {
	SessionID        string
	Status           Status
	PID              int
	ExitCode         *int
	Crashed          bool
	MCDRRoot         string
	RuntimeProfileID string
	StartedAt        *time.Time
	StoppedAt        *time.Time
	LastError        string
}

type Supervisor struct {
	processSupervisor *agentprocess.Supervisor
}

func NewSupervisor(processSupervisor *agentprocess.Supervisor) *Supervisor {
	return &Supervisor{processSupervisor: processSupervisor}
}

func (s *Supervisor) Start(ctx context.Context, sessionID string, profile runtimeprofile.Profile) (RuntimeState, error) {
	if profile.RuntimeType != runtimeprofile.TypeMCDRPython {
		return RuntimeState{}, fmt.Errorf("MCDR supervisor requires mcdr-python profile, got %q", profile.RuntimeType)
	}

	runtimeRoot := s.processSupervisor.RuntimeRoot()
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(runtimeRoot, sessionID)
	if err != nil {
		return RuntimeState{}, err
	}

	mcdrLayout, err := sessionLayout.MCDR()
	if err != nil {
		return RuntimeState{}, err
	}

	if err := mcdrLayout.Create(); err != nil {
		return RuntimeState{}, fmt.Errorf("prepare MCDR runtime directories: %w", err)
	}

	if err := mcdrLayout.WriteManifest(); err != nil {
		return RuntimeState{}, fmt.Errorf("write MCDR layout manifest: %w", err)
	}

	manifestData := readMaterializationManifest(sessionLayout.ConfigDir)
	serverJarName := manifestData["serverJarName"]
	mcdrExecutable := manifestData["mcdrExecutable"]
	javaExecutable := manifestData["javaExecutable"]

	mcdrConfig := NewRuntimeConfig(mcdrLayout)
	mcdrConfig.ServerJarName = serverJarName
	mcdrConfig.JavaExecutable = javaExecutable
	mcdrConfig.HTTPProxy = os.Getenv("STRATUM_HTTP_PROXY")
	if err := writeServerBootstrapFiles(mcdrLayout); err != nil {
		return RuntimeState{}, err
	}
	if err := writePermissionFile(mcdrLayout); err != nil {
		return RuntimeState{}, err
	}
	if _, err := WriteRuntimeConfig(mcdrLayout, mcdrConfig); err != nil {
		return RuntimeState{}, fmt.Errorf("write MCDR config.yml: %w", err)
	}

	actualProfile := profile
	workingDir, err := filepath.Rel(sessionLayout.RuntimeRoot, mcdrLayout.MCDRConfigDir)
	if err != nil {
		return RuntimeState{}, fmt.Errorf("resolve MCDR working directory: %w", err)
	}
	actualProfile.WorkingDir = workingDir
	if mcdrExecutable != "" {
		if _, statErr := os.Stat(mcdrExecutable); statErr == nil {
			if len(profile.CommandArgv) > 0 {
				actualProfile.CommandArgv = append([]string{mcdrExecutable}, profile.CommandArgv[1:]...)
			} else {
				actualProfile.CommandArgv = []string{mcdrExecutable}
			}
		}
	}

	model, err := s.processSupervisor.StartProcess(ctx, sessionID, actualProfile)
	if err != nil {
		return RuntimeState{}, err
	}

	if profile.ReadinessCheck != nil && profile.ReadinessCheck.Type == runtimeprofile.ReadinessLogPattern {
		if err := s.processSupervisor.WaitForLog(sessionID, profile.ReadinessCheck.Pattern, profile.ReadinessCheck.Timeout); err != nil {
			s.processSupervisor.StopProcess(ctx, sessionID)
			return RuntimeState{}, fmt.Errorf("MCDR readiness check failed: %w", err)
		}
	}

	return s.runtimeState(model), nil
}

func writeServerBootstrapFiles(layout agentprocess.MCDRRuntimeLayout) error {
	if err := os.MkdirAll(layout.MCDRServerDir, 0o750); err != nil {
		return fmt.Errorf("create Minecraft server directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layout.MCDRServerDir, "eula.txt"), []byte("eula=true\n"), 0o640); err != nil {
		return fmt.Errorf("write Minecraft eula.txt: %w", err)
	}
	properties := "server-port=25565\n" +
		"gamemode=creative\n" +
		"difficulty=peaceful\n" +
		"spawn-protection=0\n" +
		"max-players=20\n" +
		"online-mode=false\n" +
		"enable-command-block=true\n"
	if err := os.WriteFile(filepath.Join(layout.MCDRServerDir, "server.properties"), []byte(properties), 0o640); err != nil {
		return fmt.Errorf("write Minecraft server.properties: %w", err)
	}
	return nil
}

func writePermissionFile(layout agentprocess.MCDRRuntimeLayout) error {
	content := "default_level: user\nowner:\nadmin:\nhelper:\nuser:\nguest:\n"
	if err := os.WriteFile(filepath.Join(layout.MCDRConfigDir, "permission.yml"), []byte(content), 0o640); err != nil {
		return fmt.Errorf("write MCDR permission.yml: %w", err)
	}
	return nil
}

func readMaterializationManifest(configDir string) map[string]string {
	manifestPath := filepath.Join(configDir, "environment-materialization.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return map[string]string{}
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	for key, value := range manifest {
		if str, ok := value.(string); ok {
			result[key] = str
		}
	}
	return result
}

func (s *Supervisor) Stop(ctx context.Context, sessionID string) (RuntimeState, error) {
	if s.processSupervisor.IsRunning(sessionID) {
		model, err := s.processSupervisor.StopProcess(ctx, sessionID)
		if err != nil {
			return RuntimeState{}, err
		}
		return s.runtimeState(model), nil
	}
	model := s.processSupervisor.InspectProcess(sessionID)
	if model.Status == agentprocess.StatusStopped ||
		model.Status == agentprocess.StatusExited ||
		model.Status == agentprocess.StatusCrashed {
		return s.runtimeState(model), nil
	}
	return RuntimeState{}, fmt.Errorf("session %q process cannot stop from %s", sessionID, model.Status)
}

func (s *Supervisor) Restart(ctx context.Context, sessionID string, profile runtimeprofile.Profile) (RuntimeState, error) {
	if profile.RuntimeType != runtimeprofile.TypeMCDRPython {
		return RuntimeState{}, fmt.Errorf("MCDR supervisor requires mcdr-python profile, got %q", profile.RuntimeType)
	}
	if _, err := s.Stop(ctx, sessionID); err != nil && !errors.Is(err, context.Canceled) {
		if s.processSupervisor.IsRunning(sessionID) {
			return RuntimeState{}, err
		}
	}
	return s.Start(ctx, sessionID, profile)
}

func (s *Supervisor) SendCommand(sessionID, command string) error {
	return s.processSupervisor.SendCommand(sessionID, command)
}

func (s *Supervisor) Inspect(sessionID string) RuntimeState {
	return s.runtimeState(s.processSupervisor.InspectProcess(sessionID))
}

func (s *Supervisor) IsRunning(sessionID string) bool {
	return s.processSupervisor.IsRunning(sessionID)
}

func (s *Supervisor) CollectLogs(sessionID string, maxBytes int) []string {
	return s.processSupervisor.CollectLogs(sessionID, maxBytes)
}

func (s *Supervisor) RuntimeRoot() string {
	return s.processSupervisor.RuntimeRoot()
}

func (s *Supervisor) runtimeState(model agentprocess.RuntimeProcess) RuntimeState {
	status := StatusNotStarted
	switch model.Status {
	case agentprocess.StatusStarting:
		status = StatusStarting
	case agentprocess.StatusRunning:
		status = StatusRunning
	case agentprocess.StatusStopping:
		status = StatusStopping
	case agentprocess.StatusStopped:
		status = StatusStopped
	case agentprocess.StatusCrashed:
		status = StatusCrashed
	}
	return RuntimeState{
		SessionID:        model.SessionID,
		Status:           status,
		PID:              model.PID,
		ExitCode:         model.ExitCode,
		Crashed:          model.Crashed,
		RuntimeProfileID: model.RuntimeProfileID,
		StartedAt:        model.StartedAt,
		StoppedAt:        model.StoppedAt,
		LastError:        model.LastError,
	}
}
