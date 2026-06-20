package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
	"github.com/stratummc/stratum/internal/worldprofile"
)

type mockRepo struct {
	sessions    map[string]session.Session
	rooms       map[string]room.Room
	checkpoints map[string]checkpoint.Checkpoint
	auditEvents []audit.Event
	createErr   error
	auditErr    error
	updateErr   error
}

func (m *mockRepo) GetSession(ctx context.Context, id string) (session.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return session.Session{}, fmt.Errorf("session not found")
}

func (m *mockRepo) GetRoom(ctx context.Context, id string) (room.Room, error) {
	if r, ok := m.rooms[id]; ok {
		return r, nil
	}
	return room.Room{}, fmt.Errorf("room not found")
}

func (m *mockRepo) CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.checkpoints[cp.ID] = cp
	return nil
}

func (m *mockRepo) UpdateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error {
	if m.updateErr != nil {
		return m.updateErr
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
	if m.auditErr != nil {
		return m.auditErr
	}
	m.auditEvents = append(m.auditEvents, event)
	return nil
}

type mockAgent struct {
	commands         []string
	calls            []string
	failAt           int
	capabilities     []string
	infoErr          error
	serverProperties string
	minecraftVersion string
}

func (m *mockAgent) Info(context.Context) (agent.AgentInfo, error) {
	if m.infoErr != nil {
		return agent.AgentInfo{}, m.infoErr
	}
	caps := m.capabilities
	if caps == nil {
		caps = []string{"prepare", "start", "stop", "send-command"}
	}
	return agent.AgentInfo{ID: "mock", Capabilities: caps}, nil
}

func (m *mockAgent) RuntimeProfiles(context.Context) ([]runtimeprofile.Profile, error) {
	return nil, nil
}

func (m *mockAgent) PrepareSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) StartSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) StopSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) RestartSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) FreezeSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) UnfreezeSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) InspectSession(context.Context, string) (agent.SessionStatus, error) {
	return agent.SessionStatus{}, nil
}

func (m *mockAgent) CollectLogs(context.Context, string) (agent.LogBatch, error) {
	return agent.LogBatch{}, nil
}

func (m *mockAgent) ReportResources(context.Context) (agent.ResourceReport, error) {
	return agent.ResourceReport{}, nil
}

func (m *mockAgent) CreateCheckpointStub(context.Context, agent.CheckpointRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) RestoreCheckpointStub(context.Context, agent.CheckpointRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockAgent) MaterializeArtifact(context.Context, agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	return agent.ArtifactMaterializationResult{}, nil
}

func (m *mockAgent) InspectMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifacts, error) {
	return agent.MaterializedArtifacts{}, nil
}

func (m *mockAgent) InspectMaterializedArtifact(context.Context, string, string) (agent.MaterializedArtifact, error) {
	return agent.MaterializedArtifact{}, nil
}

func (m *mockAgent) VerifyMaterializedArtifact(context.Context, string, string) (agent.MaterializedArtifactVerification, error) {
	return agent.MaterializedArtifactVerification{}, nil
}

func (m *mockAgent) VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
	return agent.MaterializedArtifactsVerification{}, nil
}

func (m *mockAgent) DryRunArtifactApply(context.Context, agent.ArtifactApplyDryRunRequest) (agent.ArtifactApplyDryRunResult, error) {
	return agent.ArtifactApplyDryRunResult{}, nil
}

func (m *mockAgent) ExecuteArtifactApply(context.Context, agent.ArtifactApplyExecuteRequest) (agent.ArtifactApplyExecuteResult, error) {
	return agent.ArtifactApplyExecuteResult{}, nil
}

func (m *mockAgent) ListAppliedArtifacts(context.Context, string) (agent.AppliedArtifactsResponse, error) {
	return agent.AppliedArtifactsResponse{}, nil
}

func (m *mockAgent) InspectAppliedArtifact(context.Context, string, string) (agent.AppliedArtifactRecord, error) {
	return agent.AppliedArtifactRecord{}, nil
}

func (m *mockAgent) VerifyAppliedArtifact(context.Context, string, string) (agent.AppliedArtifactVerification, error) {
	return agent.AppliedArtifactVerification{}, nil
}

func (m *mockAgent) VerifyAllAppliedArtifacts(context.Context, string) (agent.BatchAppliedArtifactVerification, error) {
	return agent.BatchAppliedArtifactVerification{}, nil
}

