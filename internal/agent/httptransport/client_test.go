package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func newTestClient(t *testing.T, rawURL, token string) *Client {
	t.Helper()
	client, err := NewClient(rawURL, token, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
