package runtimeprofile

import (
	"fmt"
	"strings"
	"time"
)

type Type string
type StopStrategy string
type LogMode string

const (
	TypeDummy    Type = "dummy"
	TypeTerminal Type = "terminal"

	StopNone      StopStrategy = "none"
	StopStdin     StopStrategy = "stdin"
	StopTerminate StopStrategy = "terminate"

	LogMemory   LogMode = "memory"
	LogCombined LogMode = "combined"
)

const DefaultProfileID = "dummy-process"

type Profile struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	RuntimeType         Type              `json:"runtimeType"`
	CommandArgv         []string          `json:"commandArgv,omitempty"`
	WorkingDir          string            `json:"workingDir,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	StopStrategy        StopStrategy      `json:"stopStrategy"`
	StopStdinCommand    string            `json:"stopStdinCommand,omitempty"`
	GracefulStopTimeout time.Duration     `json:"gracefulStopTimeout"`
	ForceKillTimeout    time.Duration     `json:"forceKillTimeout"`
	LogMode             LogMode           `json:"logMode"`
	Enabled             bool              `json:"enabled"`
	Notes               string            `json:"notes,omitempty"`
}

func DummyProcess() Profile {
	return Profile{ID: DefaultProfileID, Name: "Built-in dummy process", RuntimeType: TypeDummy, StopStrategy: StopNone, GracefulStopTimeout: 5 * time.Second, ForceKillTimeout: time.Second, LogMode: LogMemory, Enabled: true, Notes: "Go-native development runtime; starts no OS process"}
}

func (p Profile) Public() Profile {
	p.CommandArgv = nil
	p.WorkingDir = ""
	p.Env = nil
	p.StopStdinCommand = ""
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
	case TypeTerminal:
		if len(value.CommandArgv) == 0 || strings.TrimSpace(value.CommandArgv[0]) == "" {
			return fmt.Errorf("terminal runtime profile %q requires command argv", value.ID)
		}
		for _, arg := range value.CommandArgv {
			if strings.ContainsAny(arg, "\r\n\x00") {
				return fmt.Errorf("runtime profile %q command argv contains control characters", value.ID)
			}
		}
		if looksLikeShell(value.CommandArgv[0]) {
			return fmt.Errorf("runtime profile %q must use executable argv, not a shell command", value.ID)
		}
		if strings.TrimSpace(value.WorkingDir) == "" {
			return fmt.Errorf("terminal runtime profile %q requires a working directory", value.ID)
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
