package lucy

import (
	"context"
	"errors"
	"testing"
)

type fakeBackend struct {
	caps          Capabilities
	plan          EnvironmentPlan
	lock          EnvironmentLock
	status        EnvironmentStatus
	installResult InstallPackagesResult
	installErr    error
	integrity     IntegrityResult
	integrityErr  error
	err           error
}

func (f *fakeBackend) Capabilities(_ context.Context) (Capabilities, error) {
	return f.caps, f.err
}

func (f *fakeBackend) Plan(_ context.Context, _ EnvironmentSpec) (EnvironmentPlan, error) {
	return f.plan, f.err
}

func (f *fakeBackend) Lock(_ context.Context, _ EnvironmentSpec) (EnvironmentLock, error) {
	return f.lock, f.err
}

func (f *fakeBackend) Status(_ context.Context, _ EnvironmentSpec, _ *EnvironmentLock) (EnvironmentStatus, error) {
	return f.status, f.err
}

func (f *fakeBackend) Install(_ context.Context, _ InstallPackagesRequest) (InstallPackagesResult, error) {
	if f.installErr != nil {
		return InstallPackagesResult{}, f.installErr
	}
	return f.installResult, f.err
}

func (f *fakeBackend) VerifyIntegrity(_ context.Context, _ IntegrityRequest) (IntegrityResult, error) {
	if f.integrityErr != nil {
		return IntegrityResult{}, f.integrityErr
	}
	return f.integrity, f.err
}

func TestEmbeddedAdapterRequiresBackend(t *testing.T) {
	_, err := NewEmbeddedAdapter(nil)
	if err == nil {
		t.Fatal("expected error when backend nil")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestEmbeddedAdapterCapabilitiesCallsBackendAndValidates(t *testing.T) {
	caps := Capabilities{SupportsPlan: true, SupportedSources: []string{"test"}, SupportedLoaders: []string{}, Metadata: map[string]string{}}
	backend := &fakeBackend{caps: caps}
	adapter, err := NewEmbeddedAdapter(backend)
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

func TestEmbeddedAdapterPlanValidatesRequestBeforeBackend(t *testing.T) {
	backend := &fakeBackend{}
	adapter, err := NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{}
	_, err = adapter.PlanEnvironment(context.Background(), PlanEnvironmentRequest{Spec: spec})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestEmbeddedAdapterInvalidBackendResponseReturnsValidationFailed(t *testing.T) {
	backend := &fakeBackend{caps: Capabilities{SupportedSources: []string{""}, SupportedLoaders: []string{}}}
	adapter, err := NewEmbeddedAdapter(backend)
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

func TestEmbeddedAdapterBackendAdapterErrorPreservesCode(t *testing.T) {
	aerr := NewAdapterError(ErrorCodeTimeout, "timeout", nil, true)
	backend := &fakeBackend{err: aerr}
	adapter, err := NewEmbeddedAdapter(backend)
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
}

func TestEmbeddedAdapterOrdinaryBackendErrorClassifiesAsInternalError(t *testing.T) {
	backend := &fakeBackend{err: errors.New("backend failure")}
	adapter, err := NewEmbeddedAdapter(backend)
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

func TestEmbeddedAdapterLockValidatesSpec(t *testing.T) {
	backend := &fakeBackend{}
	adapter, err := NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{}
	_, err = adapter.LockEnvironment(context.Background(), LockEnvironmentRequest{Spec: spec})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestEmbeddedAdapterStatusValidatesSpecAndLock(t *testing.T) {
	backend := &fakeBackend{}
	adapter, err := NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	spec := EnvironmentSpec{EnvironmentID: "env-1", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet", Packages: []PackageRef{}, LocalArtifacts: []LocalArtifactRef{}, Metadata: map[string]string{}}
	lock := EnvironmentLock{LockID: "   ", LockHash: "   "}
	_, err = adapter.CheckStatus(context.Background(), StatusRequest{Spec: spec, Lock: &lock})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !IsCode(err, ErrorCodeInvalidRequest) {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestEmbeddedAdapterVerifyIntegrity(t *testing.T) {
	backend := &fakeBackend{integrity: IntegrityResult{OK: true, Status: "ok", Missing: []string{}, Corrupt: []string{}, Checked: 1}}
	adapter, err := NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.VerifyIntegrity(context.Background(), IntegrityRequest{LockPath: "lock.json", ModsDir: "mods"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Status != "ok" || result.Checked != 1 {
		t.Fatalf("unexpected integrity result: %#v", result)
	}
}