func (m *mockAgent) MaterializeEnvironment(context.Context, agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	return agent.EnvironmentMaterializationResult{}, nil
}

func (m *mockAgent) SessionReadyForStart(context.Context, string) (agent.SessionStartReadiness, error) {
	return agent.SessionStartReadiness{}, nil
}

func (m *mockAgent) InspectMCDRConfigStub(context.Context, string) (agent.MCDRConfigStubInspection, error) {
	return agent.MCDRConfigStubInspection{}, nil
}

func (m *mockAgent) SendCommand(ctx context.Context, sessionID, command string) (agent.CommandResult, error) {
	m.commands = append(m.commands, command)
	if m.failAt > 0 && len(m.commands) >= m.failAt {
		return agent.CommandResult{}, fmt.Errorf("mock send-command failure at step %d", len(m.commands))
	}
	return agent.CommandResult{AgentID: "mock", Status: "sent", Message: "ok"}, nil
}

func (m *mockAgent) CreateWorldSnapshot(ctx context.Context, request agent.WorldCheckpointRequest) (agent.WorldCheckpointResult, error) {
	if m.failAt == 3 {
		return agent.WorldCheckpointResult{}, fmt.Errorf("mock snapshot failure")
	}
	return agent.WorldCheckpointResult{
		SessionID:   request.SessionID,
		SnapshotRef: "agent-local://mock/sessions/" + request.SessionID + "/checkpoints/world.zip",
		SizeBytes:   2048,
		SHA256:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		CreatedAt:   testTime,
	}, nil
}

func (m *mockAgent) RestoreWorldSnapshot(ctx context.Context, request agent.WorldCheckpointRestoreRequest) (agent.WorldCheckpointRestoreResult, error) {
	m.calls = append(m.calls, "restore_world_snapshot")
	return agent.WorldCheckpointRestoreResult{
		SessionID:   request.SessionID,
		RestoredRef: "agent-local://mock/sessions/" + request.SessionID + "/work/world_restored",
		EntryCount:  5,
		SizeBytes:   4096,
		RestoredAt:  testTime,
	}, nil
}

func (m *mockAgent) ReadSessionFile(ctx context.Context, sessionID, relativePath string) ([]byte, error) {
	m.calls = append(m.calls, "read_session_file")
	if relativePath == "server.properties" && m.serverProperties != "" {
		return []byte(m.serverProperties), nil
	}
	return nil, fmt.Errorf("file not found: %s", relativePath)
}

func (m *mockAgent) WriteSessionFile(ctx context.Context, sessionID, relativePath string, data []byte) error {
	m.calls = append(m.calls, "write_session_file")
	return nil
}

func (m *mockAgent) GetSessionRuntimeStatus(ctx context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	envManifest := &agent.EnvironmentManifestStatus{
		MinecraftVersion: m.minecraftVersion,
	}
	if m.minecraftVersion == "" {
		envManifest.MinecraftVersion = "1.17.1"
	}
	return agent.SessionRuntimeStatus{
		SessionID:           sessionID,
		EnvironmentManifest: envManifest,
	}, nil
}

var _ agent.AgentClient = (*mockAgent)(nil)

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
	if cp.ConsistencyLevel != consistency.LevelMetadataOnly {
		t.Fatalf("consistency level = %s, want metadata_only", cp.ConsistencyLevel)
	}
	if len(repo.auditEvents) != 1 || repo.auditEvents[0].Action != "checkpoint.created" {
		t.Fatalf("audit events: %+v", repo.auditEvents)
	}
	if repo.auditEvents[0].Metadata["consistencyLevel"] != string(consistency.LevelMetadataOnly) {
		t.Fatalf("audit metadata: %+v", repo.auditEvents[0].Metadata)
	}
	if cp.RuntimeStatusSnapshot != nil || repo.auditEvents[0].Metadata["runtimeStatusSnapshotCaptured"] != "false" {
		t.Fatalf("unexpected runtime status snapshot: checkpoint=%+v audit=%+v", cp.RuntimeStatusSnapshot, repo.auditEvents[0])
	}
}

func TestCreateCheckpointWithLucyLockHash(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-lucy", SessionID: "s-1", ActorID: "actor-1", LucyLockHash: "hash123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.LucyLockHash != "hash123" {
		t.Fatalf("LucyLockHash = %q, want hash123", cp.LucyLockHash)
	}
}

