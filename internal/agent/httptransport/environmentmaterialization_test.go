package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
)

func TestMaterializeEnvironmentHTTP(t *testing.T) {
	fake := local.NewFake()
	server := httptest.NewServer(NewServer(fake, "", nil).Handler())
	defer server.Close()
	client, err := NewClient(server.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "session-http",
		EnvironmentID:          "env-http",
		EnvironmentName:        "HTTP Environment",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.12.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
	result, err := client.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.SessionID != "session-http" {
		t.Errorf("session id: got %q, want %q", result.SessionID, "session-http")
	}
	if result.EnvironmentID != "env-http" {
		t.Errorf("environment id: got %q, want %q", result.EnvironmentID, "env-http")
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want %q", result.Status, "prepared")
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1", len(calls))
	}
	if calls[0] != agent.OperationMaterializeEnvironment {
		t.Errorf("operation: got %q, want %q", calls[0], agent.OperationMaterializeEnvironment)
	}
}

func TestMaterializeEnvironmentServerDirect(t *testing.T) {
	fake := local.NewFake()
	server := NewServer(fake, "", nil)
	requestBody := EnvironmentMaterializationRequest{
		SessionID:              "session-direct",
		EnvironmentID:          "env-direct",
		EnvironmentName:        "Direct Environment",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.12.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/environments/materialize", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code: got %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response EnvironmentMaterializationResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.SessionID != "session-direct" {
		t.Errorf("session id: got %q, want %q", response.SessionID, "session-direct")
	}
	if response.Status != "prepared" {
		t.Errorf("status: got %q, want %q", response.Status, "prepared")
	}
}
