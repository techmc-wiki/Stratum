package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/repository/filesystem"
)

func TestCreateSharedSessionRequiresRoom(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	if code := Run([]string{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create project: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1"}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "require --room") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLIUsesHTTPAgentWhenConfigured(t *testing.T) {
	server := httptest.NewServer(httptransport.NewServer(local.NewFake(), "secret", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	base := []string{"--data-dir", dataDirectory, "--agent-url", server.URL, "--agent-token", "secret"}
	commands := [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"sessions", "start", "--id", "session-1", "--actor", "actor-1"},
		{"sessions", "inspect", "--id", "session-1"},
		{"sessions", "logs", "--id", "session-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		args := append(append([]string{}, base...), command...)
		if code := Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.State != "running" || value.RuntimeEndpoint != server.URL {
		t.Fatalf("session = %+v", value)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events = sessionAuditEvents(events)
	if len(events) != 1 || events[0].Metadata["agentMode"] != "http" {
		t.Fatalf("events = %+v", events)
	}
}

func TestCLIObservesRuntimeWithoutMutatingSession(t *testing.T) {
	server := httptest.NewServer(httptransport.NewServer(local.NewProcessAgent(), "", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	base := []string{"--data-dir", dataDirectory, "--agent-url", server.URL}
	commands := [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"sessions", "prepare", "--id", "session-1", "--actor", "actor-1"},
		{"sessions", "start", "--id", "session-1", "--actor", "actor-1", "--runtime-profile", "dummy-process"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "sessions", "observe", "--id", "session-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("observe running: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "controllerState=running") || !strings.Contains(stdout.String(), "agentStatus=running") || !strings.Contains(stdout.String(), "mismatch=false") || !strings.Contains(stdout.String(), "recommendedAction=none") {
		t.Fatalf("running observation=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "sessions", "stop", "--id", "session-1", "--actor", "actor-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("stop: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "sessions", "observe", "--id", "session-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("observe stopped: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "controllerState=stopped") || !strings.Contains(stdout.String(), "agentStatus=stopped") || !strings.Contains(stdout.String(), "mismatch=false") {
		t.Fatalf("stopped observation=%q", stdout.String())
	}

	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "stopped" {
		t.Fatalf("session=%+v err=%v", value, err)
	}
}

func TestCLIRuntimeProfilesAndLogLimit(t *testing.T) {
	server := httptest.NewServer(httptransport.NewServer(local.NewProcessAgent(), "", nil).Handler())
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--data-dir", filepath.Join(t.TempDir(), "data"), "--agent-url", server.URL, "agents", "runtime-profiles", "--id", "local"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "dummy-process") {
		t.Fatalf("profiles: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	client, err := httptransport.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-log-limit", RuntimeProfileID: "dummy-process"}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--data-dir", filepath.Join(t.TempDir(), "data"), "--agent-url", server.URL, "sessions", "logs", "--id", "session-log-limit", "--max-bytes", "20"}, &stdout, &stderr)
	if code != 0 || stdout.Len() != 20 {
		t.Fatalf("logs: code=%d bytes=%d stdout=%q stderr=%q", code, stdout.Len(), stdout.String(), stderr.String())
	}
}

func TestCheckpointListAndGet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"--data-dir", dataDirectory, "checkpoints", "create", "--id", "checkpoint-1", "--session", "session-1", "--note", "before test"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(command, &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "checkpoints", "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("list: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "checkpoint-1\tsession-1\tmanual\tbefore test") {
		t.Fatalf("list stdout=%q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "checkpoints", "get", "--id", "checkpoint-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("get: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "checkpoint-1\tsession-1") {
		t.Fatalf("get stdout=%q", stdout.String())
	}
}

func TestLifecycleCLIUpdatesPersistentSession(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"--data-dir", dataDirectory, "sessions", "start", "--id", "session-1", "--actor", "actor-1"},
		{"--data-dir", dataDirectory, "sessions", "freeze", "--id", "session-1", "--actor", "actor-1"},
		{"--data-dir", dataDirectory, "sessions", "unfreeze", "--id", "session-1", "--actor", "actor-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(command, &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.State != "running" {
		t.Fatalf("state = %s", value.State)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events = sessionAuditEvents(events)
	if len(events) != 3 {
		t.Fatalf("events = %d", len(events))
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "operations", "list", "--session", "session-1"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "succeeded") {
		t.Fatalf("operations list: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func sessionAuditEvents(values []audit.Event) []audit.Event {
	result := make([]audit.Event, 0, len(values))
	for _, value := range values {
		if value.TargetType == "session" {
			result = append(result, value)
		}
	}
	return result
}

func TestAgentAndSessionInspectionCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"--data-dir", dataDirectory, "sessions", "start", "--id", "session-1", "--actor", "actor-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(command, &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "agents", "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("agents list: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "local\tavailable\tlocal://agent/local") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "agents", "inspect", "--id", "local"}, &stdout, &stderr); code != 0 {
		t.Fatalf("agents inspect: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "cpu=8") || !strings.Contains(stdout.String(), "memory=2048/16384MB") || !strings.Contains(stdout.String(), "disk=32768/262144MB") || !strings.Contains(stdout.String(), "capabilities=") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "sessions", "inspect", "--id", "session-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("session inspect: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "project=project-1") || !strings.Contains(stdout.String(), "room=room-1") || !strings.Contains(stdout.String(), "type=shared") || !strings.Contains(stdout.String(), "state=running") || !strings.Contains(stdout.String(), "agent=local") || !strings.Contains(stdout.String(), "agentStatus=success") || !strings.Contains(stdout.String(), "runtimeMessage=\"running\"") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "sessions", "logs", "--id", "session-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("session logs: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "no real JVM process was started") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "agents", "resources", "--id", "local"}, &stdout, &stderr); code != 0 {
		t.Fatalf("agent resources: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "disk=32768/262144MB") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestProjectPersistsAcrossRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	if code := Run([]string{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project One"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "projects", "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("list: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "project-1\tProject One") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