func TestCreateCheckpointWithoutLucyLockHash(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-no-lucy", SessionID: "s-1", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.LucyLockHash != "" {
		t.Fatalf("LucyLockHash = %q, want empty", cp.LucyLockHash)
	}
}

func TestCreateCheckpointCapturesWorldProfile(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		rooms: map[string]room.Room{
			"r-1": {
				ID:        "r-1",
				ProjectID: "p-1",
				DefaultWorldProfile: &worldprofile.WorldProfile{
					ID:                 "wp-1",
					Name:               "Test World",
					Seed:               "12345",
					LevelType:          worldprofile.LevelFlat,
					GeneratorSettings:  `{"layers":[]}`,
					GenerateStructures: false,
					SpawnRadius:        5,
					Difficulty:         worldprofile.DifficultyPeaceful,
					MinecraftVersion:   "1.17.1",
				},
			},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID:                  "cp-world",
		SessionID:           "s-1",
		ActorID:             "actor-1",
		CaptureWorldProfile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.WorldProfileSnapshot == nil {
		t.Fatal("expected world profile snapshot")
	}
	ws := cp.WorldProfileSnapshot
	if ws.Seed != "12345" || ws.LevelType != string(worldprofile.LevelFlat) || ws.Difficulty != string(worldprofile.DifficultyPeaceful) {
		t.Fatalf("world profile snapshot: %+v", ws)
	}
	if ws.GeneratorSettings != `{"layers":[]}` {
		t.Fatalf("GeneratorSettings = %q", ws.GeneratorSettings)
	}
	if ws.SpawnRadius != 5 || ws.GenerateStructures != false {
		t.Fatalf("SpawnRadius=%d GenerateStructures=%v", ws.SpawnRadius, ws.GenerateStructures)
	}
	if ws.SourceProfileID != "wp-1" || ws.CapturedFrom != "room" {
		t.Fatalf("SourceProfileID=%q CapturedFrom=%q", ws.SourceProfileID, ws.CapturedFrom)
	}
}

func TestCreateCheckpointWithoutCaptureWorldProfile(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		rooms: map[string]room.Room{
			"r-1": {
				ID:        "r-1",
				ProjectID: "p-1",
				DefaultWorldProfile: &worldprofile.WorldProfile{
					ID:   "wp-1",
					Name: "Test World",
				},
			},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID:                  "cp-no-world",
		SessionID:           "s-1",
		ActorID:             "actor-1",
		CaptureWorldProfile: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.WorldProfileSnapshot != nil {
		t.Fatalf("expected no world profile snapshot, got %+v", cp.WorldProfileSnapshot)
	}
}

func TestCreateCheckpointCapturesServerProperties(t *testing.T) {
	agent := &mockAgent{
		serverProperties: "level-seed=999\nlevel-type=amplified\ndifficulty=hard\ngenerate-structures=false\nspawn-protection=10\nview-distance=8\n",
		minecraftVersion: "1.19.4",
	}
	wp, _ := worldprofile.New(worldprofile.CreateParams{
		ID:         "wp-1",
		Name:       "Room World",
		Seed:       "123",
		LevelType:  worldprofile.LevelFlat,
		Difficulty: worldprofile.DifficultyPeaceful,
	})
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		rooms: map[string]room.Room{
			"r-1": {ID: "r-1", DefaultWorldProfile: &wp},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID:                  "cp-props",
		SessionID:           "s-1",
		ActorID:             "actor-1",
		CaptureWorldProfile: true,
		AgentClient:         agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.WorldProfileSnapshot == nil {
		t.Fatal("expected world profile snapshot")
	}
	if cp.WorldProfileSnapshot.Seed != "999" {
		t.Errorf("Seed = %q, want 999 from server.properties", cp.WorldProfileSnapshot.Seed)
	}
	if cp.WorldProfileSnapshot.LevelType != "amplified" {
		t.Errorf("LevelType = %q, want amplified", cp.WorldProfileSnapshot.LevelType)
	}
	if cp.WorldProfileSnapshot.Difficulty != "hard" {
		t.Errorf("Difficulty = %q, want hard", cp.WorldProfileSnapshot.Difficulty)
	}
	if cp.WorldProfileSnapshot.GenerateStructures != false {
		t.Errorf("GenerateStructures = %v, want false", cp.WorldProfileSnapshot.GenerateStructures)
	}
	if cp.WorldProfileSnapshot.SpawnRadius != 10 {
		t.Errorf("SpawnRadius = %d, want 10", cp.WorldProfileSnapshot.SpawnRadius)
	}
	if cp.WorldProfileSnapshot.ViewDistance != 8 {
		t.Errorf("ViewDistance = %d, want 8", cp.WorldProfileSnapshot.ViewDistance)
	}
	if cp.WorldProfileSnapshot.MinecraftVersion != "1.19.4" {
		t.Errorf("MinecraftVersion = %q, want 1.19.4", cp.WorldProfileSnapshot.MinecraftVersion)
	}
	if cp.WorldProfileSnapshot.CapturedFrom != "server.properties" {
		t.Errorf("CapturedFrom = %q, want server.properties", cp.WorldProfileSnapshot.CapturedFrom)
	}
}

func TestCreateCheckpointFallbackToRoomProfile(t *testing.T) {
	agent := &mockAgent{
		serverProperties: "",
	}
	wp, _ := worldprofile.New(worldprofile.CreateParams{
		ID:         "wp-1",
		Name:       "Room World",
		Seed:       "456",
		LevelType:  worldprofile.LevelDefault,
		Difficulty: worldprofile.DifficultyEasy,
	})
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		rooms: map[string]room.Room{
			"r-1": {ID: "r-1", DefaultWorldProfile: &wp},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID:                  "cp-fallback",
		SessionID:           "s-1",
		ActorID:             "actor-1",
		CaptureWorldProfile: true,
		AgentClient:         agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.WorldProfileSnapshot == nil {
		t.Fatal("expected world profile snapshot")
	}
	if cp.WorldProfileSnapshot.Seed != "456" {
		t.Errorf("Seed = %q, want 456 from room", cp.WorldProfileSnapshot.Seed)
	}
	if cp.WorldProfileSnapshot.CapturedFrom != "room" {
		t.Errorf("CapturedFrom = %q, want room", cp.WorldProfileSnapshot.CapturedFrom)
	}
}

func TestCreateCommandQuiescedRunsSequenceAndCreatesWorldSnapshot(t *testing.T) {
	agent := &mockAgent{}
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	cp, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-q", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel:    consistency.LevelCommandQuiesced,
		ConsistencyMetadata: map[string]string{"testKey": "testValue"},
		AgentClient:         agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.ConsistencyLevel != consistency.LevelCommandQuiesced {
		t.Fatalf("consistency level = %s", cp.ConsistencyLevel)
	}
	if cp.ConsistencyMetadata["worldSnapshot"] != "true" {
		t.Fatalf("worldSnapshot = %q", cp.ConsistencyMetadata["worldSnapshot"])
	}
	if cp.ConsistencyMetadata["testKey"] != "testValue" {
		t.Fatalf("user metadata not preserved: %v", cp.ConsistencyMetadata)
	}
	if cp.WorldStateRef == "" {
		t.Fatal("WorldStateRef should be set")
	}
	expectedCommands := []string{"save-off", "save-all flush", "save-on"}
	for i, want := range expectedCommands {
		if i >= len(agent.commands) || agent.commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q; all commands = %v", i, agent.commands[min(i, len(agent.commands)-1)], want, agent.commands)
		}
	}
	event := repo.auditEvents[0]
	if event.Metadata["worldSnapshot"] != "true" || event.Metadata["commandQuiesced"] != "true" {
		t.Fatalf("audit metadata: %+v", event.Metadata)
	}
	if event.Metadata["snapshotSHA256"] == "" || event.Metadata["snapshotSizeBytes"] == "" {
		t.Fatalf("audit missing snapshot fields: %+v", event.Metadata)
	}
}

func TestCreateCommandQuiescedSaveOnExecutesEvenWhenSaveAllFails(t *testing.T) {
	agent := &mockAgent{failAt: 2}
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-fail", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
		AgentClient:      agent,
	})
	if err == nil || !strings.Contains(err.Error(), "save-all") {
		t.Fatalf("expected save-all failure, got %v", err)
	}
	if len(agent.commands) < 3 || agent.commands[2] != "save-on" {
		t.Fatalf("save-on must execute even on failure: commands = %v", agent.commands)
	}
}

func TestCreateCommandQuiescedSaveOnAlwaysExecutes(t *testing.T) {
	agent := &mockAgent{failAt: 1}
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-fail2", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
		AgentClient:      agent,
	})
	if err == nil || !strings.Contains(err.Error(), "save-off") {
		t.Fatalf("expected save-off failure, got %v", err)
	}
	if len(agent.commands) < 2 || agent.commands[1] != "save-on" {
		t.Fatalf("save-on must execute even on save-off failure: commands = %v", agent.commands)
	}
}

