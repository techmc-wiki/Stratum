package lucy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeRunner struct {
	result CommandResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ CommandRequest) (CommandResult, error) {
	return f.result, f.err
}

func TestCLIAdapterRequiresCommandPath(t *testing.T) {
	_, err := NewCLIAdapter(CLIAdapterOptions{Runner: &fakeRunner{}})
	if err == nil {
		t.Fatal("expected error when command path missing")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestCLIAdapterRequiresRunner(t *testing.T) {
	_, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy"})
	if err == nil {
		t.Fatal("expected error when runner missing")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestCLIAdapterCapabilities(t *testing.T) {
	caps := Capabilities{SupportsPlan: true, SupportedSources: []string{"test"}, SupportedLoaders: []string{}, Metadata: map[string]string{}}
	stdout, _ := json.Marshal(caps)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.SupportsPlan {
		t.Fatal("expected SupportsPlan true")
	}
}

func TestCLIAdapterPlanEnvironment(t *testing.T) {
	plan := EnvironmentPlan{Actions: []PlanAction{}, Warnings: []string{}, Errors: []string{}, Metadata: map[string]string{}}
	stdout, _ := json.Marshal(plan)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{EnvironmentID: "env-1", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet", Packages: []PackageRef{}, LocalArtifacts: []LocalArtifactRef{}, Metadata: map[string]string{}}
	result, err := adapter.PlanEnvironment(context.Background(), PlanEnvironmentRequest{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if result.Actions == nil {
		t.Fatal("expected actions")
	}
}

func TestCLIAdapterLockEnvironment(t *testing.T) {
	lock := EnvironmentLock{Packages: []LockedPackage{}, Artifacts: []LockedArtifact{}, ProviderMetadata: map[string]string{}}
	stdout, _ := json.Marshal(lock)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{EnvironmentID: "env-1", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet", Packages: []PackageRef{}, LocalArtifacts: []LocalArtifactRef{}, Metadata: map[string]string{}}
	result, err := adapter.LockEnvironment(context.Background(), LockEnvironmentRequest{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if result.Packages == nil {
		t.Fatal("expected packages")
	}
}

func TestCLIAdapterCheckStatus(t *testing.T) {
	status := EnvironmentStatus{Missing: []string{}, Drifted: []string{}, Warnings: []string{}, Errors: []string{}, Metadata: map[string]string{}}
	stdout, _ := json.Marshal(status)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{EnvironmentID: "env-1", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet", Packages: []PackageRef{}, LocalArtifacts: []LocalArtifactRef{}, Metadata: map[string]string{}}
	result, err := adapter.CheckStatus(context.Background(), StatusRequest{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if result.Missing == nil {
		t.Fatal("expected missing")
	}
}

func TestCLIAdapterInvalidRequestDTOFailsBeforeRunner(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("{}"), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{}
	_, err = adapter.PlanEnvironment(context.Background(), PlanEnvironmentRequest{Spec: spec})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !IsCode(err, ErrorCodeValidationFailed) {
		t.Fatalf("expected validation_failed, got %v", err)
	}
}

func TestCLIAdapterInvalidResponseDTOReturnsValidationFailed(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte(`{"supported_sources":[""]}`), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !IsCode(err, ErrorCodeValidationFailed) {
		t.Fatalf("expected validation_failed, got %v", err)
	}
}

func TestCLIAdapterMalformedJSONReturnsInternalError(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("{invalid}"), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, ErrorCodeInternalError) {
		t.Fatalf("expected internal_error, got %v", err)
	}
}

func TestCLIAdapterNonZeroExitReturnsExternalCommandFailed(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("{}"), Stderr: []byte("error"), ExitCode: 1}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, ErrorCodeExternalCommandFailed) {
		t.Fatalf("expected external_command_failed, got %v", err)
	}
}

func TestCLIAdapterTimeoutReturnsTimeout(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{TimedOut: true}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner, Timeout: 1 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, ErrorCodeTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
	var adapterErr AdapterError
	if errors.As(err, &adapterErr) && !adapterErr.IsRetryable() {
		t.Fatal("timeout should be retryable")
	}
}

func TestCLIAdapterOutputTooLargeReturnsError(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: make([]byte, 100), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner, MaxOutputBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected error for too large output")
	}
	if !IsCode(err, ErrorCodeIOError) {
		t.Fatalf("expected io_error, got %v", err)
	}
}

func TestCLIAdapterDefaultsTimeout(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("{}"), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.timeout != 30*time.Second {
		t.Fatalf("expected 30s default timeout, got %v", adapter.timeout)
	}
}

func TestCLIAdapterDefaultsMaxOutputBytes(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{Stdout: []byte("{}"), ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.maxOutputBytes != 10*1024*1024 {
		t.Fatalf("expected 10MB default, got %d", adapter.maxOutputBytes)
	}
}

func TestCLIAdapterNoopAdapterUnchanged(t *testing.T) {
	noop := NoopAdapter{}
	caps, err := noop.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.SupportedSources == nil {
		t.Fatal("NoopAdapter behavior changed")
	}
}

func TestCLIAdapterWithFakeRunnerStillWorks(t *testing.T) {
	caps := Capabilities{SupportsPlan: true, SupportedSources: []string{"test"}, SupportedLoaders: []string{}, Metadata: map[string]string{}}
	stdout, _ := json.Marshal(caps)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.SupportsPlan {
		t.Fatal("expected SupportsPlan true")
	}
}

func TestCLIAdapterWithUseExecBuildsWithoutExecuting(t *testing.T) {
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", UseExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if adapter == nil {
		t.Fatal("expected adapter")
	}
}

func TestCLIAdapterWithoutRunnerWithoutUseExecFails(t *testing.T) {
	_, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestCLIAdapterWithUseExecButEmptyCommandPathFails(t *testing.T) {
	_, err := NewCLIAdapter(CLIAdapterOptions{UseExec: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}
