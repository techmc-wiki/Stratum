package fork

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
)

var testTime = time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

type mockRepo struct {
	sessions     map[string]session.Session
	rooms        map[string]room.Room
	checkpoints  map[string]checkpoint.Checkpoint
	environments map[string]environment.Environment
	auditEvents  []audit.Event
	saveErr      error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		sessions:     map[string]session.Session{},
		rooms:        map[string]room.Room{},
		checkpoints:  map[string]checkpoint.Checkpoint{},
		environments: map[string]environment.Environment{},
	}
}

func (m *mockRepo) GetSession(_ context.Context, id string) (session.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return session.Session{}, fmt.Errorf("session %q not found", id)
	}
	return s, nil
}

func (m *mockRepo) GetRoom(_ context.Context, id string) (room.Room, error) {
	r, ok := m.rooms[id]
	if !ok {
		return room.Room{}, fmt.Errorf("room %q not found", id)
	}
	return r, nil
}

func (m *mockRepo) GetCheckpoint(_ context.Context, id string) (checkpoint.Checkpoint, error) {
	cp, ok := m.checkpoints[id]
	if !ok {
		return checkpoint.Checkpoint{}, fmt.Errorf("checkpoint %q not found", id)
	}
	return cp, nil
}

func (m *mockRepo) GetEnvironment(_ context.Context, id string) (environment.Environment, error) {
	env, ok := m.environments[id]
	if !ok {
		return environment.Environment{}, fmt.Errorf("environment %q not found", id)
	}
	return env, nil
}

func (m *mockRepo) SaveSession(_ context.Context, value session.Session) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.sessions[value.ID] = value
	return nil
}

func (m *mockRepo) AppendAuditEvent(_ context.Context, event audit.Event) error {
	m.auditEvents = append(m.auditEvents, event)
	return nil
}

func newTestService(repo *mockRepo) *Service {
	s := New(repo)
	s.now = func() time.Time { return testTime }
	seq := 0
	s.newID = func(prefix string) (string, error) {
		seq++
		return fmt.Sprintf("%s-%d", prefix, seq), nil
	}
	return s
}

func TestCreateForkFromSession(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", RuntimeProfileID: "dummy-process",
		CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "testing dangerous experiment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.Type != session.TypeFork {
		t.Fatalf("type = %s, want fork", fork.Type)
	}
	if fork.EnvironmentID != "env-1" {
		t.Fatalf("environment = %s", fork.EnvironmentID)
	}
	if fork.ForkProvenance == nil {
		t.Fatal("fork provenance is nil")
	}
	if fork.ForkProvenance.SourceType != "session" || fork.ForkProvenance.SourceID != "source-session" || fork.ForkProvenance.CreatorID != "user-fork" || fork.ForkProvenance.Reason != "testing dangerous experiment" {
		t.Fatalf("provenance = %+v", fork.ForkProvenance)
	}
	if fork.ForkProvenance.SourceSessionID != "source-session" {
		t.Fatalf("source session = %q", fork.ForkProvenance.SourceSessionID)
	}
	if len(repo.auditEvents) != 1 || repo.auditEvents[0].Action != "session.fork" {
		t.Fatalf("audit events = %+v", repo.auditEvents)
	}
}