func TestCreateCommandQuiescedSnapshotFailureStillExecutesSaveOnAndNoCheckpoint(t *testing.T) {
	agent := &mockAgent{failAt: 3}
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", RuntimeProfileID: "mcdr-python-1.17"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-snap-fail", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
		AgentClient:      agent,
	})
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("expected snapshot failure, got %v", err)
	}
	if len(agent.commands) < 3 || agent.commands[2] != "save-on" {
		t.Fatalf("save-on must execute on snapshot failure: commands = %v", agent.commands)
	}
	if len(repo.checkpoints) != 0 {
		t.Fatal("no checkpoint should be created on snapshot failure")
	}
}

func TestCreateCommandQuiescedRequiresAgent(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-noagent", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
	})
	if err == nil || !strings.Contains(err.Error(), "agent client") {
		t.Fatalf("expected agent required, got %v", err)
	}
	if len(repo.checkpoints) != 0 || len(repo.auditEvents) != 0 {
		t.Fatalf("writes after failure: checkpoints=%+v audits=%+v", repo.checkpoints, repo.auditEvents)
	}
}

func TestCreateRejectsUnsupportedNonMetadataOnlyConsistencyLevel(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-1", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel:    consistency.LevelBestEffort,
		ConsistencyMetadata: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected unsupported consistency level to fail")
	}
	if len(repo.checkpoints) != 0 || len(repo.auditEvents) != 0 {
		t.Fatalf("writes after failure: checkpoints=%+v audits=%+v", repo.checkpoints, repo.auditEvents)
	}
}

