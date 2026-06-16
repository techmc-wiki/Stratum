package runtimeprofile

import (
	"strings"
	"testing"
	"time"
)

func TestValidationAndBuiltInRegistry(t *testing.T) {
	if err := Validate(DummyProcess()); err != nil {
		t.Fatal(err)
	}
	invalid := Profile{ID: "terminal-empty", Name: "Empty", RuntimeType: TypeTerminal, StopStrategy: StopTerminate, LogMode: LogMemory, Enabled: true}
	if err := Validate(invalid); err == nil {
		t.Fatal("terminal profile without command should fail")
	}
	unsafe := invalid
	unsafe.ID, unsafe.CommandArgv, unsafe.WorkingDir = "terminal-shell", []string{"sh", "-c", "echo unsafe"}, "."
	if err := Validate(unsafe); err == nil {
		t.Fatal("shell executable should fail")
	}
	registry := Builtins()
	values := registry.ListEnabled()
	if len(values) != 1 || values[0].ID != DefaultProfileID {
		t.Fatalf("profiles=%+v", values)
	}
	if _, err := registry.Get(""); err != nil {
		t.Fatal(err)
	}
	safe := invalid
	safe.ID, safe.CommandArgv, safe.WorkingDir = "terminal-safe", []string{"trusted-helper", "--serve"}, "."
	if err := Validate(safe); err != nil {
		t.Fatalf("safe terminal profile: %v", err)
	}
	defaultWorkDir := safe
	defaultWorkDir.ID, defaultWorkDir.WorkingDir = "terminal-default-work", ""
	if err := Validate(defaultWorkDir); err != nil {
		t.Fatalf("empty working dir should use session work dir: %v", err)
	}
}

func TestMCDRPythonProfileValidation(t *testing.T) {
	valid := Profile{
		ID: "mcdr-python-1.17", Name: "MCDR Python 1.17",
		RuntimeType: TypeMCDRPython, CommandArgv: []string{"python3", "-m", "mcdreforged"},
		WorkingDir: ".", StopStrategy: StopStdin, StopStdinCommand: "!!MCDR stop",
		GracefulStopTimeout: 30 * time.Second, ForceKillTimeout: 10 * time.Second,
		LogMode: LogCombined, Enabled: true,
		GracefulStopSteps: []GracefulStopStep{
			{Type: GracefulStopStdinCommand, Command: "!!MCDR stop", Timeout: 30 * time.Second},
			{Type: GracefulStopSignal, Signal: "SIGTERM", Timeout: 10 * time.Second},
			{Type: GracefulStopSignal, Signal: "SIGKILL", Timeout: 5 * time.Second},
		},
		ReadinessCheck: &ReadinessCheckConfig{Type: ReadinessLogPattern, Pattern: "MCDR started", Timeout: 60 * time.Second},
		HealthCheck:    &HealthCheckConfig{Type: HealthProcessAlive, MaxSilentSeconds: 300},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid mcdr-python profile: %v", err)
	}
	shell := valid
	shell.ID, shell.CommandArgv = "mcdr-shell", []string{"bash", "-c", "mcdreforged"}
	if err := Validate(shell); err == nil {
		t.Fatal("mcdr-python with shell executable should fail")
	}
	emptyArgv := valid
	emptyArgv.ID, emptyArgv.CommandArgv = "mcdr-no-argv", nil
	if err := Validate(emptyArgv); err == nil {
		t.Fatal("mcdr-python without argv should fail")
	}
}

func TestReadinessCheckValidation(t *testing.T) {
	invalidType := Profile{
		ID: "invalid-readiness", Name: "Invalid", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		ReadinessCheck: &ReadinessCheckConfig{Type: "bad-type", Timeout: time.Second},
	}
	if err := Validate(invalidType); err == nil || !strings.Contains(err.Error(), "unsupported readiness check type") {
		t.Fatalf("expected unsupported readiness check type: %v", err)
	}
	noPattern := Profile{
		ID: "no-pattern", Name: "No Pattern", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		ReadinessCheck: &ReadinessCheckConfig{Type: ReadinessLogPattern, Timeout: time.Second},
	}
	if err := Validate(noPattern); err == nil || !strings.Contains(err.Error(), "requires a pattern") {
		t.Fatalf("expected pattern required: %v", err)
	}
	badTimeout := Profile{
		ID: "bad-timeout", Name: "Bad Timeout", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		ReadinessCheck: &ReadinessCheckConfig{Type: ReadinessNone, Timeout: -time.Second},
	}
	if err := Validate(badTimeout); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("expected negative timeout error: %v", err)
	}
}

