package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/artifact"
	artifactstaging "github.com/stratummc/stratum/internal/artifact/staging"
	artifactstagingsvc "github.com/stratummc/stratum/internal/artifact/stagingservice"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/session"
	"github.com/stratummc/stratum/internal/storage/artifactblob"
	"github.com/stratummc/stratum/internal/storage/memory"
)

func TestImportFileStoresBlobUpdatesPendingArtifactAndAudits(t *testing.T) {
	store := artifactImportStore(t, artifact.StatusPending)
	blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	path := writeArtifactFile(t, "payload.jar", "hello artifact")
	service := NewWithBlobStore(store, blobs)
	service.now = fixedReviewTime
	service.newID = fixedReviewID
	value, err := service.ImportFile(context.Background(), "artifact-1", path, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != artifact.StatusPending || value.PayloadStatus != artifact.PayloadAvailable || value.PayloadAlgorithm != artifactblob.Algorithm || value.SHA256 == "" || value.SizeBytes != 14 || value.PayloadReference == "" || value.PayloadImportedBy != "actor-1" || value.PayloadImportedAt == nil || !value.PayloadImportedAt.Equal(fixedReviewTime()) {
		t.Fatalf("artifact=%+v", value)
	}
	if _, err := blobs.Verify(context.Background(), value.SHA256); err != nil {
		t.Fatal(err)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionPayloadImported || events[0].Metadata["artifactId"] != "artifact-1" || events[0].Metadata["artifactName"] != "Artifact" || events[0].Metadata["actor"] != "actor-1" || events[0].Metadata["payloadAlgorithm"] != "sha256" || events[0].Metadata["payloadHash"] != value.SHA256 || events[0].Metadata["payloadSize"] != "14" {
		t.Fatalf("events=%+v", events)
	}
}

func TestImportFileValidationIdempotencyAndConflict(t *testing.T) {
	store := artifactImportStore(t, artifact.StatusPending)
	blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithBlobStore(store, blobs)
	path := writeArtifactFile(t, "payload.jar", "same payload")
	first, err := service.ImportFile(context.Background(), "artifact-1", path, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, _ := store.ListAuditEvents(context.Background())
	second, err := service.ImportFile(context.Background(), "artifact-1", path, "actor-2")
	if err != nil || second.SHA256 != first.SHA256 || second.PayloadImportedBy != "actor-1" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	eventsAfter, _ := store.ListAuditEvents(context.Background())
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("duplicate import wrote audit: %d -> %d", len(eventsBefore), len(eventsAfter))
	}
	different := writeArtifactFile(t, "different.jar", "different payload")
	if _, err := service.ImportFile(context.Background(), "artifact-1", different, "actor-1"); err == nil || !strings.Contains(err.Error(), "replace is not supported") {
		t.Fatalf("different payload err=%v", err)
	}
	for _, args := range [][3]string{{"", path, "actor"}, {"artifact-1", "", "actor"}, {"artifact-1", path, ""}} {
		if _, err := service.ImportFile(context.Background(), args[0], args[1], args[2]); err == nil {
			t.Fatalf("args=%v should fail", args)
		}
	}
	if _, err := service.ImportFile(context.Background(), "missing", path, "actor-1"); err == nil || !strings.Contains(err.Error(), `artifact "missing" not found`) {
		t.Fatalf("missing artifact err=%v", err)
	}
	if _, err := service.ImportFile(context.Background(), "artifact-1", filepath.Join(t.TempDir(), "missing.jar"), "actor-1"); err == nil || !strings.Contains(err.Error(), "inspect import file") {
		t.Fatalf("missing file err=%v", err)
	}
	if _, err := service.ImportFile(context.Background(), "artifact-1", t.TempDir(), "actor-1"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory err=%v", err)
	}
}

func TestImportFileRejectsNonPendingArtifacts(t *testing.T) {
	path := writeArtifactFile(t, "payload.jar", "payload")
	for _, status := range []artifact.Status{artifact.StatusApproved, artifact.StatusRejected, artifact.StatusDeprecated} {
		t.Run(string(status), func(t *testing.T) {
			blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NewWithBlobStore(artifactImportStore(t, status), blobs).ImportFile(context.Background(), "artifact-1", path, "actor-1"); err == nil || !strings.Contains(err.Error(), "must be pending") {
				t.Fatalf("status=%s err=%v", status, err)
			}
		})
	}
}

func TestCreateMetadataCreatesPendingArtifactAndAudit(t *testing.T) {
	store := memory.New()
	now := fixedReviewTime()
	store.Projects["project-1"] = project.Project{ID: "project-1", Name: "Project", CreatedAt: now}
	service := New(store)
	service.now = fixedReviewTime
	service.newID = fixedReviewID
	value, err := service.CreateMetadata(context.Background(), "artifact-1", "Test Artifact", artifact.TypeJar, "project-1", "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != artifact.StatusPending || value.ProjectID != "project-1" || value.UploaderID != "actor-1" || value.PayloadStatus != artifact.PayloadMetadataOnly || value.SHA256 != "" || value.SizeBytes != 0 {
		t.Fatalf("artifact=%+v", value)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionCreated || events[0].ProjectID != "project-1" || events[0].Metadata["artifactName"] != "Test Artifact" || events[0].Metadata["artifactType"] != "jar" || events[0].Metadata["projectId"] != "project-1" || events[0].Metadata["status"] != "pending" || events[0].Metadata["actor"] != "actor-1" {
		t.Fatalf("events=%+v", events)
	}
}

func TestCreateMetadataValidationAndDuplicate(t *testing.T) {
	store := memory.New()
	store.Projects["project-1"] = project.Project{ID: "project-1", Name: "Project", CreatedAt: fixedReviewTime()}
	service := New(store)
	tests := []struct {
		name, id, artifactName, projectID, actor string
		kind                                     artifact.Type
	}{
		{name: "id", artifactName: "Artifact", kind: artifact.TypeJar, projectID: "project-1", actor: "actor-1"},
		{name: "name", id: "artifact-1", kind: artifact.TypeJar, projectID: "project-1", actor: "actor-1"},
		{name: "type", id: "artifact-1", artifactName: "Artifact", projectID: "project-1", actor: "actor-1"},
		{name: "project", id: "artifact-1", artifactName: "Artifact", kind: artifact.TypeJar, actor: "actor-1"},
		{name: "actor", id: "artifact-1", artifactName: "Artifact", kind: artifact.TypeJar, projectID: "project-1"},
		{name: "invalid type", id: "artifact-1", artifactName: "Artifact", kind: artifact.Type("binary"), projectID: "project-1", actor: "actor-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.CreateMetadata(context.Background(), test.id, test.artifactName, test.kind, test.projectID, test.actor); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if _, err := service.CreateMetadata(context.Background(), "artifact-1", "Artifact", artifact.TypeJar, "missing-project", "actor-1"); err == nil || !strings.Contains(err.Error(), "load project") {
		t.Fatalf("missing project err=%v", err)
	}
	if _, err := service.CreateMetadata(context.Background(), "artifact-1", "Artifact", artifact.TypeJar, "project-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMetadata(context.Background(), "artifact-1", "Replacement", artifact.TypeJar, "project-1", "actor-1"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate err=%v", err)
	}
}

func TestCreatedMetadataArtifactApprovalAndStaging(t *testing.T) {
	store := memory.New()
	now := fixedReviewTime()
	store.Projects["project-1"] = project.Project{ID: "project-1", Name: "Project", CreatedAt: now}
	_ = store.SaveSession(context.Background(), session.Session{ID: "session-1", ProjectID: "project-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now})
	blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithBlobStore(store, blobs)
	if _, err := service.CreateMetadata(context.Background(), "artifact-1", "Artifact", artifact.TypeJar, "project-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	pending, err := artifactstagingsvc.New(store).CreatePlan(context.Background(), artifactstagingsvc.CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "test-artifact.jar"})
	if err != nil || pending.Status != artifactstaging.StatusRejected {
		t.Fatalf("pending plan=%+v err=%v", pending, err)
	}
	if _, err := service.ImportFile(context.Background(), "artifact-1", writeArtifactFile(t, "artifact.jar", "artifact"), "actor-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "trusted payload"); err != nil {
		t.Fatal(err)
	}
	planned, err := artifactstagingsvc.NewWithPayloadVerifier(store, blobs).CreatePlan(context.Background(), artifactstagingsvc.CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "test-artifact.jar"})
	if err != nil || planned.Status != artifactstaging.StatusPlanned || planned.ArtifactHash == "" {
		t.Fatalf("planned=%+v err=%v", planned, err)
	}
}

func TestApproveArtifactUpdatesReviewMetadataAndAudit(t *testing.T) {
	store := artifactReviewStore(t, artifact.StatusPending)
	service := NewWithPayloadVerifier(store, matchingPayloadVerifier(store))
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

func TestApproveArtifactRequiresVerifiedPayloadWithoutMutation(t *testing.T) {
	t.Run("missing payload metadata", func(t *testing.T) {
		store := artifactImportStore(t, artifact.StatusPending)
		blobs, err := artifactblob.Open(filepath.Join(t.TempDir(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		assertApprovalFailureUnchanged(t, store, NewWithPayloadVerifier(store, blobs), "payload metadata is missing")
	})

	t.Run("missing blob", func(t *testing.T) {
		store := artifactReviewStore(t, artifact.StatusPending)
		blobs, err := artifactblob.Open(filepath.Join(t.TempDir(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		assertApprovalFailureUnchanged(t, store, NewWithPayloadVerifier(store, blobs), "payload blob is missing")
	})

	t.Run("corrupted blob", func(t *testing.T) {
		store := artifactImportStore(t, artifact.StatusPending)
		blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		service := NewWithBlobStore(store, blobs)
		value, err := service.ImportFile(context.Background(), "artifact-1", writeArtifactFile(t, "artifact.jar", "artifact"), "actor-1")
		if err != nil {
			t.Fatal(err)
		}
		path, err := blobs.Path(value.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("corrupted"), 0o640); err != nil {
			t.Fatal(err)
		}
		assertApprovalFailureUnchanged(t, store, service, "payload blob is corrupted")
	})
}

func TestApproveArtifactRejectsInvalidPayloadMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*artifact.Artifact)
		want   string
	}{
		{name: "unsupported algorithm", mutate: func(value *artifact.Artifact) { value.PayloadAlgorithm = "sha512" }, want: "unsupported payload algorithm"},
		{name: "invalid hash", mutate: func(value *artifact.Artifact) { value.SHA256 = "invalid" }, want: "invalid payload SHA-256 hash"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := artifactReviewStore(t, artifact.StatusPending)
			value := store.Artifacts["artifact-1"]
			test.mutate(&value)
			store.Artifacts["artifact-1"] = value
			assertApprovalFailureUnchanged(t, store, NewWithPayloadVerifier(store, matchingPayloadVerifier(store)), test.want)
		})
	}
}

func TestApproveArtifactRejectsVerifiedBlobMetadataMismatch(t *testing.T) {
	store := artifactReviewStore(t, artifact.StatusPending)
	verifier := payloadVerifierFunc(func(_ context.Context, hash string) (string, string, string, int64, error) {
		return "sha256", hash, "sha256/different", 999, nil
	})
	assertApprovalFailureUnchanged(t, store, NewWithPayloadVerifier(store, verifier), "payload metadata does not match verified blob")
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
	if _, err := NewWithPayloadVerifier(store, matchingPayloadVerifier(store)).ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "trusted"); err != nil {
		t.Fatal(err)
	}
	plan, err := artifactstagingsvc.NewWithPayloadVerifier(store, matchingPayloadVerifier(store)).CreatePlan(context.Background(), artifactstagingsvc.CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
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
	_ = store.SaveArtifact(context.Background(), artifact.Artifact{
		ID: "artifact-1", Name: "Test Artifact", Type: artifact.TypeJar, UploaderID: "uploader-1",
		SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, PayloadStatus: artifact.PayloadAvailable,
		PayloadAlgorithm: "sha256", PayloadReference: "sha256/example", PayloadImportedBy: "uploader-1", PayloadImportedAt: &now,
		Status: status, CreatedAt: now,
	})
	_ = store.SaveSession(context.Background(), session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now})
	return store
}

func artifactImportStore(t *testing.T, status artifact.Status) *memory.Store {
	t.Helper()
	store := memory.New()
	if err := store.SaveArtifact(context.Background(), artifact.Artifact{
		ID: "artifact-1", ProjectID: "project-1", Name: "Artifact", Type: artifact.TypeJar,
		UploaderID: "creator-1", PayloadStatus: artifact.PayloadMetadataOnly,
		TargetMinecraftVersions: []string{}, LoaderCompatibility: []string{}, Status: status, CreatedAt: fixedReviewTime(),
	}); err != nil {
		t.Fatal(err)
	}
	return store
}

func writeArtifactFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertApprovalFailureUnchanged(t *testing.T, store *memory.Store, service *Service, want string) {
	t.Helper()
	before, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, _ := store.ListAuditEvents(context.Background())
	if _, err := service.ApproveArtifact(context.Background(), "artifact-1", "reviewer-1", "trusted"); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("approval err=%v want=%q", err, want)
	}
	after, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, _ := store.ListAuditEvents(context.Background())
	if after.Status != before.Status || after.ReviewedBy != "" || after.ReviewedAt != nil || after.ReviewReason != "" || len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("failed approval mutated state: before=%+v after=%+v events=%d->%d", before, after, len(eventsBefore), len(eventsAfter))
	}
}

func fixedReviewTime() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) }

func fixedReviewID(prefix string) (string, error) { return prefix + "-fixed", nil }

type payloadVerifierFunc func(context.Context, string) (algorithm, hash, reference string, size int64, err error)

func (function payloadVerifierFunc) VerifyPayload(ctx context.Context, hash string) (algorithm, verifiedHash, reference string, size int64, err error) {
	return function(ctx, hash)
}

func matchingPayloadVerifier(store *memory.Store) payloadVerifierFunc {
	return func(_ context.Context, hash string) (string, string, string, int64, error) {
		value := store.Artifacts["artifact-1"]
		return value.PayloadAlgorithm, hash, value.PayloadReference, value.SizeBytes, nil
	}
}
