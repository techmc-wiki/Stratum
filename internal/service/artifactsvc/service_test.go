package artifactsvc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/repository/memory"
	"github.com/stratummc/stratum/internal/service/artifactstagingsvc"
)

func TestApproveArtifactUpdatesReviewMetadataAndAudit(t *testing.T) {
	store := artifactReviewStore(t, artifact.StatusPending)
	service := New(store)
	service.now = fixedReviewTime
	service.newID = fixedReviewID
	value, err := service.ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "trusted test artifact")
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != artifact.StatusApproved || value.ReviewedBy != "reviewer-1" || value.ReviewedAt == nil || !value.ReviewedAt.Equal(fixedReviewTime()) || value.ReviewReason != "trusted test artifact" {
		t.Fatalf("artifact=%+v", value)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionApproved || events[0].Metadata["previousStatus"] != string(artifact.StatusPending) || events[0].Metadata["nextStatus"] != string(artifact.StatusApproved) || events[0].Metadata["reason"] != "trusted test artifact" {
		t.Fatalf("events=%+v", events)
	}
}

func TestRejectArtifactUpdatesReviewMetadataAndAudit(t *testing.T) {
	store := artifactReviewStore(t, artifact.StatusPending)
	service := New(store)
	service.now = fixedReviewTime
	value, err := service.RejectArtifact(context.Background(), "artifact-1", "reviewer-1", "unsafe payload")
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != artifact.StatusRejected || value.ReviewedBy != "reviewer-1" || value.ReviewedAt == nil || value.ReviewReason != "unsafe payload" {
		t.Fatalf("artifact=%+v", value)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionRejected || events[0].Metadata["nextStatus"] != string(artifact.StatusRejected) {
		t.Fatalf("events=%+v", events)
	}
}

func TestArtifactReviewValidationAndStateRules(t *testing.T) {
	service := New(artifactReviewStore(t, artifact.StatusPending))
	if _, err := service.ApproveArtifact(context.Background(), "artifact-1", "", "reason"); err == nil || !strings.Contains(err.Error(), "actor") {
		t.Fatalf("actor err=%v", err)
	}
	if _, err := service.ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", " "); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("reason err=%v", err)
	}
	if _, err := service.RejectArtifact(context.Background(), "artifact-1", "", "reason"); err == nil || !strings.Contains(err.Error(), "actor") {
		t.Fatalf("reject actor err=%v", err)
	}
	if _, err := service.RejectArtifact(context.Background(), "artifact-1", "reviewer-1", " "); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("reject reason err=%v", err)
	}
	if _, err := service.ApproveArtifact(context.Background(), "missing", "reviewer-1", "reason"); err == nil || !strings.Contains(err.Error(), "load artifact") {
		t.Fatalf("missing err=%v", err)
	}
	for _, status := range []artifact.Status{artifact.StatusApproved, artifact.StatusRejected, artifact.StatusDeprecated} {
		t.Run(string(status), func(t *testing.T) {
			store := artifactReviewStore(t, status)
			if _, err := New(store).ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "reason"); err == nil || !strings.Contains(err.Error(), "cannot transition") {
				t.Fatalf("approve err=%v", err)
			}
			if _, err := New(store).RejectArtifact(context.Background(), "artifact-1", "reviewer-1", "reason"); err == nil || !strings.Contains(err.Error(), "cannot transition") {
				t.Fatalf("reject err=%v", err)
			}
		})
	}
}

func TestApprovedArtifactCanCreateStagingPlanAfterReview(t *testing.T) {
	store := artifactReviewStore(t, artifact.StatusPending)
	_, pendingErr := artifactstagingsvc.New(store).CreatePlan(context.Background(), artifactstagingsvc.CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
	if pendingErr != nil {
		t.Fatal(pendingErr)
	}
	plans, _ := store.ListArtifactStagingPlans(context.Background())
	if len(plans) != 1 || plans[0].Status != artifactstaging.StatusRejected {
		t.Fatalf("pending plans=%+v", plans)
	}
	if _, err := New(store).ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "trusted"); err != nil {
		t.Fatal(err)
	}
	plan, err := artifactstagingsvc.New(store).CreatePlan(context.Background(), artifactstagingsvc.CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != artifactstaging.StatusPlanned {
		t.Fatalf("plan=%+v", plan)
	}
}

func artifactReviewStore(t *testing.T, status artifact.Status) *memory.Store {
	t.Helper()
	store := memory.New()
	now := fixedReviewTime()
	_ = store.SaveArtifact(context.Background(), artifact.Artifact{ID: "artifact-1", Name: "Test Artifact", Type: artifact.TypeJar, UploaderID: "uploader-1", SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, Status: status, CreatedAt: now})
	_ = store.SaveSession(context.Background(), session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now})
	return store
}

func fixedReviewTime() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) }

func fixedReviewID(prefix string) (string, error) { return prefix + "-fixed", nil }
