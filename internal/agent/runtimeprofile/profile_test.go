package runtimeprofile

import "testing"

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
}