func TestCreateRejectsUnknownConsistencyLevel(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1"},
		},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-1", SessionID: "s-1", ActorID: "actor-1", ConsistencyLevel: consistency.Level("unknown"),
	})
	if err == nil {
		t.Fatal("expected unknown consistency level to fail")
	}
	if len(repo.checkpoints) != 0 || len(repo.auditEvents) != 0 {
		t.Fatalf("writes after failure: checkpoints=%+v audits=%+v", repo.checkpoints, repo.auditEvents)
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

func TestCreateCommandQuiescedRejectsAgentWithoutSendCommand(t *testing.T) {
	agent := &mockAgent{capabilities: []string{"prepare", "start", "stop"}}
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-1": {ID: "s-1", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", RuntimeProfileID: "mcdr-python-1.17"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-nosend", SessionID: "s-1", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
		AgentClient:      agent,
	})
	if err == nil || !strings.Contains(err.Error(), "send-command") {
		t.Fatalf("expected send-command rejection, got %v", err)
	}
	if len(repo.checkpoints) != 0 {
		t.Fatal("no checkpoint should be created")
	}
}

func TestCreateMetadataOnlyAuditAppendFailureReturnsError(t *testing.T) {
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-1": {ID: "s-1", ProjectID: "p-1", EnvironmentID: "env-1"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
		auditErr:    fmt.Errorf("audit storage failure"),
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-audit-fail", SessionID: "s-1", ActorID: "actor-1",
	})
	if err == nil || !strings.Contains(err.Error(), "audit append failed") {
		t.Fatalf("expected audit append error, got %v", err)
	}
	if len(repo.checkpoints) != 1 {
		t.Fatal("checkpoint should still be persisted")
	}
}

