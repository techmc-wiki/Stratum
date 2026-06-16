package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/session"
)

type mockRepo struct {
	sessions    map[string]session.Session
	checkpoints map[string]checkpoint.Checkpoint
	auditEvents []audit.Event
	createErr   error
}

func (m *mockRepo) GetSession(ctx context.Context, id string) (session.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return session.Session{}, fmt.Errorf("session not found")
}

func (m *mockRepo) CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.checkpoints[cp.ID] = cp
	return nil
}

func (m *mockRepo) GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error) {
	return m.checkpoints[id], nil
}

func (m *mockRepo) ListCheckpoints(ctx context.Context) ([]checkpoint.Checkpoint, error) {
	var result []checkpoint.Checkpoint
	for _, cp := range m.checkpoints {
		result = append(result, cp)
	}
	return result, nil
}

func (m *mockRepo) ListCheckpointsBySession(ctx context.Context, sessionID string) ([]checkpoint.Checkpoint, error) {
	var result []checkpoint.Checkpoint
	for _, cp := range m.checkpoints {
		if cp.SourceSessionID == sessionID {
			result = append(result, cp)
		}
	}
	return result, nil
}

func (m *mockRepo) AppendAuditEvent(ctx context.Context, event audit.Event) error {
	m.auditEvents = append(m.auditEvents, event)
	return nil
}

var testTime = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