func TestHealthCheckValidation(t *testing.T) {
	invalidType := Profile{
		ID: "invalid-health", Name: "Invalid", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		HealthCheck: &HealthCheckConfig{Type: "bad-type"},
	}
	if err := Validate(invalidType); err == nil || !strings.Contains(err.Error(), "unsupported health check type") {
		t.Fatalf("expected unsupported health check type: %v", err)
	}
	negativeSilent := Profile{
		ID: "negative-silent", Name: "Negative", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		HealthCheck: &HealthCheckConfig{Type: HealthProcessAlive, MaxSilentSeconds: -1},
	}
	if err := Validate(negativeSilent); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("expected negative max silent seconds error: %v", err)
	}
}

func TestGracefulStopStepsValidation(t *testing.T) {
	badType := Profile{
		ID: "bad-step-type", Name: "Bad Step", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		GracefulStopSteps: []GracefulStopStep{
			{Type: "bad-type", Timeout: time.Second},
		},
	}
	if err := Validate(badType); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported step type: %v", err)
	}
	noCommand := Profile{
		ID: "no-command", Name: "No Command", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		GracefulStopSteps: []GracefulStopStep{
			{Type: GracefulStopStdinCommand, Timeout: time.Second},
		},
	}
	if err := Validate(noCommand); err == nil || !strings.Contains(err.Error(), "requires a command") {
		t.Fatalf("expected command required: %v", err)
	}
	badSignal := Profile{
		ID: "bad-signal", Name: "Bad Signal", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		GracefulStopSteps: []GracefulStopStep{
			{Type: GracefulStopSignal, Signal: "SIGHUP", Timeout: time.Second},
		},
	}
	if err := Validate(badSignal); err == nil || !strings.Contains(err.Error(), "unsupported signal") {
		t.Fatalf("expected unsupported signal: %v", err)
	}
	negativeTimeout := Profile{
		ID: "negative-timeout", Name: "Negative", RuntimeType: TypeDummy,
		StopStrategy: StopNone, LogMode: LogMemory, Enabled: true,
		GracefulStopSteps: []GracefulStopStep{
			{Type: GracefulStopSignal, Signal: "SIGTERM", Timeout: -time.Second},
		},
	}
	if err := Validate(negativeTimeout); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("expected negative timeout error: %v", err)
	}
}

func TestPublicSanitization(t *testing.T) {
	profile := Profile{
		ID: "sensitive", Name: "Sensitive", RuntimeType: TypeMCDRPython,
		CommandArgv: []string{"python3", "-m", "mcdreforged"},
		WorkingDir:  ".", Env: map[string]string{"SECRET": "value"},
		StopStrategy: StopStdin, StopStdinCommand: "!!MCDR stop",
		GracefulStopTimeout: 30 * time.Second, ForceKillTimeout: 10 * time.Second,
		LogMode: LogCombined, Enabled: true,
		ReadinessCheck:    &ReadinessCheckConfig{Type: ReadinessLogPattern, Pattern: "MCDR started", Timeout: 60 * time.Second},
		HealthCheck:       &HealthCheckConfig{Type: HealthProcessAlive, MaxSilentSeconds: 300},
		GracefulStopSteps: []GracefulStopStep{{Type: GracefulStopStdinCommand, Command: "!!MCDR stop", Timeout: 30 * time.Second}},
	}
	public := profile.Public()
	if public.CommandArgv != nil || public.WorkingDir != "" || public.Env != nil ||
		public.StopStdinCommand != "" || public.ReadinessCheck != nil ||
		public.HealthCheck != nil || public.GracefulStopSteps != nil {
		t.Fatalf("public should sanitize sensitive fields: %+v", public)
	}
}
