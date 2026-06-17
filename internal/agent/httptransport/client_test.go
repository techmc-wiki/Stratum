package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
)

func TestClientStartSessionSuccess(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewFake(), "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	result, err := client.StartSession(WithRequestID(context.Background(), "client-request-1"), agent.SessionRequest{SessionID: "session-1", ProjectID: "project-1", EnvironmentID: "environment-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.AgentID != local.DefaultAgentID || result.Status != "success" || result.Message != "running" || result.Mode != "http" {
		t.Fatalf("result = %+v", result)
	}
	status, err := client.InspectSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.Status != "running" {
		t.Fatalf("status = %+v", status)
	}
}

func TestClientReturnsProcessRuntimeStatus(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewProcessAgent(), "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	if _, err := client.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-process"}); err != nil {
		t.Fatal(err)
	}
	status, err := client.InspectSession(context.Background(), "session-process")
	if err != nil || !status.Running || status.ProcessID == "" || status.RuntimeMode != "dummy-process" || status.StartedAt == nil {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	logs, err := client.CollectLogs(context.Background(), "session-process")
	if err != nil || len(logs.Lines) < 2 || !logsContain(logs.Lines, "dummy-runtime") {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
}

func logsContain(lines []string, text string) bool {
	for _, line := range lines {
		if strings.Contains(line, text) {
			return true
		}
	}
	return false
}

func TestClientListsRuntimeProfiles(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewProcessAgent(), "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	profiles, err := client.RuntimeProfiles(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].ID != "dummy-process" {
		t.Fatalf("profiles=%+v err=%v", profiles, err)
	}
}

