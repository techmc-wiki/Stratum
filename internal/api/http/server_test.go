package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	NewServer().Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestAuthenticatedHandlerRequiresToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	NewServer().WithToken("secret").AuthenticatedHandler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	NewServer().WithToken("secret").AuthenticatedHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized status = %d", response.Code)
	}
}

func TestRestoreCheckpointSuccess(t *testing.T) {
	repo := &mockCheckpointRepo{
		sessions: map[string]session.Session{
			"session-target": {ID: "session-target", ProjectID: "project-1", RoomID: "room-1", EnvironmentID: "env-1", State: session.StateStopped},
		},
		checkpoints: map[string]checkpoint.Checkpoint{
			"checkpoint-source": {
				ID:               "checkpoint-source",
				ProjectID:        "project-1",
				RoomID:           "room-1",
				SourceSessionID:  "session-source",
				CreatorID:        "actor-original",
				Kind:             checkpoint.KindManual,
				Status:           checkpoint.StatusMetadataOnly,
				ConsistencyLevel: consistency.LevelMetadataOnly,
				EnvironmentID:    "env-1",
				WorldStateRef:    "agent-local://mock/sessions/session-source/checkpoints/world.zip",
			},
		},
	}
	server := httptest.NewServer(NewServerWithServices(repo, &mockCheckpointAgent{}).Handler())
	t.Cleanup(server.Close)

	body := []byte(`{"checkpointId":"checkpoint-source","targetSessionId":"session-target","actorId":"actor-1"}`)
	response, err := http.Post(server.URL+"/v1/checkpoints/restore", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		var errBody map[string]string
		json.NewDecoder(response.Body).Decode(&errBody)
		t.Fatalf("status = %d, want %d, error: %v", response.StatusCode, http.StatusOK, errBody)
	}
	var payload CheckpointRestoreResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.CheckpointID == "" || payload.CheckpointID == "checkpoint-source" {
		t.Fatalf("checkpointId = %q, want generated restored checkpoint id", payload.CheckpointID)
	}
	if payload.WorldStateRef != "agent-local://mock/sessions/session-target/work/world_restored" {
		t.Fatalf("worldStateRef = %q", payload.WorldStateRef)
	}
}

func TestRestoreCheckpointRejectsMissingFields(t *testing.T) {
	server := httptest.NewServer(NewServerWithServices(&mockCheckpointRepo{}, &mockCheckpointAgent{}).Handler())
	t.Cleanup(server.Close)

	body := []byte(`{"checkpointId":"checkpoint-source","actorId":"actor-1"}`)
	response, err := http.Post(server.URL+"/v1/checkpoints/restore", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

type mockCheckpointRepo struct {
	sessions    map[string]session.Session
	checkpoints map[string]checkpoint.Checkpoint
	auditEvents []audit.Event
}

func (m *mockCheckpointRepo) GetSession(_ context.Context, id string) (session.Session, error) {
	value, ok := m.sessions[id]
	if !ok {
		return session.Session{}, fmt.Errorf("session %q not found", id)
	}
	return value, nil
}

func (m *mockCheckpointRepo) CreateCheckpoint(_ context.Context, value checkpoint.Checkpoint) error {
	if m.checkpoints == nil {
		m.checkpoints = map[string]checkpoint.Checkpoint{}
	}
	m.checkpoints[value.ID] = value
	return nil
}

func (m *mockCheckpointRepo) UpdateCheckpoint(_ context.Context, value checkpoint.Checkpoint) error {
	if m.checkpoints == nil {
		m.checkpoints = map[string]checkpoint.Checkpoint{}
	}
	m.checkpoints[value.ID] = value
	return nil
}

func (m *mockCheckpointRepo) GetCheckpoint(_ context.Context, id string) (checkpoint.Checkpoint, error) {
	value, ok := m.checkpoints[id]
	if !ok {
		return checkpoint.Checkpoint{}, fmt.Errorf("checkpoint %q not found", id)
	}
	return value, nil
}

func (m *mockCheckpointRepo) ListCheckpoints(_ context.Context) ([]checkpoint.Checkpoint, error) {
	result := make([]checkpoint.Checkpoint, 0, len(m.checkpoints))
	for _, value := range m.checkpoints {
		result = append(result, value)
	}
	return result, nil
}

func (m *mockCheckpointRepo) ListCheckpointsBySession(_ context.Context, sessionID string) ([]checkpoint.Checkpoint, error) {
	var result []checkpoint.Checkpoint
	for _, value := range m.checkpoints {
		if value.SourceSessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (m *mockCheckpointRepo) AppendAuditEvent(_ context.Context, event audit.Event) error {
	m.auditEvents = append(m.auditEvents, event)
	return nil
}

func (m *mockCheckpointRepo) GetRoom(_ context.Context, id string) (room.Room, error) {
	return room.Room{}, fmt.Errorf("room %q not found", id)
}

type mockCheckpointAgent struct{}

func (m *mockCheckpointAgent) Info(context.Context) (agent.AgentInfo, error) {
	return agent.AgentInfo{ID: "mock", Capabilities: []string{"restore-world-snapshot"}}, nil
}

func (m *mockCheckpointAgent) RuntimeProfiles(context.Context) ([]runtimeprofile.Profile, error) {
	return nil, nil
}

func (m *mockCheckpointAgent) PrepareSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) StartSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) StopSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) RestartSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) FreezeSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) UnfreezeSession(context.Context, agent.SessionRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) InspectSession(context.Context, string) (agent.SessionStatus, error) {
	return agent.SessionStatus{}, nil
}