func TestCreateCommandQuiescedRequiresRuntimeProfileOrEnvironment(t *testing.T) {
	agent := &mockAgent{}
	repo := &mockRepo{
		sessions:    map[string]session.Session{"s-no-profile": {ID: "s-no-profile", ProjectID: "p-1", RoomID: "r-1"}},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Create(context.Background(), repo, CreateRequest{
		ID: "cp-noprofile", SessionID: "s-no-profile", ActorID: "actor-1",
		ConsistencyLevel: consistency.LevelCommandQuiesced,
		AgentClient:      agent,
	})
	if err == nil || !strings.Contains(err.Error(), "environment or runtime") {
		t.Fatalf("expected runtime profile or environment required, got %v", err)
	}
}

func TestRestoreCreatesNewCheckpoint(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
			},
		},
	}
	ctx := context.Background()
	agent := &mockAgent{}
	cp, err := Restore(ctx, repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.SourceSessionID != "s-target" {
		t.Fatalf("SourceSessionID = %q, want s-target", cp.SourceSessionID)
	}
	if cp.Metadata["restoredFromCheckpoint"] != "cp-source" {
		t.Fatalf("Metadata[restoredFromCheckpoint] = %q", cp.Metadata["restoredFromCheckpoint"])
	}
	if cp.WorldStateRef != "agent-local://mock/sessions/s-target/work/world_restored" {
		t.Fatalf("WorldStateRef = %q", cp.WorldStateRef)
	}
	if cp.ConsistencyLevel != consistency.LevelMetadataOnly {
		t.Fatalf("ConsistencyLevel = %q", cp.ConsistencyLevel)
	}
	if len(repo.auditEvents) != 1 || repo.auditEvents[0].Action != "checkpoint.restored" {
		t.Fatalf("audit events: %+v", repo.auditEvents)
	}
	auditMeta := repo.auditEvents[0].Metadata
	if auditMeta["sourceCheckpointId"] != "cp-source" || auditMeta["targetSessionId"] != "s-target" {
		t.Fatalf("audit metadata: %+v", auditMeta)
	}
	if auditMeta["worldDirRel"] != "world_restored" {
		t.Fatalf("worldDirRel = %q", auditMeta["worldDirRel"])
	}
}

func TestRestoreCheckpointCarriesLucyLockHash(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
				LucyLockHash:  "hash123",
			},
		},
	}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID: "cp-source", TargetSessionID: "s-target", ActorID: "actor-1", AgentClient: &mockAgent{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.LucyLockHash != "hash123" {
		t.Fatalf("LucyLockHash = %q, want hash123", cp.LucyLockHash)
	}
}

func TestRestoreRejectsCheckpointWithoutWorldState(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-target": {ID: "s-target", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-no-world": {
				ID: "cp-no-world", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelMetadataOnly, EnvironmentID: "env-1",
			},
		},
	}
	agent := &mockAgent{}
	_, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-no-world",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err == nil || !strings.Contains(err.Error(), "no world state") {
		t.Fatalf("expected no world state error, got %v", err)
	}
}

func TestRestoreRejectsTargetSessionInDifferentProject(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-target": {ID: "s-target", ProjectID: "p-other", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
			},
		},
	}
	agent := &mockAgent{}
	_, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected project mismatch error, got %v", err)
	}
}

func TestRestoreRequiresActorAndAgentClient(t *testing.T) {
	repo := &mockRepo{
		sessions:    map[string]session.Session{},
		checkpoints: map[string]checkpoint.Checkpoint{},
	}
	_, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "",
		AgentClient:     &mockAgent{},
	})
	if err == nil || !strings.Contains(err.Error(), "actor required") {
		t.Fatalf("expected actor required, got %v", err)
	}
	_, err = Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     nil,
	})
	if err == nil || !strings.Contains(err.Error(), "agent client required") {
		t.Fatalf("expected agent client required, got %v", err)
	}
}

func TestRestoreRejectsNonStoppedSession(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-running": {ID: "s-running", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateRunning},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-running", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-running/checkpoints/world.zip",
			},
		},
	}
	_, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-running",
		ActorID:         "actor-1",
		AgentClient:     &mockAgent{},
	})
	if err == nil || !strings.Contains(err.Error(), "must be stopped") || !strings.Contains(err.Error(), "JVM file locks") {
		t.Fatalf("expected JVM file lock error, got %v", err)
	}
}

func TestRestoreDefaultsWorldDirRelToWorldRestored(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
			},
		},
	}
	agent := &mockAgent{}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.Metadata["worldDirRel"] != "world_restored" {
		t.Fatalf("worldDirRel = %q, want world_restored", cp.Metadata["worldDirRel"])
	}
}

