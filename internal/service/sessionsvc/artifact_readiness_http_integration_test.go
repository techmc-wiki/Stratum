package sessionsvc_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
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

const artifactHTTPStorageSessionID = "artifact-http-lifecycle-session"

type artifactHTTPFixture struct {
	ctx        context.Context
	store      *filesystem.Store
	agent      agent.AgentClient
	sessions   *sessionsvc.Service
	targetPath string
}

func TestHTTPArtifactApplyReadinessAllowsSessionStart(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)

	verification, err := fixture.agent.VerifyAllAppliedArtifacts(fixture.ctx, artifactHTTPStorageSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Total != 1 || verification.ValidCount != 1 || verification.Entries[0].Status != "valid" {
		t.Fatalf("verification = %+v", verification)
	}

	value, _, err := fixture.sessions.StartWithOptions(fixture.ctx, artifactHTTPStorageSessionID, "operator", sessionsvc.OperationOptions{})
	if err != nil {
		t.Fatalf("start session through HTTP Agent: %v", err)
	}
	if value.Status != operation.StatusSucceeded || value.Metadata["stagingReadinessStatus"] != "ready" || value.Metadata["appliedVerifyStatus"] != "valid" {
		t.Fatalf("operation = %+v", value)
	}
	stored, err := fixture.store.GetSession(fixture.ctx, artifactHTTPStorageSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != session.StateRunning {
		t.Fatalf("state = %s, want running", stored.State)
	}
}

func TestHTTPArtifactApplyReadinessBlocksCorruptedTarget(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)
	if err := os.WriteFile(fixture.targetPath, []byte("corrupted through test"), 0o640); err != nil {
		t.Fatal(err)
	}

	verification, err := fixture.agent.VerifyAllAppliedArtifacts(fixture.ctx, artifactHTTPStorageSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Total != 1 || verification.CorruptedCount != 1 || verification.Entries[0].Status != "corrupted" {
		t.Fatalf("verification = %+v", verification)
	}

	value, _, err := fixture.sessions.StartWithOptions(fixture.ctx, artifactHTTPStorageSessionID, "operator", sessionsvc.OperationOptions{})
	if err == nil {
		t.Fatal("start succeeded with a corrupted applied artifact")
	}
	if value.Status != operation.StatusFailed || value.Metadata["appliedVerifyStatus"] != "not_ready" || value.Metadata["corruptedApplied"] != "1" {
		t.Fatalf("operation = %+v", value)
	}
	stored, loadErr := fixture.store.GetSession(fixture.ctx, artifactHTTPStorageSessionID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if stored.State == session.StateRunning {
		t.Fatal("blocked session became running")
	}
	assertHTTPArtifactFailureAudit(t, fixture, "1", "0")
}

func newArtifactHTTPFixture(t *testing.T) artifactHTTPFixture {
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
	if err := store.CreateProject(ctx, project.Project{ID: "project-http", Name: "HTTP Project", Members: []project.Member{}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRoom(ctx, room.Room{ID: "room-http", ProjectID: "project-http", Name: "HTTP Room", EnvironmentID: "environment-1", BaseWorldRef: "base-world:test", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, session.Session{ID: artifactHTTPStorageSessionID, ProjectID: "project-http", RoomID: "room-http", OwnerUserID: "owner-http", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "environment-1", CreatedAt: now, LastActiveAt: now}); err != nil {
		t.Fatal(err)
	}

	blobs, err := artifactblob.New(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("artifact HTTP lifecycle payload")
	payloadPath := filepath.Join(root, "payload.jar")
	if err := os.WriteFile(payloadPath, payload, 0o640); err != nil {
		t.Fatal(err)
	}
	artifactService := artifactsvc.NewWithBlobStore(store, blobs)
	if _, err := artifactService.CreateMetadata(ctx, "artifact-http", "HTTP Artifact", artifact.TypeJar, "project-http", "uploader"); err != nil {
		t.Fatal(err)
	}
	imported, err := artifactService.ImportFile(ctx, "artifact-http", payloadPath, "uploader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := artifactService.ApproveArtifact(ctx, imported.ID, "reviewer", "HTTP integration approval"); err != nil {
		t.Fatal(err)
	}
	stagingPlan, err := artifactstagingsvc.NewWithPayloadVerifier(store, blobs).CreatePlan(ctx, artifactstagingsvc.CreateParams{SessionID: artifactHTTPStorageSessionID, ArtifactID: imported.ID, ActorID: "uploader", Name: "nested/http-test.jar"})
	if err != nil {
		t.Fatal(err)
	}

	runtimeAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, filepath.Join(root, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(runtimeAgent, "", nil).Handler())
	t.Cleanup(server.Close)
	httpAgent, err := httptransport.NewClient(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := httpAgent.MaterializeArtifact(ctx, agent.ArtifactMaterializationRequest{SessionID: artifactHTTPStorageSessionID, ArtifactID: imported.ID, StagingPlanID: stagingPlan.ID, ArtifactName: imported.Name, ArtifactType: string(imported.Type), TargetName: stagingPlan.TargetStagingName, PayloadAlgorithm: imported.PayloadAlgorithm, PayloadHash: imported.SHA256, PayloadSize: imported.SizeBytes, ActorID: "uploader", Payload: payload}); err != nil {
		t.Fatal(err)
	}

	applyPlan, err := artifactapplysvc.New(store, httpAgent).CreatePlan(ctx, artifactapplysvc.CreateParams{SessionID: artifactHTTPStorageSessionID, StagingPlanID: stagingPlan.ID, ActorID: "operator", TargetPath: "http-test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	dryRunRequest := agent.ArtifactApplyDryRunRequest{ApplyPlanID: applyPlan.ID, SessionID: applyPlan.SessionID, StagingPlanID: applyPlan.SourceStagingPlanID, ArtifactID: applyPlan.ArtifactID, TargetRoot: string(applyPlan.TargetRoot), TargetRelativePath: applyPlan.TargetRelativePath, ExpectedHash: applyPlan.MaterializedArtifactHash, ExpectedSize: imported.SizeBytes}
	dryRun, err := httpAgent.DryRunArtifactApply(ctx, dryRunRequest)
	if err != nil || dryRun.Status != "ready" {
		t.Fatalf("dry-run = %+v, err = %v", dryRun, err)
	}
	executed, err := httpAgent.ExecuteArtifactApply(ctx, agent.ArtifactApplyExecuteRequest(dryRunRequest))
	if err != nil || executed.Status != "applied" {
		t.Fatalf("execute = %+v, err = %v", executed, err)
	}

	gate := preStartGate{service: artifactstagingsvc.NewPreStartService(store, blobs, httpAgent)}
	sessionService := sessionsvc.New(store, resourcepolicy.MVPDefault(), httpAgent).WithArtifactReadinessGate(gate)
	t.Cleanup(func() {
		_, _ = httpAgent.StopSession(context.Background(), agent.SessionRequest{SessionID: artifactHTTPStorageSessionID})
	})
	return artifactHTTPFixture{ctx: ctx, store: store, agent: httpAgent, sessions: sessionService, targetPath: executed.TargetPath}
}

func TestHTTPArtifactApplyReadinessBlocksMissingTarget(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)
	if err := os.Remove(fixture.targetPath); err != nil {
		t.Fatal(err)
	}

	verification, err := fixture.agent.VerifyAllAppliedArtifacts(fixture.ctx, artifactHTTPStorageSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Total != 1 || verification.MissingCount != 1 || verification.Entries[0].Status != "missing" {
		t.Fatalf("verification = %+v", verification)
	}

	value, _, err := fixture.sessions.StartWithOptions(fixture.ctx, artifactHTTPStorageSessionID, "operator", sessionsvc.OperationOptions{})
	if err == nil {
		t.Fatal("start succeeded with a missing applied artifact")
	}
	if value.Status != operation.StatusFailed || value.Metadata["appliedVerifyStatus"] != "not_ready" || value.Metadata["missingApplied"] != "1" {
		t.Fatalf("operation = %+v", value)
	}
	stored, loadErr := fixture.store.GetSession(fixture.ctx, artifactHTTPStorageSessionID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if stored.State == session.StateRunning {
		t.Fatal("blocked session became running")
	}
	assertHTTPArtifactFailureAudit(t, fixture, "0", "1")
}

func TestHTTPArtifactReadinessAgentUnreachable(t *testing.T) {
	tmpDir := t.TempDir()
	runtimeRoot := filepath.Join(tmpDir, "runtime")
	dataDir := filepath.Join(tmpDir, "data")
	store, err := filesystem.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	proj := project.Project{ID: "proj", Name: "Test Project", Members: []project.Member{}, CreatedAt: now}
	_ = store.CreateProject(ctx, proj)
	r := room.Room{ID: "room", ProjectID: "proj", Name: "Test Room", EnvironmentID: "env-1", BaseWorldRef: "base:test", CreatedAt: now}
	_ = store.CreateRoom(ctx, r)
	sess := session.Session{ID: "unreachable-session", ProjectID: "proj", RoomID: "room", OwnerUserID: "user", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now}
	_ = store.CreateSession(ctx, sess)

	unreachableAgent, _ := httptransport.NewClient("http://127.0.0.1:1", "", 1*time.Second)
	blobs, _ := artifactblob.New(filepath.Join(runtimeRoot, "blobs"))
	gate := preStartGate{service: artifactstagingsvc.NewPreStartService(store, blobs, unreachableAgent)}
	sessionService := sessionsvc.New(store, resourcepolicy.MVPDefault(), unreachableAgent).WithArtifactReadinessGate(gate)

	value, _, err := sessionService.StartWithOptions(ctx, "unreachable-session", "operator", sessionsvc.OperationOptions{})
	if err == nil {
		t.Fatal("start succeeded with unreachable agent")
	}
	if value.Status != operation.StatusFailed {
		t.Fatalf("operation status = %s, want failed", value.Status)
	}
	stored, err := store.GetSession(ctx, "unreachable-session")
	if err != nil {
		t.Fatal(err)
	}
	if stored.State == session.StateRunning {
		t.Fatal("blocked session became running")
	}
}

func assertHTTPArtifactFailureAudit(t *testing.T, fixture artifactHTTPFixture, expectedCorrupted, expectedMissing string) {
	t.Helper()
	events, err := fixture.store.ListAuditEvents(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.TargetType == "session" && event.TargetID == artifactHTTPStorageSessionID && event.Metadata["result"] == "failure" {
			if event.Metadata["corruptedApplied"] == expectedCorrupted || event.Metadata["missingApplied"] == expectedMissing {
				return
			}
		}
	}
	t.Fatal("HTTP artifact readiness failure audit not found")
}