func TestCreateForkFromRoom(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["shared-session"] = session.Session{
		ID: "shared-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	repo.rooms["room-1"] = room.Room{
		ID: "room-1", ProjectID: "project-1", Name: "Test Room",
		EnvironmentID: "env-1", SharedSessionID: "shared-session", CreatedAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeRoom,
		SourceID:   "room-1",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "forking from room",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ForkProvenance.SourceID != "room-1" || fork.ForkProvenance.SourceSessionID != "shared-session" {
		t.Fatalf("provenance = %+v", fork.ForkProvenance)
	}
	if fork.EnvironmentID != "env-1" {
		t.Fatalf("environment = %s", fork.EnvironmentID)
	}
}

func TestCreateForkFromCheckpoint(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env"}
	repo.checkpoints["cp-1"] = checkpoint.Checkpoint{
		ID: "cp-1", ProjectID: "project-1", RoomID: "room-1",
		SourceSessionID: "original-session", CreatorID: "owner-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType:         SourceTypeCheckpoint,
		SourceID:           "cp-1",
		ProjectID:          "project-1",
		RoomID:             "room-1",
		SourceCheckpointID: "cp-1",
		CreatorID:          "user-fork",
		Reason:             "forking from checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ForkProvenance.SourceID != "cp-1" {
		t.Fatalf("source id = %q", fork.ForkProvenance.SourceID)
	}
	if fork.EnvironmentID != "env-1" {
		t.Fatalf("environment = %s", fork.EnvironmentID)
	}
}

func TestForkFromCheckpointCarriesLucyLockHash(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env"}
	repo.checkpoints["cp-1"] = checkpoint.Checkpoint{
		ID: "cp-1", ProjectID: "project-1", RoomID: "room-1",
		SourceSessionID: "original-session", CreatorID: "owner-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", LucyLockHash: "hash123", CreatedAt: testTime,
	}
	svc := newTestService(repo)
	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType:         SourceTypeCheckpoint,
		SourceID:           "cp-1",
		ProjectID:          "project-1",
		RoomID:             "room-1",
		SourceCheckpointID: "cp-1",
		CreatorID:          "user-fork",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.Metadata["sourceLucyLockHash"] != "hash123" {
		t.Fatalf("sourceLucyLockHash = %q, want hash123", fork.Metadata["sourceLucyLockHash"])
	}
}

func TestCreateForkFromCheckpointWithoutSession(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env"}
	repo.checkpoints["cp-1"] = checkpoint.Checkpoint{
		ID: "cp-1", ProjectID: "project-1", RoomID: "room-1",
		CreatorID: "owner-1", Kind: checkpoint.KindManual,
		Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly, EnvironmentID: "env-1",
		SourceSessionID: "", CreatedAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeCheckpoint,
		SourceID:   "cp-1",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "fork from metadata-only checkpoint",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ForkProvenance.SourceID != "cp-1" {
		t.Fatalf("source id = %q", fork.ForkProvenance.SourceID)
	}
	if fork.EnvironmentID != "env-1" {
		t.Fatalf("environment = %s", fork.EnvironmentID)
	}
}

func TestCreateForkRejectsMissingSourceType(t *testing.T) {
	svc := newTestService(newMockRepo())
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceID:  "source-1",
		ProjectID: "project-1",
		CreatorID: "user-1",
	})
	if err == nil {
		t.Fatal("expected missing source type error")
	}
}

func TestCreateForkRejectsInvalidSourceType(t *testing.T) {
	svc := newTestService(newMockRepo())
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: "invalid",
		SourceID:   "source-1",
		ProjectID:  "project-1",
		CreatorID:  "user-1",
	})
	if err == nil {
		t.Fatal("expected invalid source type error")
	}
}

func TestCreateForkRejectsMissingCreator(t *testing.T) {
	svc := newTestService(newMockRepo())
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "source-1",
		ProjectID:  "project-1",
	})
	if err == nil {
		t.Fatal("expected missing creator error")
	}
}

func TestCreateForkFailsOnNonexistentSourceSession(t *testing.T) {
	svc := newTestService(newMockRepo())
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "nonexistent",
		ProjectID:  "project-1",
		CreatorID:  "user-1",
	})
	if err == nil {
		t.Fatal("expected source session not found error")
	}
}

func TestCreateForkFailsOnRoomWithoutSharedSession(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env"}
	repo.rooms["room-1"] = room.Room{
		ID: "room-1", ProjectID: "project-1", Name: "Empty Room",
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	svc := newTestService(repo)
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeRoom,
		SourceID:   "room-1",
		ProjectID:  "project-1",
		CreatorID:  "user-1",
	})
	if err == nil {
		t.Fatal("expected room without shared session error")
	}
}

func TestCreateForkFromCheckpointWithExistingSourceSession(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["original-session"] = session.Session{
		ID: "original-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	repo.checkpoints["cp-1"] = checkpoint.Checkpoint{
		ID: "cp-1", ProjectID: "project-1", RoomID: "room-1",
		SourceSessionID: "original-session", CreatorID: "owner-1",
		Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly, ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID: "env-1", CreatedAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeCheckpoint,
		SourceID:   "cp-1",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "fork from checkpoint with live source",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ForkProvenance.SourceSessionID != "original-session" {
		t.Fatalf("source session = %q", fork.ForkProvenance.SourceSessionID)
	}
}

func TestCreateForkWithTTL(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	ttl := 4 * time.Hour
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "ttl fork",
		TTL:        &ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ExpiresAt == nil {
		t.Fatal("expires at should be set")
	}
	expected := testTime.Add(ttl)
	if !fork.ExpiresAt.Equal(expected) {
		t.Fatalf("expires at = %s, want %s", fork.ExpiresAt.Format(time.RFC3339), expected.Format(time.RFC3339))
	}
}

func TestCreateForkWithCustomEnvironment(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Default Env", MinecraftVersion: "1.17.1"}
	repo.environments["env-custom"] = environment.Environment{ID: "env-custom", Name: "Custom Env", MinecraftVersion: "1.18"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType:    SourceTypeSession,
		SourceID:      "source-session",
		ProjectID:     "project-1",
		RoomID:        "room-1",
		CreatorID:     "user-fork",
		Reason:        "custom env fork",
		EnvironmentID: "env-custom",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.EnvironmentID != "env-custom" {
		t.Fatalf("environment = %s, want env-custom", fork.EnvironmentID)
	}
	if fork.ForkProvenance.InheritedEnvironmentID != "env-custom" {
		t.Fatalf("inherited environment = %s", fork.ForkProvenance.InheritedEnvironmentID)
	}
}

func TestCreateForkRejectsMissingEnvironment(t *testing.T) {
	repo := newMockRepo()
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)
	_, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		CreatorID:  "user-1",
	})
	if err == nil {
		t.Fatal("expected missing environment error")
	}
}