func TestCreateMetadataOnlyCheckpoint(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	ctx := context.Background()
	cp, err := Create(ctx, repo, CreateRequest{
		ID: "cp-1", SessionID: "s-1", ActorID: "actor-1", Notes: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID != "cp-1" || cp.ProjectID != "p-1" || cp.RoomID != "r-1" || cp.SourceSessionID != "s-1" || cp.EnvironmentID != "env-1" {
		t.Fatalf("checkpoint fields: %+v", cp)
	}
	if cp.Status != checkpoint.StatusMetadataOnly {
		t.Fatalf("status = %s, want metadata_only", cp.Status)
	}
	if len(repo.auditEvents) != 1 || repo.auditEvents[0].Action != "checkpoint.created" {
		t.Fatalf("audit events: %+v", repo.auditEvents)
	}
	if cp.RuntimeStatusSnapshot != nil || repo.auditEvents[0].Metadata["runtimeStatusSnapshotCaptured"] != "false" {
		t.Fatalf("unexpected runtime status snapshot: checkpoint=%+v audit=%+v", cp.RuntimeStatusSnapshot, repo.auditEvents[0])
	}
}

func TestCreateStoresRuntimeStatusSnapshot(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-snapshot": {ID: "s-snapshot", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", RuntimeProfileID: "dummy-process"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	snapshot := &checkpoint.RuntimeStatusSnapshot{
		CapturedAt: time.Date(2026, 6, 14, 13, 0, 0, 0, time.UTC), SessionID: "s-snapshot",
		RuntimeRootExists: true, SessionRootExists: true, EnvironmentManifestExists: true,
		EnvironmentID: "env-1", ProcessState: "running", PID: 42, OverallStatus: "ok",
		Issues: []string{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-snapshot", SessionID: "s-snapshot", ActorID: "actor-1", RuntimeStatusSnapshot: snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.RuntimeStatusSnapshot == nil || !cp.RuntimeStatusSnapshot.EnvironmentManifestExists || cp.RuntimeStatusSnapshot.ProcessState != "running" {
		t.Fatalf("runtime status snapshot = %+v", cp.RuntimeStatusSnapshot)
	}
	if cp.RuntimeStatusSnapshot.RuntimeProfileID != "dummy-process" {
		t.Fatalf("runtime profile = %q", cp.RuntimeStatusSnapshot.RuntimeProfileID)
	}
	event := repo.auditEvents[0]
	if event.Metadata["runtimeStatusSnapshotCaptured"] != "true" || event.Metadata["runtimeStatusOverallStatus"] != "ok" || event.Metadata["processState"] != "running" || event.Metadata["runtimeProfileId"] != "dummy-process" {
		t.Fatalf("audit metadata = %+v", event.Metadata)
	}
	snapshot.Issues = append(snapshot.Issues, "mutated")
	if len(cp.RuntimeStatusSnapshot.Issues) != 0 {
		t.Fatalf("snapshot aliases request: %+v", cp.RuntimeStatusSnapshot)
	}
}

func TestCreateRejectsRuntimeStatusSnapshotForAnotherSession(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-mismatch", SessionID: "s-1", ActorID: "actor-1",
		RuntimeStatusSnapshot: &checkpoint.RuntimeStatusSnapshot{SessionID: "s-2"},
	})
	if err == nil {
		t.Fatal("expected snapshot session mismatch")
	}
	if len(repo.checkpoints) != 0 || len(repo.auditEvents) != 0 {
		t.Fatalf("writes after mismatch: checkpoints=%+v audits=%+v", repo.checkpoints, repo.auditEvents)
	}
}

func TestCreateCapturesRuntimeProfileID(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-profile": {ID: "s-profile", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", RuntimeProfileID: "dummy-process"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	ctx := context.Background()
	cp, err := Create(ctx, repo, CreateRequest{
		ID: "cp-profile", SessionID: "s-profile", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.RuntimeProfileID != "dummy-process" {
		t.Fatalf("RuntimeProfileID = %q, want dummy-process", cp.RuntimeProfileID)
	}
}

func TestCreateSucceedsWithEmptyRuntimeProfileID(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-noprofile": {ID: "s-noprofile", ProjectID: "p-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	ctx := context.Background()
	cp, err := Create(ctx, repo, CreateRequest{
		ID: "cp-noprofile", SessionID: "s-noprofile", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.RuntimeProfileID != "" {
		t.Fatalf("RuntimeProfileID = %q, want empty", cp.RuntimeProfileID)
	}
}

func TestCreateDerivesFieldsFromSession(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-test": {ID: "s-test", ProjectID: "p-test", RoomID: "r-test", EnvironmentID: "env-test"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	ctx := context.Background()
	cp, err := Create(ctx, repo, CreateRequest{
		ID: "cp-derived", SessionID: "s-test", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.ProjectID != "p-test" || cp.RoomID != "r-test" || cp.EnvironmentID != "env-test" {
		t.Fatalf("fields not derived: %+v", cp)
	}
}

func TestCreateMissingActorFails(t *testing.T) {
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-1": {ID: "s-1", ProjectID: "p-1", EnvironmentID: "env-1"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	ctx := context.Background()
	_, err := Create(ctx, repo, CreateRequest{
		ID: "cp-1", SessionID: "s-1", ActorID: "",
	})
	if err == nil {
		t.Fatal("missing actor should fail")
	}
}

func TestCreateFailureDoesNotWriteAudit(t *testing.T) {
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-1": {ID: "s-1", ProjectID: "p-1", EnvironmentID: "env-1"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
		createErr:   fmt.Errorf("duplicate id"),
	}
	ctx := context.Background()
	_, err := Create(ctx, repo, CreateRequest{
		ID: "cp-1", SessionID: "s-1", ActorID: "actor-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(repo.auditEvents) != 0 {
		t.Fatalf("audit events written on failure: %+v", repo.auditEvents)
	}
}

func TestGetReturnsCheckpoint(t *testing.T) {
	repo := &mockRepo{
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-1": {ID: "cp-1", SourceSessionID: "s-1", CreatorID: "actor-1", Status: checkpoint.StatusMetadataOnly, EnvironmentID: "env-1", CreatedAt: testTime},
		},
	}
	ctx := context.Background()
	cp, err := Get(ctx, repo, "cp-1")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID != "cp-1" {
		t.Fatalf("got %+v", cp)
	}
}

func TestListReturnsCheckpoints(t *testing.T) {
	repo := &mockRepo{
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-1": {ID: "cp-1"},
			"cp-2": {ID: "cp-2"},
		},
	}
	ctx := context.Background()
	values, err := List(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("got %d checkpoints", len(values))
	}
}

func TestListBySessionReturnsOnlyMatching(t *testing.T) {
	repo := &mockRepo{
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-1": {ID: "cp-1", SourceSessionID: "s-1"},
			"cp-2": {ID: "cp-2", SourceSessionID: "s-2"},
			"cp-3": {ID: "cp-3", SourceSessionID: "s-1"},
		},
	}
	ctx := context.Background()
	values, err := ListBySession(ctx, repo, "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("got %d checkpoints, want 2", len(values))
	}
	for _, cp := range values {
		if cp.SourceSessionID != "s-1" {
			t.Fatalf("wrong session: %+v", cp)
		}
	}
}
