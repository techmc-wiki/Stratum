package lucy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type fakeRunner struct {
	result CommandResult
	err    error
	req    CommandRequest
}

func (f *fakeRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	f.req = req
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

func TestCLIAdapterInstallPackages(t *testing.T) {
	targetDir := t.TempDir()
	workDir := t.TempDir()
	content := []byte("mod jar")
	hashBytes := sha256.Sum256(content)
	hash := hex.EncodeToString(hashBytes[:])
	installedPath := filepath.Join(targetDir, "carpet-1.4.83.jar")
	if err := os.WriteFile(installedPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	response := InstallPackagesResult{
		Installed: []InstalledPackage{{ID: "fabric/carpet", Name: "carpet", Version: "1.4.83", Path: installedPath, Hash: hash, Size: int64(len(content))}},
		Failed:    []FailedPackage{},
		Status:    "ok",
		TotalSize: int64(len(content)),
	}
	stdout, _ := json.Marshal(response)
	runner := &fakeRunner{result: CommandResult{Stdout: stdout, ExitCode: 0}}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.InstallPackages(context.Background(), InstallPackagesRequest{
		Packages:  []LockedPackage{{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: hash, Size: int64(len(content))}},
		TargetDir: targetDir,
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || len(result.Installed) != 1 || len(result.Failed) != 0 {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if result.Installed[0].Path != installedPath {
		t.Fatalf("installed path: got %q, want %q", result.Installed[0].Path, installedPath)
	}
	if runner.req.WorkingDir != workDir {
		t.Fatalf("working dir: got %q, want %q", runner.req.WorkingDir, workDir)
	}
	if !reflect.DeepEqual(runner.req.Args, []string{"install", "--json"}) {
		t.Fatalf("args: got %#v", runner.req.Args)
	}
}

func TestCLIAdapterVerifyIntegrity(t *testing.T) {
	targetDir := t.TempDir()
	workDir := t.TempDir()
	content := []byte("mod jar")
	hashBytes := sha256.Sum256(content)
	hash := hex.EncodeToString(hashBytes[:])
	if err := os.WriteFile(filepath.Join(targetDir, "carpet-1.4.83.jar"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "lucy-lock.yaml")
	lock := EnvironmentLock{
		LockID:      "lock-1",
		LockHash:    "lockhash",
		GeneratedAt: time.Now().UTC(),
		Packages: []LockedPackage{
			{ID: "fabric/carpet", Source: "modrinth", Name: "carpet", Version: "1.4.83", Hash: hash, Size: int64(len(content))},
		},
		ProviderMetadata: map[string]string{},
	}
	data, _ := json.Marshal(lock)
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewCLIAdapter(CLIAdapterOptions{CommandPath: "lucy", Runner: &fakeRunner{result: CommandResult{Stdout: []byte(`{}`), ExitCode: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.VerifyIntegrity(context.Background(), IntegrityRequest{LockPath: lockPath, ModsDir: targetDir})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Status != "ok" || result.Checked != 1 {
		t.Fatalf("unexpected integrity result: %#v", result)
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
