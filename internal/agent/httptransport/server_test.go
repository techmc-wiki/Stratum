package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestServerHealthAndInfo(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewFake(), "", nil).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get(requestIDHeader) == "" {
		t.Fatalf("health status=%d requestID=%q", response.StatusCode, response.Header.Get(requestIDHeader))
	}

	response, err = http.Get(server.URL + "/v1/agent")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var info AgentInfoResponse
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.ID != local.DefaultAgentID || info.Status != "available" || len(info.Capabilities) == 0 || info.RequestID == "" {
		t.Fatalf("info = %+v", info)
	}
}

func TestServerTokenRequiredMissingAndAccepted(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewFake(), "secret", nil).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/agent")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var failure ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure.RequestID == "" || !strings.Contains(failure.Error, "bearer token") {
		t.Fatalf("failure = %+v", failure)
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/agent", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authorized status = %d", response.StatusCode)
	}
}

func TestServerPreservesRequestIDInError(t *testing.T) {
	fake := local.NewFake()
	fake.SetFailure(agent.OperationStart, "planned failure")
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/session-1/start", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(requestIDHeader, "request-test-1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict || response.Header.Get(requestIDHeader) != "request-test-1" {
		t.Fatalf("status=%d header=%q", response.StatusCode, response.Header.Get(requestIDHeader))
	}
	var failure ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if failure.RequestID != "request-test-1" || failure.AgentID != local.DefaultAgentID {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestServerProcessAgentInspectAndLogs(t *testing.T) {
	server := httptest.NewServer(NewServer(local.NewProcessAgent(), "", nil).Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/session-1/start", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/v1/sessions/session-1/inspect")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var status SessionInspectResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.ProcessID == "" || status.RuntimeMode != "dummy-process" {
		t.Fatalf("status=%+v", status)
	}
	response, err = http.Get(server.URL + "/v1/sessions/session-1/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var logs LogsResponse
	if err := json.NewDecoder(response.Body).Decode(&logs); err != nil {
		t.Fatal(err)
	}
	if len(logs.Lines) < 2 || !logsContain(logs.Lines, "dummy-runtime") {
		t.Fatalf("logs=%+v", logs)
	}
}

func TestServerListsRuntimeProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-profiles.json")
	content := `{"runtime_profiles":[{"id":"trusted-terminal","name":"Trusted terminal","runtime_type":"terminal","command_argv":["server"],"working_dir":".","env":{"SECRET":"not-public"},"stop_strategy":"terminate","graceful_stop_timeout":"1s","force_kill_timeout":"1s","log_mode":"combined","enabled":true}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := runtimeprofile.LoadTrustedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	registry := runtimeprofile.Builtins()
	if err := registry.RegisterAll(profiles); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(local.NewProcessAgentWithRegistry(local.DefaultAgentID, registry), "", nil).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/agent/runtime-profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload RuntimeProfilesResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.AgentID != local.DefaultAgentID || len(payload.Profiles) != 2 || payload.Profiles[0].ID != "dummy-process" || payload.Profiles[1].ID != "trusted-terminal" {
		t.Fatalf("payload=%+v", payload)
	}
	if len(payload.Profiles[1].CommandArgv) != 0 || payload.Profiles[1].WorkingDir != "" || payload.Profiles[1].Env != nil {
		t.Fatalf("trusted profile leaked private configuration: %+v", payload.Profiles[1])
	}
}

func TestServerLogsMaxBytes(t *testing.T) {
	runtime := local.NewProcessAgent()
	_, _ = runtime.StartSession(context.Background(), agent.SessionRequest{SessionID: "limited-http"})
	server := httptest.NewServer(NewServer(runtime, "", nil).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/sessions/limited-http/logs?maxBytes=24")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var logs LogsResponse
	if err := json.NewDecoder(response.Body).Decode(&logs); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, line := range logs.Lines {
		total += len(line) + 1
	}
	if total > 24 || total == 0 {
		t.Fatalf("logs=%+v bytes=%d", logs, total)
	}
}

func TestServerSendCommand(t *testing.T) {
	fake := local.NewFake()
	_, _ = fake.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-1"})
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()

	body := strings.NewReader(`{"command":"save-all"}`)
	response, err := http.Post(server.URL+"/v1/sessions/session-1/send-command", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	var result SendCommandResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.SessionID != "session-1" {
		t.Fatalf("result=%+v", result)
	}

	empty := strings.NewReader(`{"command":""}`)
	response, err = http.Post(server.URL+"/v1/sessions/session-1/send-command", "application/json", empty)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var errResp ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errResp.Error, "required") {
		t.Fatalf("err=%s", errResp.Error)
	}
}

func TestServerCreateWorldSnapshot(t *testing.T) {
	fake := local.NewFake()
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()

	body := strings.NewReader(`{"worldDirRel":"world"}`)
	response, err := http.Post(server.URL+"/v1/sessions/session-1/world-snapshot", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	var result WorldCheckpointResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.SnapshotRef == "" || result.SHA256 == "" || result.SessionID != "session-1" {
		t.Fatalf("result=%+v", result)
	}
}
