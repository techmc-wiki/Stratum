package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestSessionReadyForStartAfterEnvironmentMaterialization(t *testing.T) {
	supervisor := readinessSupervisor(t)
	materializeReadinessEnvironment(t, supervisor, false)
	result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
	if err != nil || !result.Ready || result.Status != "ready" || len(result.Issues) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !result.RuntimeStatusSummary.EnvironmentManifestExists || result.RuntimeStatusSummary.EnvironmentManifestStatus != "prepared" || result.RuntimeStatusSummary.ProcessState != string(StatusNotStarted) {
		t.Fatalf("summary=%+v", result.RuntimeStatusSummary)
	}
}

func TestSessionReadyForStartMissingRuntimeLayout(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *Supervisor)
		code  string
	}{
		{name: "runtime root", setup: func(t *testing.T, supervisor *Supervisor) { t.Helper(); _ = os.RemoveAll(supervisor.runtimeRoot) }, code: "runtime_root_missing"},
		{name: "session root", setup: func(t *testing.T, _ *Supervisor) { t.Helper() }, code: "session_root_missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			supervisor := readinessSupervisor(t)
			test.setup(t, supervisor)
			result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
			if err != nil || result.Ready || !readinessHasIssue(result, test.code) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestSessionReadyForStartEnvironmentManifestFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
		code   string
	}{
		{name: "missing", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, code: "environment_manifest_missing"},
		{name: "malformed", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("not-json"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, code: "environment_manifest_malformed"},
		{name: "not prepared", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(`{"status":"pending"}`), 0o640); err != nil {
				t.Fatal(err)
			}
		}, code: "environment_not_prepared"},
	} {
		t.Run(test.name, func(t *testing.T) {
			supervisor := readinessSupervisor(t)
			materializeReadinessEnvironment(t, supervisor, false)
			path := filepath.Join(supervisor.runtimeRoot, "sessions", "session-1", "config", "environment-materialization.json")
			test.mutate(t, path)
			result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
			if err != nil || result.Ready || !readinessHasIssue(result, test.code) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestSessionReadyForStartRejectsActiveAndCrashedProcess(t *testing.T) {
	for _, test := range []struct {
		name  string
		crash bool
		code  string
	}{
		{name: "running", code: "process_active"},
		{name: "crashed", crash: true, code: "process_crashed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			supervisor := readinessSupervisor(t)
			materializeReadinessEnvironment(t, supervisor, false)
			if _, err := supervisor.StartProcess(context.Background(), "session-1", runtimeprofile.DummyProcess()); err != nil {
				t.Fatal(err)
			}
			if test.crash {
				if _, err := supervisor.MarkCrashed("session-1", "test crash"); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Cleanup(func() { _, _ = supervisor.StopProcess(context.Background(), "session-1") })
			}
			result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
			if err != nil || result.Ready || !readinessHasIssue(result, test.code) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestSessionReadyForStartAppliedArtifactVerification(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
		ready  bool
		code   string
	}{
		{name: "valid", mutate: func(*testing.T, string) {}, ready: true},
		{name: "missing", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, code: "applied_artifact_missing"},
		{name: "corrupted", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("corrupted"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, code: "applied_artifact_corrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			supervisor := readinessSupervisor(t)
			materializeReadinessEnvironment(t, supervisor, false)
			target := applyReadinessArtifact(t, supervisor)
			test.mutate(t, target)
			result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
			if err != nil || result.Ready != test.ready || (!test.ready && !readinessHasIssue(result, test.code)) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if result.RuntimeStatusSummary.AppliedArtifactsTotal != 1 {
				t.Fatalf("summary=%+v", result.RuntimeStatusSummary)
			}
		})
	}
}

func TestSessionReadyForStartRequiresMCDRLayoutWhenConfigured(t *testing.T) {
	supervisor := readinessSupervisor(t)
	materializeReadinessEnvironment(t, supervisor, true)
	result, err := supervisor.SessionReadyForStart(context.Background(), "session-1")
	if err != nil || result.Ready || !readinessHasIssue(result, "mcdr_layout_missing") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSessionReadyForStartRejectsUnsafeSessionID(t *testing.T) {
	supervisor := readinessSupervisor(t)
	if _, err := supervisor.SessionReadyForStart(context.Background(), "../escape"); err == nil {
		t.Fatal("unsafe session ID was accepted")
	}
}

func readinessSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	supervisor, err := NewSupervisorWithRoot("test-agent", t.TempDir(), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	return supervisor
}

func materializeReadinessEnvironment(t *testing.T, supervisor *Supervisor, mcdrRequired bool) {
	t.Helper()
	_, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{SessionID: "session-1", EnvironmentID: "environment-1", EnvironmentName: "Test", MinecraftVersion: "1.17.1", JavaVersion: "17", LoaderType: "fabric", ServerCore: "carpet", MCDRRequired: mcdrRequired, RuntimeProfileID: runtimeprofile.DefaultProfileID, RuntimeProfileRequired: true, ActorID: "actor-1"})
	if err != nil {
		t.Fatal(err)
	}
}

func applyReadinessArtifact(t *testing.T, supervisor *Supervisor) string {
	t.Helper()
	payload := []byte("readiness artifact")
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	request := agent.ArtifactMaterializationRequest{SessionID: "session-1", ArtifactID: "artifact-1", StagingPlanID: "staging-1", ArtifactName: "Artifact", ArtifactType: "jar", TargetName: "nested/test.jar", PayloadAlgorithm: "sha256", PayloadHash: hash, PayloadSize: int64(len(payload)), ActorID: "actor-1", Payload: payload}
	if _, err := MaterializeArtifact(context.Background(), supervisor.runtimeRoot, request, time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteArtifactApply(context.Background(), supervisor.runtimeRoot, agent.ArtifactApplyExecuteRequest{ApplyPlanID: "apply-1", SessionID: "session-1", StagingPlanID: "staging-1", ArtifactID: "artifact-1", TargetRoot: "mods", TargetRelativePath: "test.jar", ExpectedHash: hash, ExpectedSize: int64(len(payload))}, time.Now())
	if err != nil || result.Status != "applied" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	return result.TargetPath
}

func readinessHasIssue(result agent.SessionStartReadiness, code string) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
