package runtimeprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