func TestRestoreAuditAppendFailureReturnsError(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
			},
		},
		auditErr: fmt.Errorf("audit storage failure"),
	}
	agent := &mockAgent{}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err == nil || !strings.Contains(err.Error(), "audit append failed") {
		t.Fatalf("expected audit append error, got %v", err)
	}
	if cp.ID == "" {
		t.Fatal("checkpoint should still be persisted")
	}
	if len(repo.checkpoints) != 2 {
		t.Fatalf("expected source and new checkpoint, got %d", len(repo.checkpoints))
	}
}

func TestRestoreGeneratesCheckpointIDWhenEmpty(t *testing.T) {
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef: "agent-local://mock/sessions/s-source/checkpoints/world.zip",
			},
		},
	}
	agent := &mockAgent{}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:    "cp-source",
		TargetSessionID: "s-target",
		ActorID:         "actor-1",
		AgentClient:     agent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cp.ID, "cp_") {
		t.Fatalf("generated checkpoint ID = %q", cp.ID)
	}
	if cp.ID == "cp-source" {
		t.Fatalf("new checkpoint ID should differ from source ID, got %q", cp.ID)
	}
	if cp.SourceSessionID != "s-target" {
		t.Fatalf("SourceSessionID = %q", cp.SourceSessionID)
	}
}

func TestRestoreAppliesWorldProfile(t *testing.T) {
	worldSnapshot := &checkpoint.WorldProfileSnapshot{
		Seed:               "999000",
		LevelType:          "flat",
		Difficulty:         "peaceful",
		GenerateStructures: false,
		SpawnRadius:        5,
		ViewDistance:       10,
	}
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef:        "agent-local://mock/sessions/s-source/checkpoints/world.zip",
				WorldProfileSnapshot: worldSnapshot,
			},
		},
	}
	agent := &mockAgent{}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:      "cp-source",
		TargetSessionID:   "s-target",
		ActorID:           "actor-1",
		AgentClient:       agent,
		ApplyWorldProfile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.calls) < 2 {
		t.Fatalf("expected at least 2 agent calls, got %d", len(agent.calls))
	}
	writeFound := false
	for _, call := range agent.calls {
		if call == "write_session_file" {
			writeFound = true
			break
		}
	}
	if !writeFound {
		t.Fatalf("write_session_file not called, calls: %v", agent.calls)
	}
	if cp.ID == "" {
		t.Fatal("checkpoint ID is empty")
	}
}

func TestRestoreAppliesPartialWorldProfile(t *testing.T) {
	worldSnapshot := &checkpoint.WorldProfileSnapshot{
		Seed:         "888",
		LevelType:    "flat",
		Difficulty:   "hard",
		ViewDistance: 12,
	}
	repo := &mockRepo{
		sessions: map[string]session.Session{
			"s-source": {ID: "s-source", ProjectID: "p-1", EnvironmentID: "env-1", State: session.StateStopped},
			"s-target": {ID: "s-target", ProjectID: "p-1", RoomID: "r-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"cp-source": {
				ID: "cp-source", ProjectID: "p-1", SourceSessionID: "s-source", CreatorID: "creator-1",
				Kind: checkpoint.KindManual, Status: checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelCommandQuiesced, EnvironmentID: "env-1",
				WorldStateRef:        "agent-local://mock/sessions/s-source/checkpoints/world.zip",
				WorldProfileSnapshot: worldSnapshot,
			},
		},
	}
	agent := &mockAgent{
		serverProperties: "level-seed=original\nlevel-type=default\ndifficulty=easy\n",
	}
	cp, err := Restore(context.Background(), repo, RestoreRequest{
		CheckpointID:            "cp-source",
		TargetSessionID:         "s-target",
		ActorID:                 "actor-1",
		AgentClient:             agent,
		ApplyWorldProfile:       true,
		ApplyWorldProfileFields: []string{"seed", "level-type"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == "" {
		t.Fatal("checkpoint ID is empty")
	}
	writeFound := false
	for _, call := range agent.calls {
		if call == "write_session_file" {
			writeFound = true
			break
		}
	}
	if !writeFound {
		t.Fatalf("write_session_file not called, calls: %v", agent.calls)
	}
	if len(agent.calls) != 3 {
		t.Fatalf("expected 3 calls (read+restore+write), got %d: %v", len(agent.calls), agent.calls)
	}
}