func TestClientHandlesAgentFailure(t *testing.T) {
	fake := local.NewFake()
	fake.SetFailure(agent.OperationStart, "planned failure")
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	_, err := client.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-1"})
	var httpErr HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusConflict || httpErr.AgentID != local.DefaultAgentID || httpErr.RequestID == "" || !strings.Contains(httpErr.Message, "planned failure") {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientHandlesArbitraryNon2xxAndMalformedResponse(t *testing.T) {
	non2xx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusTeapot, ErrorResponse{Error: "teapot", Operation: "start", RequestID: "request-teapot"})
	}))
	defer non2xx.Close()
	client := newTestClient(t, non2xx.URL, "")
	_, err := client.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-1"})
	var httpErr HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusTeapot || httpErr.Message != "teapot" {
		t.Fatalf("error = %#v", err)
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }))
	defer malformed.Close()
	client = newTestClient(t, malformed.URL, "")
	if _, err := client.Info(context.Background()); err == nil || !strings.Contains(err.Error(), "decode agent response") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientTokenAccepted(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewFake(), "secret", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "secret")
	if _, err := client.Info(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClientMaterializesArtifactThroughHTTP(t *testing.T) {
	runtime, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(runtime, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	payload := []byte("artifact")
	result, err := client.MaterializeArtifact(context.Background(), agent.ArtifactMaterializationRequest{SessionID: "session-1", ArtifactID: "artifact-1", StagingPlanID: "plan-1", ArtifactName: "Test", ArtifactType: "jar", TargetName: "test.jar", PayloadAlgorithm: "sha256", PayloadHash: "c7c5c1d70c5dec4416ab6158afd0b223ef40c29b1dc1f97ed9428b94d4cadb1c", PayloadSize: int64(len(payload)), ActorID: "actor-1", Payload: payload})
	if err != nil || result.Status != "materialized" || result.AgentID != local.DefaultAgentID || result.RuntimeRelativePath != "artifacts/test.jar" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	inspected, err := client.InspectMaterializedArtifacts(context.Background(), "session-1")
	if err != nil || inspected.Status != "available" || len(inspected.Items) != 1 || inspected.Items[0].ArtifactID != "artifact-1" || inspected.Items[0].RuntimeRelativePath != "artifacts/test.jar" {
		t.Fatalf("inspected=%+v err=%v", inspected, err)
	}
	item, err := client.InspectMaterializedArtifact(context.Background(), "session-1", "plan-1")
	if err != nil || item.ArtifactID != "artifact-1" || item.StagingPlanID != "plan-1" || item.RuntimeRelativePath != "artifacts/test.jar" {
		t.Fatalf("item=%+v err=%v", item, err)
	}
	_, err = client.InspectMaterializedArtifact(context.Background(), "session-1", "missing-plan")
	var httpErr HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound || !strings.Contains(httpErr.Message, "materialized artifact not found") {
		t.Fatalf("missing item err=%#v", err)
	}
	verified, err := client.VerifyMaterializedArtifact(context.Background(), "session-1", "plan-1")
	if err != nil || verified.Status != "valid" || verified.ExpectedHash != verified.ActualHash || verified.ActualSize != int64(len(payload)) {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
	verifiedAll, err := client.VerifyMaterializedArtifacts(context.Background(), "session-1")
	if err != nil || verifiedAll.Total != 1 || verifiedAll.ValidCount != 1 || verifiedAll.MissingCount != 0 || verifiedAll.CorruptedCount != 0 || verifiedAll.ErrorCount != 0 || len(verifiedAll.Entries) != 1 || verifiedAll.Entries[0].Status != "valid" {
		t.Fatalf("verified all=%+v err=%v", verifiedAll, err)
	}
}

func TestClientMaterializedArtifactsMissingManifestIsEmpty(t *testing.T) {
	runtime, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(runtime, "", nil).Handler())
	defer server.Close()
	result, err := newTestClient(t, server.URL, "").InspectMaterializedArtifacts(context.Background(), "session-1")
	if err != nil || result.Status != "empty" || len(result.Items) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	verified, err := newTestClient(t, server.URL, "").VerifyMaterializedArtifacts(context.Background(), "session-1")
	if err != nil || verified.Total != 0 || len(verified.Entries) != 0 {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
}

func TestClientMaterializedArtifactMalformedManifestReturnsStructuredError(t *testing.T) {
	runtimeRoot := t.TempDir()
	runtime, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(runtimeRoot, "sessions", "session-1", "artifacts", "staged-artifacts.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("not-json"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(runtime, "", nil).Handler())
	defer server.Close()
	_, err = newTestClient(t, server.URL, "").InspectMaterializedArtifact(context.Background(), "session-1", "plan-1")
	var httpErr HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest || !strings.Contains(httpErr.Message, "decode staging manifest") {
		t.Fatalf("err=%#v", err)
	}
	_, err = newTestClient(t, server.URL, "").VerifyMaterializedArtifacts(context.Background(), "session-1")
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest || !strings.Contains(httpErr.Message, "decode staging manifest") {
		t.Fatalf("verify all err=%#v", err)
	}
}

func TestClientSendCommand(t *testing.T) {
	fake := local.NewFake()
	_, _ = fake.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-1"})
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	result, err := client.SendCommand(context.Background(), "session-1", "save-all")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.AgentID != local.DefaultAgentID {
		t.Fatalf("result=%+v", result)
	}
	_, err = client.SendCommand(context.Background(), "session-1", "")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty command err=%v", err)
	}
}

func newTestClient(t *testing.T, rawURL, token string) *Client {
	t.Helper()
	client, err := NewClient(rawURL, token, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientCreateWorldSnapshot(t *testing.T) {
	fake := local.NewFake()
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	result, err := client.CreateWorldSnapshot(context.Background(), agent.WorldCheckpointRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotRef == "" || result.SessionID != "session-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestClientRestoreWorldSnapshot(t *testing.T) {
	fake := local.NewFake()
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	result, err := client.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://local/sessions/session-1/checkpoints/world.zip",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoredRef == "" || result.SessionID != "session-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestClientRestoreWorldSnapshotFailure(t *testing.T) {
	fake := local.NewFake()
	fake.SetFailure(agent.OperationRestoreWorldSnapshot, "planned restore failure")
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client := newTestClient(t, server.URL, "")
	_, err := client.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://local/sessions/session-1/checkpoints/world.zip",
	})
	if err == nil || !strings.Contains(err.Error(), "planned restore failure") {
		t.Fatalf("err=%v", err)
	}
}
