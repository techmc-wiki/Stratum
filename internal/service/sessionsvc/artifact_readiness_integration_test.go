package sessionsvc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/room"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/repository/artifactblob"
	"github.com/stratummc/stratum/internal/repository/filesystem"
	"github.com/stratummc/stratum/internal/service/artifactapplysvc"
	"github.com/stratummc/stratum/internal/service/artifactstagingsvc"
	"github.com/stratummc/stratum/internal/service/artifactsvc"
	"github.com/stratummc/stratum/internal/service/sessionsvc"
)

const artifactLifecycleSessionID = "artifact-lifecycle-session"

type preStartGate struct {
	service *artifactstagingsvc.PreStartService
}

func (g preStartGate) Check(ctx context.Context, sessionID string) (map[string]string, error) {
	result, err := g.service.Check(ctx, sessionID)
	return result.Metadata(), err
}

type artifactLifecycleFixture struct {
	ctx        context.Context
	store      *filesystem.Store
	agent      agent.AgentClient
	sessions   *sessionsvc.Service
	targetPath string
}

func TestArtifactApplyReadinessAllowsSessionStart(t *testing.T) {
	fixture := newArtifactLifecycleFixture(t)

	verification, err := fixture.agent.VerifyAllAppliedArtifacts(fixture.ctx, artifactLifecycleSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Total != 1 || verification.ValidCount != 1 {
		t.Fatalf("verification = %+v", verification)
	}

	value, _, err := fixture.sessions.StartWithOptions(fixture.ctx, artifactLifecycleSessionID, "operator", sessionsvc.OperationOptions{})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if value.Status != operation.StatusSucceeded || value.Metadata["appliedVerifyStatus"] != "valid" || value.Metadata["validApplied"] != "1" {
		t.Fatalf("operation = %+v", value)
	}
	stored, err := fixture.store.GetSession(fixture.ctx, artifactLifecycleSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != session.StateRunning {
		t.Fatalf("state = %s, want running", stored.State)
	}
}

func TestArtifactApplyReadinessBlocksMissingOrCorruptedTarget(t *testing.T) {
	for _, test := range []struct {
		name          string
		breakTarget   func(*testing.T, string)
		metadataKey   string
		metadataValue string
	}{
		{name: "missing", breakTarget: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, metadataKey: "missingApplied", metadataValue: "1"},
		{name: "corrupted", breakTarget: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("corrupted"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, metadataKey: "corruptedApplied", metadataValue: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArtifactLifecycleFixture(t)
			test.breakTarget(t, fixture.targetPath)

			value, _, err := fixture.sessions.StartWithOptions(fixture.ctx, artifactLifecycleSessionID, "operator", sessionsvc.OperationOptions{})
			if err == nil {
				t.Fatal("start succeeded with an invalid applied artifact")
			}
			if value.Status != operation.StatusFailed || value.Metadata["appliedVerifyStatus"] != "not_ready" || value.Metadata[test.metadataKey] != test.metadataValue {
				t.Fatalf("operation = %+v", value)
			}
			stored, loadErr := fixture.store.GetSession(fixture.ctx, artifactLifecycleSessionID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if stored.State == session.StateRunning {
				t.Fatal("blocked session became running")
			}
			assertArtifactFailureAudit(t, fixture, test.metadataKey, test.metadataValue)
		})
	}
}

func newArtifactLifecycleFixture(t *testing.T) artifactLifecycleFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	store, err := filesystem.New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.CreateEnvironment(ctx, environment.Environment{ID: "environment-1", Name: "Test", MinecraftVersion: "1.17.1", LoaderType: environment.LoaderFabric, ServerCore: environment.ServerCarpet, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProject(ctx, project.Project{ID: "project-1", Name: "Project", Members: []project.Member{}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRoom(ctx, room.Room{ID: "room-1", ProjectID: "project-1", Name: "Room", EnvironmentID: "environment-1", BaseWorldRef: "base-world:test", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, session.Session{ID: artifactLifecycleSessionID, ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "environment-1", CreatedAt: now, LastActiveAt: now}); err != nil {
		t.Fatal(err)
	}

	blobs, err := artifactblob.New(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact lifecycle integration payload")
	payloadPath := filepath.Join(root, "payload.jar")
	if err := os.WriteFile(payloadPath, payload, 0o640); err != nil {
		t.Fatal(err)
	}
	artifactService := artifactsvc.NewWithBlobStore(store, blobs)
	if _, err := artifactService.CreateMetadata(ctx, "artifact-1", "Test Artifact", artifact.TypeJar, "project-1", "uploader"); err != nil {
		t.Fatal(err)
	}
	imported, err := artifactService.ImportFile(ctx, "artifact-1", payloadPath, "uploader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := artifactService.ApproveArtifact(ctx, "artifact-1", "reviewer", "integration test approval"); err != nil {
		t.Fatal(err)
	}

	stagingPlan, err := artifactstagingsvc.NewWithPayloadVerifier(store, blobs).CreatePlan(ctx, artifactstagingsvc.CreateParams{SessionID: artifactLifecycleSessionID, ArtifactID: "artifact-1", ActorID: "uploader", Name: "nested/test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, filepath.Join(root, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeAgent.MaterializeArtifact(ctx, agent.ArtifactMaterializationRequest{SessionID: artifactLifecycleSessionID, ArtifactID: imported.ID, StagingPlanID: stagingPlan.ID, ArtifactName: imported.Name, ArtifactType: string(imported.Type), TargetName: stagingPlan.TargetStagingName, PayloadAlgorithm: imported.PayloadAlgorithm, PayloadHash: imported.SHA256, PayloadSize: imported.SizeBytes, ActorID: "uploader", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	applyPlan, err := artifactapplysvc.New(store, runtimeAgent).CreatePlan(ctx, artifactapplysvc.CreateParams{SessionID: artifactLifecycleSessionID, StagingPlanID: stagingPlan.ID, ActorID: "operator", TargetPath: "test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.ArtifactApplyDryRunRequest{ApplyPlanID: applyPlan.ID, SessionID: applyPlan.SessionID, StagingPlanID: applyPlan.SourceStagingPlanID, ArtifactID: applyPlan.ArtifactID, TargetRoot: string(applyPlan.TargetRoot), TargetRelativePath: applyPlan.TargetRelativePath, ExpectedHash: applyPlan.MaterializedArtifactHash, ExpectedSize: imported.SizeBytes}
	dryRun, err := runtimeAgent.DryRunArtifactApply(ctx, request)
	if err != nil || dryRun.Status != "ready" {
		t.Fatalf("dry-run = %+v, err = %v", dryRun, err)
	}
	executed, err := runtimeAgent.ExecuteArtifactApply(ctx, agent.ArtifactApplyExecuteRequest(request))
	if err != nil || executed.Status != "applied" {
		t.Fatalf("execute = %+v, err = %v", executed, err)
	}

	gate := preStartGate{service: artifactstagingsvc.NewPreStartService(store, blobs, runtimeAgent)}
	sessionService := sessionsvc.New(store, resourcepolicy.MVPDefault(), runtimeAgent).WithArtifactReadinessGate(gate)
	t.Cleanup(func() {
		_, _ = runtimeAgent.StopSession(context.Background(), agent.SessionRequest{SessionID: artifactLifecycleSessionID})
	})
	return artifactLifecycleFixture{ctx: ctx, store: store, agent: runtimeAgent, sessions: sessionService, targetPath: executed.TargetPath}
}

func assertArtifactFailureAudit(t *testing.T, fixture artifactLifecycleFixture, key, value string) {
	t.Helper()
	events, err := fixture.store.ListAuditEvents(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.TargetType == "session" && event.TargetID == artifactLifecycleSessionID && event.Metadata["result"] == "failure" && event.Metadata[key] == value {
			return
		}
	}
	t.Fatalf("artifact readiness failure audit with %s=%s not found", key, value)
}