func (m *mockCheckpointAgent) CollectLogs(context.Context, string) (agent.LogBatch, error) {
	return agent.LogBatch{}, nil
}

func (m *mockCheckpointAgent) ReportResources(context.Context) (agent.ResourceReport, error) {
	return agent.ResourceReport{}, nil
}

func (m *mockCheckpointAgent) CreateCheckpointStub(context.Context, agent.CheckpointRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) RestoreCheckpointStub(context.Context, agent.CheckpointRequest) (agent.OperationResult, error) {
	return agent.OperationResult{}, nil
}

func (m *mockCheckpointAgent) MaterializeArtifact(context.Context, agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	return agent.ArtifactMaterializationResult{}, nil
}

func (m *mockCheckpointAgent) InspectMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifacts, error) {
	return agent.MaterializedArtifacts{}, nil
}

func (m *mockCheckpointAgent) InspectMaterializedArtifact(context.Context, string, string) (agent.MaterializedArtifact, error) {
	return agent.MaterializedArtifact{}, nil
}

func (m *mockCheckpointAgent) VerifyMaterializedArtifact(context.Context, string, string) (agent.MaterializedArtifactVerification, error) {
	return agent.MaterializedArtifactVerification{}, nil
}

func (m *mockCheckpointAgent) VerifyMaterializedArtifacts(context.Context, string) (agent.MaterializedArtifactsVerification, error) {
	return agent.MaterializedArtifactsVerification{}, nil
}

func (m *mockCheckpointAgent) DryRunArtifactApply(context.Context, agent.ArtifactApplyDryRunRequest) (agent.ArtifactApplyDryRunResult, error) {
	return agent.ArtifactApplyDryRunResult{}, nil
}

func (m *mockCheckpointAgent) ExecuteArtifactApply(context.Context, agent.ArtifactApplyExecuteRequest) (agent.ArtifactApplyExecuteResult, error) {
	return agent.ArtifactApplyExecuteResult{}, nil
}

func (m *mockCheckpointAgent) ListAppliedArtifacts(context.Context, string) (agent.AppliedArtifactsResponse, error) {
	return agent.AppliedArtifactsResponse{}, nil
}

func (m *mockCheckpointAgent) InspectAppliedArtifact(context.Context, string, string) (agent.AppliedArtifactRecord, error) {
	return agent.AppliedArtifactRecord{}, nil
}

func (m *mockCheckpointAgent) VerifyAppliedArtifact(context.Context, string, string) (agent.AppliedArtifactVerification, error) {
	return agent.AppliedArtifactVerification{}, nil
}

func (m *mockCheckpointAgent) VerifyAllAppliedArtifacts(context.Context, string) (agent.BatchAppliedArtifactVerification, error) {
	return agent.BatchAppliedArtifactVerification{}, nil
}

func (m *mockCheckpointAgent) MaterializeEnvironment(context.Context, agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	return agent.EnvironmentMaterializationResult{}, nil
}

func (m *mockCheckpointAgent) GetSessionRuntimeStatus(context.Context, string) (agent.SessionRuntimeStatus, error) {
	return agent.SessionRuntimeStatus{}, nil
}

func (m *mockCheckpointAgent) SessionReadyForStart(context.Context, string) (agent.SessionStartReadiness, error) {
	return agent.SessionStartReadiness{}, nil
}

func (m *mockCheckpointAgent) InspectMCDRConfigStub(context.Context, string) (agent.MCDRConfigStubInspection, error) {
	return agent.MCDRConfigStubInspection{}, nil
}

func (m *mockCheckpointAgent) SendCommand(context.Context, string, string) (agent.CommandResult, error) {
	return agent.CommandResult{}, nil
}

func (m *mockCheckpointAgent) CreateWorldSnapshot(context.Context, agent.WorldCheckpointRequest) (agent.WorldCheckpointResult, error) {
	return agent.WorldCheckpointResult{}, nil
}

func (m *mockCheckpointAgent) RestoreWorldSnapshot(_ context.Context, request agent.WorldCheckpointRestoreRequest) (agent.WorldCheckpointRestoreResult, error) {
	return agent.WorldCheckpointRestoreResult{
		SessionID:   request.SessionID,
		RestoredRef: "agent-local://mock/sessions/" + request.SessionID + "/work/" + request.WorldDirRel,
		EntryCount:  5,
		SizeBytes:   4096,
		RestoredAt:  time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (m *mockCheckpointAgent) ReadSessionFile(_ context.Context, sessionID, relPath string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCheckpointAgent) WriteSessionFile(_ context.Context, sessionID, relPath string, content []byte) error {
	return fmt.Errorf("not implemented")
}

var _ agent.AgentClient = (*mockCheckpointAgent)(nil)
