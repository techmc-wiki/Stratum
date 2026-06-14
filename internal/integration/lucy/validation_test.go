package lucy

import (
	"context"
	"testing"
)

func TestEnvironmentSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    EnvironmentSpec
		wantErr bool
	}{
		{name: "valid", spec: validEnvironmentSpec()},
		{name: "missing environment id", spec: EnvironmentSpec{MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet"}, wantErr: true},
		{name: "unsafe environment id", spec: EnvironmentSpec{EnvironmentID: "../escape", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet"}, wantErr: true},
		{name: "missing minecraft version", spec: EnvironmentSpec{EnvironmentID: "gtmc-1.17", LoaderType: "fabric", ServerCore: "carpet"}, wantErr: true},
		{name: "missing loader", spec: EnvironmentSpec{EnvironmentID: "gtmc-1.17", MinecraftVersion: "1.17.1", ServerCore: "carpet"}, wantErr: true},
		{name: "missing server core", spec: EnvironmentSpec{EnvironmentID: "gtmc-1.17", MinecraftVersion: "1.17.1", LoaderType: "fabric"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.spec.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestPackageRefValidateRejectsInvalidValues(t *testing.T) {
	for _, ref := range []PackageRef{
		{Source: "modrinth", Name: "Fabric API"},
		{ID: "../fabric-api", Source: "modrinth", Name: "Fabric API"},
		{ID: "fabric-api", Name: "Fabric API"},
		{ID: "fabric-api", Source: "modrinth"},
	} {
		if err := ref.Validate(); err == nil {
			t.Fatalf("invalid package passed validation: %+v", ref)
		}
	}
}

func TestLocalArtifactRefValidateRejectsUnsafeRuntimeName(t *testing.T) {
	tests := []string{"../escape.jar", `..\escape.jar`, "/absolute.jar", `C:\absolute.jar`, "nested/../../escape.jar"}
	for _, runtimeName := range tests {
		t.Run(runtimeName, func(t *testing.T) {
			ref := validLocalArtifactRef()
			ref.RuntimeName = runtimeName
			if err := ref.Validate(); err == nil {
				t.Fatalf("unsafe runtime name passed validation: %q", runtimeName)
			}
		})
	}
}

func TestLocalArtifactRefValidateRequiresPairedPayloadMetadata(t *testing.T) {
	tests := []LocalArtifactRef{
		{ArtifactID: "artifact-1", PayloadAlgorithm: "sha256", ArtifactType: "jar"},
		{ArtifactID: "artifact-1", PayloadHash: "abc", ArtifactType: "jar"},
		{ArtifactID: "artifact-1", PayloadSize: -1, ArtifactType: "jar"},
		{ArtifactID: "artifact-1"},
	}
	for _, ref := range tests {
		if err := ref.Validate(); err == nil {
			t.Fatalf("invalid artifact passed validation: %+v", ref)
		}
	}
}

func TestPlanActionValidate(t *testing.T) {
	tests := []struct {
		name    string
		action  PlanAction
		wantErr bool
	}{
		{name: "valid", action: PlanAction{ActionType: ActionCopy, ArtifactID: "artifact-1", Target: "mods/example.jar", Hash: "abc", Size: 12}},
		{name: "invalid type", action: PlanAction{ActionType: "execute"}, wantErr: true},
		{name: "target traversal", action: PlanAction{ActionType: ActionCopy, Target: "../escape.jar"}, wantErr: true},
		{name: "windows absolute target", action: PlanAction{ActionType: ActionCopy, Target: `C:\escape.jar`}, wantErr: true},
		{name: "negative size", action: PlanAction{ActionType: ActionVerify, Size: -1}, wantErr: true},
		{name: "positive size without hash", action: PlanAction{ActionType: ActionDownload, Size: 12}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.action.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestNoopAdapterOutputsValidate(t *testing.T) {
	adapter := NoopAdapter{}
	ctx := context.Background()
	capabilities, err := adapter.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanEnvironment(ctx, PlanEnvironmentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	lock, err := adapter.LockEnvironment(ctx, LockEnvironmentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := adapter.CheckStatus(ctx, StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}

	for name, err := range map[string]error{
		"capabilities": capabilities.Validate(),
		"plan":         plan.Validate(),
		"lock":         lock.Validate(),
		"status":       status.Validate(),
	} {
		if err != nil {
			t.Fatalf("%s validation: %v", name, err)
		}
	}
}

func TestEnvironmentLockValidate(t *testing.T) {
	if err := (EnvironmentLock{}).Validate(); err != nil {
		t.Fatalf("zero lock should be valid: %v", err)
	}
	if err := (EnvironmentLock{LockHash: "sha256:abc"}).Validate(); err == nil {
		t.Fatal("non-zero lock without id should fail")
	}
}

func TestCapabilitiesValidateRejectsEmptyEntries(t *testing.T) {
	if err := (Capabilities{SupportedSources: []string{"modrinth", ""}}).Validate(); err == nil {
		t.Fatal("empty supported source should fail")
	}
	if err := (Capabilities{SupportedLoaders: []string{"fabric", " "}}).Validate(); err == nil {
		t.Fatal("empty supported loader should fail")
	}
}

func validEnvironmentSpec() EnvironmentSpec {
	return EnvironmentSpec{
		EnvironmentID: "gtmc-1.17", MinecraftVersion: "1.17.1", JavaVersion: "16",
		LoaderType: "fabric", LoaderVersion: "0.11.7", ServerCore: "carpet",
		CarpetRequired: true, MCDRRequired: true, RuntimeProfileID: "mcdr-managed",
		Packages:       []PackageRef{{ID: "fabric-api", Source: "modrinth", Name: "Fabric API", Required: true}},
		LocalArtifacts: []LocalArtifactRef{validLocalArtifactRef()},
	}
}

func validLocalArtifactRef() LocalArtifactRef {
	return LocalArtifactRef{
		ArtifactID: "artifact-1", PayloadAlgorithm: "sha256", PayloadHash: "abc",
		PayloadSize: 12, ArtifactType: "jar", RuntimeName: "mods/example.jar",
	}
}
