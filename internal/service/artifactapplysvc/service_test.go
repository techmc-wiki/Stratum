package artifactapplysvc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/repository/memory"
)

func TestCreatePlanPersistsApplyPlanAndAudit(t *testing.T) {
	store := applyStore(t)
	service := New(store, validMaterializationVerifier{})
	service.now = fixedApplyTime
	service.newID = fixedApplyID

	plan, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", StagingPlanID: "staging-1", ActorID: "actor-1", TargetPath: "mods/test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != "artifact-apply-plan-fixed" || plan.Status != "planned" || plan.TargetRelativePath != "mods/test.jar" || plan.ValidationStatus != "validated" {
		t.Fatalf("plan=%+v", plan)
	}

	plans, err := store.ListArtifactApplyPlansBySession(context.Background(), "session-1")
	if err != nil || len(plans) != 1 || plans[0].ID != plan.ID {
		t.Fatalf("plans=%+v err=%v", plans, err)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil || len(events) != 1 || events[0].Action != ActionPlanCreated || events[0].Metadata["planId"] != plan.ID {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}

func TestCreatePlanReturnsPersistenceErrors(t *testing.T) {
	store := applyStore(t)
	store.ApplyPlans["artifact-apply-plan-fixed"] = store.ApplyPlans["existing"]
	service := New(store, validMaterializationVerifier{})
	service.newID = fixedApplyID

	_, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", StagingPlanID: "staging-1", ActorID: "actor-1", TargetPath: "mods/test.jar"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
}

func TestCreatePlanReturnsIDAndAuditErrors(t *testing.T) {
	t.Run("plan id", func(t *testing.T) {
		service := New(applyStore(t), validMaterializationVerifier{})
		service.newID = func(string) (string, error) { return "", errors.New("id unavailable") }

		_, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", StagingPlanID: "staging-1", ActorID: "actor-1", TargetPath: "mods/test.jar"})
		if err == nil || !strings.Contains(err.Error(), "id unavailable") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("audit", func(t *testing.T) {
		repository := failingAuditRepository{Repository: applyStore(t)}
		service := New(repository, validMaterializationVerifier{})
		service.newID = fixedApplyID

		_, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", StagingPlanID: "staging-1", ActorID: "actor-1", TargetPath: "mods/test.jar"})
		if err == nil || !strings.Contains(err.Error(), "audit unavailable") {
			t.Fatalf("err=%v", err)
		}
	})
}

type validMaterializationVerifier struct{}

func (validMaterializationVerifier) VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
	return agent.MaterializedArtifactsVerification{Entries: []agent.MaterializedArtifactVerification{{StagingPlanID: "staging-1", ArtifactID: "artifact-1", TargetName: "test.jar", ExpectedHash: artifact.HashBytes([]byte("artifact")), Status: "valid"}}}, nil
}

type failingAuditRepository struct{ Repository }

func (failingAuditRepository) AppendAuditEvent(context.Context, audit.Event) error {
	return errors.New("audit unavailable")
}

func applyStore(t *testing.T) *memory.Store {
	t.Helper()
	store := memory.New()
	now := fixedApplyTime()
	if err := store.SaveSession(context.Background(), session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveArtifact(context.Background(), artifact.Artifact{ID: "artifact-1", Name: "Test Artifact", Type: artifact.TypeJar, UploaderID: "uploader-1", SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, PayloadStatus: artifact.PayloadAvailable, PayloadAlgorithm: "sha256", PayloadReference: "sha256/test", Status: artifact.StatusApproved, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifactStagingPlan(context.Background(), artifactstaging.Plan{ID: "staging-1", SessionID: "session-1", ProjectID: "project-1", RoomID: "room-1", ArtifactID: "artifact-1", ArtifactName: "Test Artifact", ArtifactType: string(artifact.TypeJar), ArtifactStatus: string(artifact.StatusApproved), ArtifactHash: artifact.HashBytes([]byte("artifact")), TargetStagingName: "test.jar", StagingKind: artifactstaging.KindArtifact, ActorID: "actor-1", CreatedAt: now, Status: artifactstaging.StatusPlanned}); err != nil {
		t.Fatal(err)
	}
	return store
}

func fixedApplyTime() time.Time { return time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC) }

func fixedApplyID(prefix string) (string, error) { return prefix + "-fixed", nil }
