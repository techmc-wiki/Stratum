package stagingservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/artifact"
	artifactstaging "github.com/stratummc/stratum/internal/artifact/staging"
	"github.com/stratummc/stratum/internal/session"
	"github.com/stratummc/stratum/internal/storage/artifactblob"
	"github.com/stratummc/stratum/internal/storage/memory"
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

func TestMaterializationReadinessNoPlansAndNotMaterialized(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	service := NewReadinessService(store, matchingVerifier(store), materializationVerifierFunc(func(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
		return agent.MaterializedArtifactsVerification{SessionID: "session-1", Entries: []agent.MaterializedArtifactVerification{}}, nil
	}))
	service.now = fixedTime
	result, err := service.Check(context.Background(), "session-1")
	if err != nil || result.Status != "not_ready" || result.PlannedCount != 0 || !hasReadinessIssue(result, "no_planned_artifacts") {
		t.Fatalf("result=%+v err=%v", result, err)
	}

	addPlannedArtifact(store)
	result, err = service.Check(context.Background(), "session-1")
	if err != nil || result.Status != "not_ready" || result.PlannedCount != 1 || result.MissingMaterializedCount != 1 || len(result.Entries) != 1 || result.Entries[0].Materialized || result.Entries[0].VerificationStatus != "not_materialized" || !hasReadinessIssue(result, "staging_plan_not_materialized") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestMaterializationReadinessValidIsReady(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	addPlannedArtifact(store)
	service := readinessServiceWithStatus(store, "valid")
	service.now = fixedTime
	result, err := service.Check(context.Background(), "session-1")
	if err != nil || result.Status != "ready" || result.CheckedAt != fixedTime() || result.PlannedCount != 1 || result.MaterializedCount != 1 || result.ValidMaterializedCount != 1 || len(result.Issues) != 0 || len(result.Entries) != 1 || !result.Entries[0].Materialized || result.Entries[0].VerificationStatus != "valid" || result.Entries[0].RecommendedAction != "none" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	events, _ := store.ListAuditEvents(context.Background())
	if len(events) != 0 {
		t.Fatalf("readiness created audit events: %+v", events)
	}
}

func TestMaterializationReadinessMissingCorruptedAndUnapproved(t *testing.T) {
	for _, test := range []struct {
		status    string
		issueCode string
	}{
		{status: "missing", issueCode: "materialized_file_missing"},
		{status: "corrupted", issueCode: "materialized_file_corrupted"},
	} {
		t.Run(test.status, func(t *testing.T) {
			store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
			addPlannedArtifact(store)
			result, err := readinessServiceWithStatus(store, test.status).Check(context.Background(), "session-1")
			if err != nil || result.Status != "not_ready" || result.Entries[0].VerificationStatus != test.status || !hasReadinessIssue(result, test.issueCode) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}

	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	addPlannedArtifact(store)
	value := store.Artifacts["artifact-1"]
	value.Status = artifact.StatusRejected
	store.Artifacts[value.ID] = value
	result, err := readinessServiceWithStatus(store, "valid").Check(context.Background(), "session-1")
	if err != nil || result.Status != "not_ready" || result.Entries[0].ArtifactStatus != "rejected" || !hasReadinessIssue(result, "artifact_not_approved") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestMaterializationReadinessUnknownEntryAndAgentFailure(t *testing.T) {
	store := stagingStore(t, artifact.StatusApproved, artifact.TypeJar)
	addPlannedArtifact(store)
	verifier := materializationVerifierFunc(func(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
		return agent.MaterializedArtifactsVerification{SessionID: "session-1", Total: 2, ValidCount: 2, Entries: []agent.MaterializedArtifactVerification{
			{StagingPlanID: "plan-1", ArtifactID: "artifact-1", Status: "valid"},
			{StagingPlanID: "unknown-plan", ArtifactID: "unknown-artifact", Status: "valid"},
		}}, nil
	})
	result, err := NewReadinessService(store, matchingVerifier(store), verifier).Check(context.Background(), "session-1")
	if err != nil || result.Status != "not_ready" || result.UnknownMaterializedCount != 1 || !hasReadinessIssue(result, "unknown_materialized_artifact") {
		t.Fatalf("result=%+v err=%v", result, err)
	}

	failing := materializationVerifierFunc(func(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
		return agent.MaterializedArtifactsVerification{}, errors.New("agent unavailable")
	})
	result, err = NewReadinessService(store, matchingVerifier(store), failing).Check(context.Background(), "session-1")
	if err != nil || result.Status != "error" || !hasReadinessIssue(result, "agent_verification_failed") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func addPlannedArtifact(store *memory.Store) {
	value := store.Artifacts["artifact-1"]
	store.Plans["plan-1"] = artifactstaging.Plan{ID: "plan-1", SessionID: "session-1", ArtifactID: value.ID, ArtifactHash: value.SHA256, Status: artifactstaging.StatusPlanned}
}

func readinessServiceWithStatus(store *memory.Store, status string) *ReadinessService {
	verifier := materializationVerifierFunc(func(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
		result := agent.MaterializedArtifactsVerification{SessionID: "session-1", Total: 1, Entries: []agent.MaterializedArtifactVerification{{StagingPlanID: "plan-1", ArtifactID: "artifact-1", Status: status}}}
		switch status {
		case "valid":
			result.ValidCount = 1
		case "missing":
			result.MissingCount = 1
		case "corrupted":
			result.CorruptedCount = 1
		}
		return result, nil
	})
	return NewReadinessService(store, matchingVerifier(store), verifier)
}

func hasReadinessIssue(result ReadinessResult, code string) bool {
	for _, issue := range result.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

type materializationVerifierFunc func(context.Context, string) (agent.MaterializedArtifactsVerification, error)

func (function materializationVerifierFunc) VerifyMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifactsVerification, error) {
	return function(ctx, sessionID)
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
