package artifactstagingsvc

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/repository/artifactblob"
	"github.com/stratummc/stratum/internal/repository/memory"
)

func TestApprovedArtifactCreatesStagingPlan(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	service := NewWithPayloadVerifier(store, matchingVerifier(store))
	service.now = fixedTime
	service.newID = fixedID
	plan, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != artifactstaging.StatusPlanned || plan.StagingKind != artifactstaging.KindArtifact || plan.TargetStagingName != filepath.Clean("mods/test.jar") || plan.ArtifactHash == "" {
		t.Fatalf("plan=%+v", plan)
	}
	values, err := store.ListArtifactStagingPlansBySession(context.Background(), "session-1")
	if err != nil || len(values) != 1 || values[0].ID != plan.ID {
		t.Fatalf("plans=%+v err=%v", values, err)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionPlanCreated || events[0].Metadata["planId"] != plan.ID || events[0].Metadata["stagingKind"] != string(artifactstaging.KindArtifact) || events[0].Metadata["verificationStatus"] != "verified" || events[0].Metadata["payloadHash"] != plan.ArtifactHash {
		t.Fatalf("events=%+v", events)
	}
}

func TestConfigArtifactCreatesConfigStagingPlan(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeConfigPreset)
	plan, err := NewWithPayloadVerifier(store, matchingVerifier(store)).CreatePlan(context.Background(), CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "server.properties"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.StagingKind != artifactstaging.KindConfig {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestUnapprovedArtifactsPersistRejectedPlans(t *testing.T) {
	for _, status := range []artifact.Status{artifact.StatusPending, artifact.StatusRejected, artifact.StatusDeprecated} {
		t.Run(string(status), func(t *testing.T) {
			store := stagingStore(t, status, artifact.TypeJar)
			plan, err := New(store).CreatePlan(context.Background(), CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Status != artifactstaging.StatusRejected || plan.RejectionReason == "" {
				t.Fatalf("plan=%+v", plan)
			}
			events, _ := store.ListAuditEvents(context.Background())
			if len(events) != 1 || events[0].Action != ActionPlanRejected || events[0].Metadata["rejectionReason"] == "" || events[0].Metadata["verificationStatus"] != "not_attempted" {
				t.Fatalf("events=%+v", events)
			}
		})
	}
}

func TestApprovedArtifactPayloadVerificationFailuresPersistRejectedPlans(t *testing.T) {
	t.Run("missing metadata", func(t *testing.T) {
		store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
		value := store.Artifacts["artifact-1"]
		value.PayloadStatus = artifact.PayloadMetadataOnly
		value.PayloadAlgorithm = ""
		value.SHA256 = ""
		value.PayloadReference = ""
		store.Artifacts[value.ID] = value
		assertRejectedVerification(t, store, NewWithPayloadVerifier(store, matchingVerifier(store)), "payload metadata is missing")
	})

	t.Run("missing blob", func(t *testing.T) {
		store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
		blobs, err := artifactblob.Open(filepath.Join(t.TempDir(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		assertRejectedVerification(t, store, NewWithPayloadVerifier(store, blobs), "payload blob is missing")
	})

	t.Run("corrupted blob", func(t *testing.T) {
		store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
		blobs, err := artifactblob.New(filepath.Join(t.TempDir(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		payload, err := blobs.Put(context.Background(), strings.NewReader("artifact"))
		if err != nil {
			t.Fatal(err)
		}
		value := store.Artifacts["artifact-1"]
		value.SHA256, value.SizeBytes, value.PayloadAlgorithm, value.PayloadReference = payload.Hash, payload.Size, payload.Algorithm, payload.Reference
		store.Artifacts[value.ID] = value
		path, err := blobs.Path(payload.Hash)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("corrupted"), 0o640); err != nil {
			t.Fatal(err)
		}
		assertRejectedVerification(t, store, NewWithPayloadVerifier(store, blobs), "payload blob is corrupted")
	})
}

func TestApprovedArtifactRejectsUnsupportedAlgorithmAndInvalidHash(t *testing.T) {
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
			store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
			value := store.Artifacts["artifact-1"]
			test.mutate(&value)
			store.Artifacts[value.ID] = value
			assertRejectedVerification(t, store, NewWithPayloadVerifier(store, matchingVerifier(store)), test.want)
		})
	}
}

func TestCreatePlanRejectsMissingEntitiesUnsafeNameAndUnknownType(t *testing.T) {
	ctx := context.Background()
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	if _, err := New(store).CreatePlan(ctx, CreateParams{SessionID: "missing", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "test.jar"}); err == nil || !strings.Contains(err.Error(), "load session") {
		t.Fatalf("missing session err=%v", err)
	}
	if _, err := New(store).CreatePlan(ctx, CreateParams{SessionID: "session-1", ArtifactID: "missing", ActorID: "actor-1", Name: "test.jar"}); err == nil || !strings.Contains(err.Error(), "load artifact") {
		t.Fatalf("missing artifact err=%v", err)
	}
	if _, err := New(store).CreatePlan(ctx, CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "../escape.jar"}); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("unsafe name err=%v", err)
	}
	absolute := filepath.Join(t.TempDir(), "artifact.jar")
	if _, err := New(store).CreatePlan(ctx, CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: absolute}); err == nil || !strings.Contains(err.Error(), "relative") {
		t.Fatalf("absolute name err=%v", err)
	}
	unknown := stagingStore(t, artifact.StatusApproved, artifact.Type("unknown"))
	if _, err := New(unknown).CreatePlan(ctx, CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "test.bin"}); err == nil || !strings.Contains(err.Error(), "unsupported artifact type") {
		t.Fatalf("unknown type err=%v", err)
	}
}

func stagingStore(t *testing.T, status artifact.Status, kind artifact.Type) *memory.Store {
	t.Helper()
	store := memory.New()
	now := fixedTime()
	_ = store.SaveSession(context.Background(), session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now})
	_ = store.SaveArtifact(context.Background(), artifact.Artifact{ID: "artifact-1", Name: "Test Artifact", Type: kind, UploaderID: "uploader-1", SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, PayloadStatus: artifact.PayloadAvailable, PayloadAlgorithm: "sha256", PayloadReference: "sha256/test", Status: status, CreatedAt: now})
	return store
}

type payloadVerifierFunc func(context.Context, string) (string, string, string, int64, error)

func (function payloadVerifierFunc) VerifyPayload(ctx context.Context, hash string) (string, string, string, int64, error) {
	return function(ctx, hash)
}

func matchingVerifier(store *memory.Store) payloadVerifierFunc {
	return func(_ context.Context, _ string) (string, string, string, int64, error) {
		value := store.Artifacts["artifact-1"]
		return value.PayloadAlgorithm, value.SHA256, value.PayloadReference, value.SizeBytes, nil
	}
}

func assertRejectedVerification(t *testing.T, store *memory.Store, service *Service, want string) {
	t.Helper()
	before := store.Artifacts["artifact-1"]
	plan, err := service.CreatePlan(context.Background(), CreateParams{SessionID: "session-1", ArtifactID: "artifact-1", ActorID: "actor-1", Name: "mods/test.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != artifactstaging.StatusRejected || !strings.Contains(plan.RejectionReason, want) || plan.Metadata["verificationStatus"] != "failed" {
		t.Fatalf("plan=%+v", plan)
	}
	if !reflect.DeepEqual(before, store.Artifacts["artifact-1"]) {
		t.Fatalf("artifact mutated: before=%+v after=%+v", before, store.Artifacts["artifact-1"])
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 1 || events[0].Action != ActionPlanRejected || events[0].Metadata["verificationStatus"] != "failed" || !strings.Contains(events[0].Metadata["rejectionReason"], want) {
		t.Fatalf("events=%+v", events)
	}
}

func fixedTime() time.Time { return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC) }

func fixedID(prefix string) (string, error) { return prefix + "-fixed", nil }
