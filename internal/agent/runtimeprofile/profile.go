package runtimeprofile

import (
	"fmt"
	"strings"
	"time"
)

type (
	Type                 string
	StopStrategy         string
	LogMode              string
	ReadinessCheckType   string
	HealthCheckType      string
	GracefulStopStepType string
)

const (
	TypeDummy      Type = "dummy"
	TypeTerminal   Type = "terminal"
	TypeMCDRPython Type = "mcdr-python"

	StopNone      StopStrategy = "none"
	StopStdin     StopStrategy = "stdin"
	StopTerminate StopStrategy = "terminate"

	LogMemory   LogMode = "memory"
	LogCombined LogMode = "combined"

	ReadinessLogPattern ReadinessCheckType = "log-pattern"
	ReadinessNone       ReadinessCheckType = "none"

	HealthProcessAlive HealthCheckType = "process-alive"
	HealthNone         HealthCheckType = "none"

	GracefulStopStdinCommand GracefulStopStepType = "stdin-command"
	GracefulStopSignal       GracefulStopStepType = "signal"
)

const DefaultProfileID = "dummy-process"

type ReadinessCheckConfig struct {
	Type    ReadinessCheckType `json:"type"`
	Pattern string             `json:"pattern,omitempty"`
	Timeout time.Duration      `json:"timeout"`
}

type HealthCheckConfig struct {
	Type             HealthCheckType `json:"type"`
	MaxSilentSeconds int             `json:"maxSilentSeconds,omitempty"`
	Timeout          time.Duration   `json:"timeout,omitempty"`
}

type GracefulStopStep struct {
	Type    GracefulStopStepType `json:"type"`
	Command string               `json:"command,omitempty"`
	Signal  string               `json:"signal,omitempty"`
	Timeout time.Duration        `json:"timeout"`
}

type Profile struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	RuntimeType         Type                  `json:"runtimeType"`
	CommandArgv         []string              `json:"commandArgv,omitempty"`
	WorkingDir          string                `json:"workingDir,omitempty"`
	Env                 map[string]string     `json:"env,omitempty"`
	StopStrategy        StopStrategy          `json:"stopStrategy"`
	StopStdinCommand    string                `json:"stopStdinCommand,omitempty"`
	GracefulStopTimeout time.Duration         `json:"gracefulStopTimeout"`
	ForceKillTimeout    time.Duration         `json:"forceKillTimeout"`
	LogMode             LogMode               `json:"logMode"`
	Enabled             bool                  `json:"enabled"`
	Notes               string                `json:"notes,omitempty"`
	ReadinessCheck      *ReadinessCheckConfig `json:"readinessCheck,omitempty"`
	HealthCheck         *HealthCheckConfig    `json:"healthCheck,omitempty"`
	GracefulStopSteps   []GracefulStopStep    `json:"gracefulStopSteps,omitempty"`
}

func DummyProcess() Profile {
	return Profile{ID: DefaultProfileID, Name: "Built-in dummy process", RuntimeType: TypeDummy, StopStrategy: StopNone, GracefulStopTimeout: 5 * time.Second, ForceKillTimeout: time.Second, LogMode: LogMemory, Enabled: true, Notes: "Go-native development runtime; starts no OS process"}
}

func (p Profile) Public() Profile {
	p.CommandArgv = nil
	p.WorkingDir = ""
	p.Env = nil
	p.StopStdinCommand = ""
	p.ReadinessCheck = nil
	p.HealthCheck = nil
	p.GracefulStopSteps = nil
	return p
}

