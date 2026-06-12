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
	"github.com/stratummc/stratum/internal/domain/artifact"
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
	if !strings.Contains(stdout.String(), "persisted=true") {
		t.Fatalf("running observation was not persisted: %q", stdout.String())
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
	observations, err := store.ListRuntimeObservationsBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 2 || observations[0].SessionID != "session-1" || observations[1].SessionID != "session-1" {
		t.Fatalf("observations=%+v", observations)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var observationEvents int
	for _, event := range events {
		if event.Action == "runtime.observation.created" && event.TargetType == "runtime-observation" && event.Metadata["sessionId"] == "session-1" {
			observationEvents++
		}
	}
	if observationEvents != 2 {
		t.Fatalf("observation audit events=%d events=%+v", observationEvents, events)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "runtime-observations", "list", "--session", "session-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("observations list: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "session-1") || !strings.Contains(stdout.String(), "\tnone\tinfo\tnone\t") {
		t.Fatalf("observations list stdout=%q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "runtime-observations", "inspect", "--id", observations[0].ID), &stdout, &stderr); code != 0 {
		t.Fatalf("observations inspect: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "id="+observations[0].ID) || !strings.Contains(stdout.String(), "session=session-1") {
		t.Fatalf("observations inspect stdout=%q", stdout.String())
	}
}

func TestCLIManualMarkStoppedReconciliation(t *testing.T) {
	runtime := local.NewProcessAgent()
	server := httptest.NewServer(httptransport.NewServer(runtime, "", nil).Handler())
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
	reconcile := append(append([]string{}, base...), "sessions", "reconcile", "mark-stopped", "--id", "session-1", "--actor", "actor-1", "--reason", "operator confirmed metadata repair", "--request-id", "request-reconcile-1")
	if code := Run(reconcile, &stdout, &stderr); code != 0 {
		t.Fatalf("reconcile: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=succeeded") || !strings.Contains(stdout.String(), "observationAvailable=true") {
		t.Fatalf("stdout=%q", stdout.String())
	}

	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "stopped" || value.LastRuntimeMessage != "manually reconciled as stopped" {
		t.Fatalf("session=%+v err=%v", value, err)
	}
	status, err := runtime.InspectSession(context.Background(), "session-1")
	if err != nil || !status.Running {
		t.Fatalf("manual reconciliation must not stop runtime: status=%+v err=%v", status, err)
	}
	operations, err := store.ListOperationsBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, operationValue := range operations {
		if operationValue.Action == "session.reconcile.mark-stopped" {
			found = operationValue.Status == "succeeded" && operationValue.RequestID == "request-reconcile-1" && operationValue.Metadata["mismatchType"] == "none" && operationValue.Metadata["observationWarning"] != ""
		}
	}
	if !found {
		t.Fatalf("reconciliation operation missing: %+v", operations)
	}
}

func TestCLIManualMarkStoppedRequiresActorAndReason(t *testing.T) {
	for _, args := range [][]string{
		{"sessions", "reconcile", "mark-stopped", "--id", "session-1", "--reason", "reason"},
		{"sessions", "reconcile", "mark-stopped", "--id", "session-1", "--actor", "actor-1"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(append([]string{"--data-dir", filepath.Join(t.TempDir(), "data")}, args...), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "--id, --actor, and --reason are required") {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestCLIManualStopRuntimeReconciliation(t *testing.T) {
	runtime := local.NewProcessAgent()
	server := httptest.NewServer(httptransport.NewServer(runtime, "", nil).Handler())
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
	command := append(append([]string{}, base...), "sessions", "reconcile", "stop-runtime", "--id", "session-1", "--actor", "actor-1", "--reason", "stop orphan runtime", "--request-id", "request-stop-runtime")
	if code := Run(command, &stdout, &stderr); code != 0 {
		t.Fatalf("stop-runtime: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=succeeded") || !strings.Contains(stdout.String(), "agentResult=success") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	status, err := runtime.InspectSession(context.Background(), "session-1")
	if err != nil || status.Running || status.Status != "stopped" {
		t.Fatalf("runtime status=%+v err=%v", status, err)
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "running" {
		t.Fatalf("controller state changed: %+v err=%v", value, err)
	}
	operations, _ := store.ListOperationsBySession(context.Background(), "session-1")
	var found bool
	for _, operationValue := range operations {
		if operationValue.Action == "session.reconcile.stop-runtime" {
			found = operationValue.Status == "succeeded" && operationValue.RequestID == "request-stop-runtime" && operationValue.Metadata["agentRuntimeStatus"] == "running" && operationValue.Metadata["agentResult"] == "success" && operationValue.Metadata["agentMode"] == "http"
		}
	}
	if !found {
		t.Fatalf("runtime stop operation missing: %+v", operations)
	}
}

func TestCLIManualMarkCrashedReconciliation(t *testing.T) {
	runtime := local.NewProcessAgent()
	server := httptest.NewServer(httptransport.NewServer(runtime, "", nil).Handler())
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
	command := append(append([]string{}, base...), "sessions", "reconcile", "mark-crashed", "--id", "session-1", "--actor", "actor-1", "--reason", "operator confirmed crash", "--request-id", "request-mark-crashed")
	if code := Run(command, &stdout, &stderr); code != 0 {
		t.Fatalf("mark-crashed: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mark-crashed status=succeeded") || !strings.Contains(stdout.String(), "observationAvailable=true") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "crashed" || value.LastRuntimeMessage != "manually reconciled as crashed" {
		t.Fatalf("session=%+v err=%v", value, err)
	}
	status, err := runtime.InspectSession(context.Background(), "session-1")
	if err != nil || !status.Running {
		t.Fatalf("mark-crashed must not stop runtime: status=%+v err=%v", status, err)
	}
	observations, err := store.ListRuntimeObservationsBySession(context.Background(), "session-1")
	if err != nil || len(observations) != 1 {
		t.Fatalf("observations=%+v err=%v", observations, err)
	}
	operations, _ := store.ListOperationsBySession(context.Background(), "session-1")
	var found bool
	for _, operationValue := range operations {
		if operationValue.Action == "session.reconcile.mark-crashed" {
			found = operationValue.Status == "succeeded" && operationValue.RequestID == "request-mark-crashed" && operationValue.Metadata["observationId"] == observations[0].ID && operationValue.Metadata["agentRuntimeStatus"] == "running" && operationValue.Metadata["controllerSessionState"] == "running"
		}
	}
	if !found {
		t.Fatalf("mark-crashed operation missing: %+v", operations)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, event := range events {
		if event.Action == "session.reconcile.mark-crashed" && event.Metadata["requestId"] == "request-mark-crashed" && event.Metadata["reason"] == "operator confirmed crash" && event.Metadata["operationId"] != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mark-crashed audit missing: %+v", events)
	}
}

func TestCLIManualMarkCrashedUnreachableAgentStillSucceeds(t *testing.T) {
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
	code := Run([]string{"--data-dir", dataDirectory, "--agent-url", "http://127.0.0.1:1", "--agent-timeout", "100ms", "sessions", "reconcile", "mark-crashed", "--id", "session-1", "--actor", "actor-1", "--reason", "agent unreachable but operator confirmed crash"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "status=succeeded") || !strings.Contains(stdout.String(), "observationAvailable=false") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "crashed" {
		t.Fatalf("session=%+v err=%v", value, err)
	}
	operations, _ := store.ListOperationsBySession(context.Background(), "session-1")
	var found bool
	for _, operationValue := range operations {
		if operationValue.Action == "session.reconcile.mark-crashed" {
			found = operationValue.Status == "succeeded" && operationValue.Metadata["observationAvailable"] == "false" && operationValue.Metadata["observationError"] != ""
		}
	}
	if !found {
		t.Fatalf("mark-crashed operation missing: %+v", operations)
	}
}

func TestCLIManualStopRuntimeRequiresAgentURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--data-dir", filepath.Join(t.TempDir(), "data"), "sessions", "reconcile", "stop-runtime", "--id", "session-1", "--actor", "actor-1", "--reason", "manual stop"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "requires --agent-url") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLIManualStopRuntimeUnreachableAgentFailsWithoutStateChange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"--data-dir", dataDirectory, "sessions", "prepare", "--id", "session-1", "--actor", "actor-1"},
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
	code := Run([]string{"--data-dir", dataDirectory, "--agent-url", "http://127.0.0.1:1", "--agent-timeout", "100ms", "sessions", "reconcile", "stop-runtime", "--id", "session-1", "--actor", "actor-1", "--reason", "agent unreachable test"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "inspect agent runtime") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSession(context.Background(), "session-1")
	if err != nil || value.State != "running" {
		t.Fatalf("session=%+v err=%v", value, err)
	}
	operations, _ := store.ListOperationsBySession(context.Background(), "session-1")
	var found bool
	for _, operationValue := range operations {
		if operationValue.Action == "session.reconcile.stop-runtime" {
			found = operationValue.Status == "failed" && operationValue.Metadata["agentResult"] == "failure"
		}
	}
	if !found {
		t.Fatalf("failed operation missing: %+v", operations)
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

func TestCLIArtifactStagingPlanListAndInspect(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
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
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	if err := store.CreateArtifact(context.Background(), artifact.Artifact{ID: "artifact-1", Name: "Test Mod", Type: artifact.TypeJar, UploaderID: "uploader-1", SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, Status: artifact.StatusApproved, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "staging", "plan", "--session", "session-1", "--artifact", "artifact-1", "--actor", "actor-1", "--name", "mods/test.jar"}, &stdout, &stderr); code != 0 {
		t.Fatalf("plan: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=planned") || !strings.Contains(stdout.String(), "No payload was copied") {
		t.Fatalf("plan stdout=%q", stdout.String())
	}
	plans, err := store.ListArtifactStagingPlansBySession(context.Background(), "session-1")
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans=%+v err=%v", plans, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "staging", "list", "--session", "session-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("list: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), plans[0].ID+"\tsession-1\tartifact-1\tartifact\t"+filepath.Clean("mods/test.jar")+"\tplanned") {
		t.Fatalf("list stdout=%q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "staging", "inspect", "--id", plans[0].ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "artifact=artifact-1") || !strings.Contains(stdout.String(), "target="+filepath.Clean("mods/test.jar")) {
		t.Fatalf("inspect stdout=%q", stdout.String())
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
