package runtimeprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadTrustedFile(t *testing.T) {
	path := writeConfig(t, `{
  "runtime_profiles": [{
    "id": "trusted-terminal",
    "name": "Trusted terminal",
    "runtime_type": "terminal",
    "command_argv": ["server", "--nogui"],
    "working_dir": ".",
    "env": {"JAVA_HOME": "trusted"},
    "stop_strategy": "stdin",
    "stop_stdin_command": "stop",
    "graceful_stop_timeout": "5s",
    "force_kill_timeout": "2s",
    "log_mode": "combined",
    "enabled": true,
    "notes": "local deployment configuration"
  }]
}`)
	profiles, err := LoadTrustedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "trusted-terminal" || profiles[0].GracefulStopTimeout.String() != "5s" || profiles[0].LogMode != LogCombined {
		t.Fatalf("profiles=%+v", profiles)
	}
}

func TestLoadTrustedFileAcceptsUTF8BOM(t *testing.T) {
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"runtime_profiles":[]}`)...)
	path := filepath.Join(t.TempDir(), "runtime-profiles.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTrustedFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTrustedFileRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "malformed", content: `{"runtime_profiles":[`, want: "decode runtime profile config"},
		{name: "unknown field", content: `{"runtime_profiles":[],"unexpected":true}`, want: "unknown field"},
		{name: "invalid profile", content: `{"runtime_profiles":[{"id":"bad","name":"Bad","runtime_type":"terminal","working_dir":".","stop_strategy":"terminate","log_mode":"memory","enabled":true}]}`, want: "requires command argv"},
		{name: "invalid duration", content: `{"runtime_profiles":[{"id":"bad","name":"Bad","runtime_type":"dummy","stop_strategy":"none","graceful_stop_timeout":"soon","log_mode":"memory","enabled":true}]}`, want: "invalid graceful_stop_timeout"},
		{name: "duplicate", content: `{"runtime_profiles":[{"id":"same","name":"One","runtime_type":"dummy","stop_strategy":"none","log_mode":"memory","enabled":true},{"id":"same","name":"Two","runtime_type":"dummy","stop_strategy":"none","log_mode":"memory","enabled":true}]}`, want: "duplicates an earlier profile"},
		{name: "invalid readiness check duration", content: `{"runtime_profiles":[{"id":"bad","name":"Bad","runtime_type":"dummy","stop_strategy":"none","log_mode":"memory","enabled":true,"readiness_check":{"type":"log-pattern","pattern":"ready","timeout":"soon"}}]}`, want: "invalid readiness_check.timeout"},
		{name: "invalid graceful stop step duration", content: `{"runtime_profiles":[{"id":"bad","name":"Bad","runtime_type":"dummy","stop_strategy":"none","log_mode":"memory","enabled":true,"graceful_stop_steps":[{"type":"signal","signal":"SIGTERM","timeout":"soon"}]}]}`, want: "invalid graceful_stop_steps"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadTrustedFile(writeConfig(t, test.content))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestRegisterAllIsAtomicAndDisabledProfilesStayUnavailable(t *testing.T) {
	registry := Builtins()
	disabled := Profile{ID: "disabled", Name: "Disabled", RuntimeType: TypeDummy, StopStrategy: StopNone, LogMode: LogMemory}
	if err := registry.RegisterAll([]Profile{disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Get(disabled.ID); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled profile err=%v", err)
	}
	if len(registry.ListEnabled()) != 1 {
		t.Fatalf("enabled=%+v", registry.ListEnabled())
	}

	valid := Profile{ID: "would-be-added", Name: "Would be added", RuntimeType: TypeDummy, StopStrategy: StopNone, LogMode: LogMemory, Enabled: true}
	conflict := DummyProcess()
	if err := registry.RegisterAll([]Profile{valid, conflict}); err == nil {
		t.Fatal("registration conflict should fail")
	}
	if _, err := registry.Get(valid.ID); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("batch was partially registered: %v", err)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime-profiles.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMCDRPythonProfileFromConfig(t *testing.T) {
	path := writeConfig(t, `{
  "runtime_profiles": [{
    "id": "mcdr-python-1.17",
    "name": "MCDR Python 1.17",
    "runtime_type": "mcdr-python",
    "command_argv": ["python3", "-m", "mcdreforged"],
    "working_dir": "sessions/{sessionId}/mcdr",
    "env": {"MCDR_CONFIG": "{sessionRoot}/mcdr/config.yml", "PYTHONUNBUFFERED": "1"},
    "stop_strategy": "stdin",
    "stop_stdin_command": "!!MCDR stop",
    "graceful_stop_timeout": "30s",
    "force_kill_timeout": "10s",
    "log_mode": "combined",
    "enabled": true,
    "notes": "1.17 Fabric MCDR Python runtime",
    "readiness_check": {
      "type": "log-pattern",
      "pattern": "MCDR started",
      "timeout": "60s"
    },
    "health_check": {
      "type": "process-alive",
      "max_silent_seconds": 300,
      "timeout": "0s"
    },
    "graceful_stop_steps": [
      {"type": "stdin-command", "command": "!!MCDR stop", "timeout": "30s"},
      {"type": "signal", "signal": "SIGTERM", "timeout": "10s"},
      {"type": "signal", "signal": "SIGKILL", "timeout": "5s"}
    ]
  }]
}`)
	profiles, err := LoadTrustedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile got %d", len(profiles))
	}
	p := profiles[0]
	if p.ID != "mcdr-python-1.17" || p.RuntimeType != TypeMCDRPython {
		t.Fatalf("profile=%+v", p)
	}
	if p.ReadinessCheck == nil || p.ReadinessCheck.Type != ReadinessLogPattern || p.ReadinessCheck.Pattern != "MCDR started" || p.ReadinessCheck.Timeout != 60*time.Second {
		t.Fatalf("readiness=%+v", p.ReadinessCheck)
	}
	if p.HealthCheck == nil || p.HealthCheck.Type != HealthProcessAlive || p.HealthCheck.MaxSilentSeconds != 300 {
		t.Fatalf("health=%+v", p.HealthCheck)
	}
	if len(p.GracefulStopSteps) != 3 {
		t.Fatalf("expected 3 graceful stop steps got %d", len(p.GracefulStopSteps))
	}
	if p.GracefulStopSteps[0].Type != GracefulStopStdinCommand || p.GracefulStopSteps[0].Command != "!!MCDR stop" || p.GracefulStopSteps[0].Timeout != 30*time.Second {
		t.Fatalf("step0=%+v", p.GracefulStopSteps[0])
	}
	if p.GracefulStopSteps[1].Type != GracefulStopSignal || p.GracefulStopSteps[1].Signal != "SIGTERM" || p.GracefulStopSteps[1].Timeout != 10*time.Second {
		t.Fatalf("step1=%+v", p.GracefulStopSteps[1])
	}
	if p.GracefulStopSteps[2].Type != GracefulStopSignal || p.GracefulStopSteps[2].Signal != "SIGKILL" || p.GracefulStopSteps[2].Timeout != 5*time.Second {
		t.Fatalf("step2=%+v", p.GracefulStopSteps[2])
	}
}