func TestCreateForkWithInheritedArtifacts(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType:            SourceTypeSession,
		SourceID:              "source-session",
		ProjectID:             "project-1",
		RoomID:                "room-1",
		CreatorID:             "user-fork",
		Reason:                "artifact fork",
		InheritedArtifactIDs:  []string{"artifact-1", "artifact-2"},
		InheritedServerConfig: map[string]string{"difficulty": "hard", "gamemode": "survival"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fork.ForkProvenance.InheritedArtifactIDs) != 2 || fork.ForkProvenance.InheritedArtifactIDs[0] != "artifact-1" {
		t.Fatalf("artifacts = %v", fork.ForkProvenance.InheritedArtifactIDs)
	}
	if fork.ForkProvenance.InheritedServerConfig["difficulty"] != "hard" {
		t.Fatalf("server config = %v", fork.ForkProvenance.InheritedServerConfig)
	}
}

func TestCreateForkGeneratesSessionID(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)

	fork, err := svc.CreateFork(context.Background(), ForkOptions{
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		CreatorID:  "user-fork",
		Reason:     "auto id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fork.ID == "" {
		t.Fatal("session id should be generated")
	}
}

func TestCreateForkPersistsAndAudits(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	svc := newTestService(repo)

	_, err := svc.CreateFork(context.Background(), ForkOptions{
		ID:         "fork-1",
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		RoomID:     "room-1",
		CreatorID:  "user-fork",
		Reason:     "persistence check",
	})
	if err != nil {
		t.Fatal(err)
	}

	persisted, ok := repo.sessions["fork-1"]
	if !ok {
		t.Fatal("fork session not persisted")
	}
	if persisted.ForkProvenance == nil || persisted.ForkProvenance.Reason != "persistence check" {
		t.Fatalf("persisted = %+v", persisted)
	}

	if len(repo.auditEvents) != 1 {
		t.Fatalf("audit events = %d, want 1", len(repo.auditEvents))
	}
	event := repo.auditEvents[0]
	if event.Action != "session.fork" || event.TargetID != "fork-1" || event.ActorID != "user-fork" {
		t.Fatalf("event = %+v", event)
	}
	if event.Metadata["sourceId"] != "source-session" || event.Metadata["forkSessionId"] != "fork-1" {
		t.Fatalf("metadata = %+v", event.Metadata)
	}
}

func TestCreateForkStoreFailureRollsBackImplicitly(t *testing.T) {
	repo := newMockRepo()
	repo.environments["env-1"] = environment.Environment{ID: "env-1", Name: "Test Env", MinecraftVersion: "1.17.1"}
	repo.sessions["source-session"] = session.Session{
		ID: "source-session", ProjectID: "project-1", RoomID: "room-1",
		OwnerUserID: "owner-1", Type: session.TypeShared, State: session.StateRunning,
		EnvironmentID: "env-1", CreatedAt: testTime, LastActiveAt: testTime,
	}
	repo.saveErr = fmt.Errorf("disk full")
	svc := newTestService(repo)

	_, err := svc.CreateFork(context.Background(), ForkOptions{
		ID:         "fork-fail",
		SourceType: SourceTypeSession,
		SourceID:   "source-session",
		ProjectID:  "project-1",
		CreatorID:  "user-fork",
		Reason:     "should fail",
	})
	if err == nil {
		t.Fatal("expected save failure")
	}
	if len(repo.auditEvents) != 0 {
		t.Fatal("audit should not be recorded on failed save")
	}
}