func Validate(value Profile) error {
	if strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.Name) == "" {
		return fmt.Errorf("runtime profile requires id and name")
	}
	switch value.RuntimeType {
	case TypeDummy:
		if len(value.CommandArgv) != 0 {
			return fmt.Errorf("dummy runtime profile %q must not define command argv", value.ID)
		}
	case TypeTerminal, TypeMCDRPython:
		if len(value.CommandArgv) == 0 || strings.TrimSpace(value.CommandArgv[0]) == "" {
			return fmt.Errorf("%s runtime profile %q requires command argv", value.RuntimeType, value.ID)
		}
		for _, arg := range value.CommandArgv {
			if strings.ContainsAny(arg, "\r\n\x00") {
				return fmt.Errorf("runtime profile %q command argv contains control characters", value.ID)
			}
		}
		if looksLikeShell(value.CommandArgv[0]) {
			return fmt.Errorf("runtime profile %q must use executable argv, not a shell command", value.ID)
		}
	default:
		return fmt.Errorf("runtime profile %q has unsupported runtime type %q", value.ID, value.RuntimeType)
	}
	switch value.StopStrategy {
	case StopNone, StopStdin, StopTerminate:
	default:
		return fmt.Errorf("runtime profile %q has unsupported stop strategy %q", value.ID, value.StopStrategy)
	}
	if value.StopStrategy == StopStdin && strings.TrimSpace(value.StopStdinCommand) == "" {
		return fmt.Errorf("runtime profile %q stdin stop strategy requires a command", value.ID)
	}
	if value.GracefulStopTimeout < 0 || value.ForceKillTimeout < 0 {
		return fmt.Errorf("runtime profile %q timeouts must not be negative", value.ID)
	}
	if err := validateReadinessCheck(value); err != nil {
		return err
	}
	if err := validateHealthCheck(value); err != nil {
		return err
	}
	if err := validateGracefulStopSteps(value); err != nil {
		return err
	}
	if value.LogMode != LogMemory && value.LogMode != LogCombined {
		return fmt.Errorf("runtime profile %q has unsupported log mode %q", value.ID, value.LogMode)
	}
	for key, item := range value.Env {
		if strings.TrimSpace(key) == "" || strings.Contains(key, "=") || strings.ContainsAny(key+item, "\r\n\x00") {
			return fmt.Errorf("runtime profile %q contains an invalid environment entry", value.ID)
		}
	}
	return nil
}

func looksLikeShell(executable string) bool {
	value := strings.ToLower(strings.TrimSpace(executable))
	return value == "sh" || value == "bash" || value == "zsh" || value == "cmd" || value == "cmd.exe" || value == "powershell" || value == "powershell.exe" || value == "pwsh"
}

var validSignals = map[string]bool{"SIGTERM": true, "SIGINT": true, "SIGKILL": true}

func validateReadinessCheck(value Profile) error {
	if value.ReadinessCheck == nil {
		return nil
	}
	switch value.ReadinessCheck.Type {
	case ReadinessLogPattern:
		if strings.TrimSpace(value.ReadinessCheck.Pattern) == "" {
			return fmt.Errorf("runtime profile %q readiness check log-pattern requires a pattern", value.ID)
		}
	case ReadinessNone:
	default:
		return fmt.Errorf("runtime profile %q has unsupported readiness check type %q", value.ID, value.ReadinessCheck.Type)
	}
	if value.ReadinessCheck.Timeout < 0 {
		return fmt.Errorf("runtime profile %q readiness check timeout must not be negative", value.ID)
	}
	return nil
}

func validateHealthCheck(value Profile) error {
	if value.HealthCheck == nil {
		return nil
	}
	switch value.HealthCheck.Type {
	case HealthProcessAlive:
		if value.HealthCheck.MaxSilentSeconds < 0 {
			return fmt.Errorf("runtime profile %q health check max silent seconds must not be negative", value.ID)
		}
	case HealthNone:
	default:
		return fmt.Errorf("runtime profile %q has unsupported health check type %q", value.ID, value.HealthCheck.Type)
	}
	if value.HealthCheck.Timeout < 0 {
		return fmt.Errorf("runtime profile %q health check timeout must not be negative", value.ID)
	}
	return nil
}

func validateGracefulStopSteps(value Profile) error {
	if len(value.GracefulStopSteps) == 0 {
		return nil
	}
	for i, step := range value.GracefulStopSteps {
		switch step.Type {
		case GracefulStopStdinCommand:
			if strings.TrimSpace(step.Command) == "" {
				return fmt.Errorf("runtime profile %q graceful stop step %d stdin-command requires a command", value.ID, i)
			}
		case GracefulStopSignal:
			if !validSignals[step.Signal] {
				return fmt.Errorf("runtime profile %q graceful stop step %d has unsupported signal %q", value.ID, i, step.Signal)
			}
		default:
			return fmt.Errorf("runtime profile %q graceful stop step %d has unsupported type %q", value.ID, i, step.Type)
		}
		if step.Timeout < 0 {
			return fmt.Errorf("runtime profile %q graceful stop step %d timeout must not be negative", value.ID, i)
		}
	}
	return nil
}
