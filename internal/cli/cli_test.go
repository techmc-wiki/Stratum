package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/repository/artifactblob"
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

func TestCLIArtifactCreateCannotApproveOrStageWithoutPayload(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"},
		{"--data-dir", dataDirectory, "artifacts", "create", "--id", "artifact-1", "--name", "Test Artifact", "--type", "jar", "--project", "project-1", "--actor", "actor-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(command, &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "status=pending") || !strings.Contains(stdout.String(), "project=project-1") || !strings.Contains(stdout.String(), "payload=metadata-only") || !strings.Contains(stdout.String(), "no payload was uploaded") {
		t.Fatalf("create stdout=%q", stdout.String())
	}

	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil || created.Status != artifact.StatusPending || created.ProjectID != "project-1" || created.PayloadStatus != artifact.PayloadMetadataOnly || created.SHA256 != "" {
		t.Fatalf("artifact=%+v err=%v", created, err)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var createdAudit bool
	for _, event := range events {
		if event.Action == "artifact.created" && event.ProjectID == "project-1" && event.ActorID == "actor-1" && event.Metadata["artifactId"] == "artifact-1" && event.Metadata["artifactType"] == "jar" && event.Metadata["status"] == "pending" {
			createdAudit = true
		}
	}
	if !createdAudit {
		t.Fatalf("artifact creation audit missing: %+v", events)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "staging", "plan", "--session", "session-1", "--artifact", "artifact-1", "--actor", "actor-1", "--name", "test-artifact.jar"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "status=rejected") {
		t.Fatalf("pending staging: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "approve", "--id", "artifact-1", "--actor", "actor-1", "--reason", "should fail"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "payload metadata is missing") {
		t.Fatalf("approve: code=%d stderr=%q", code, stderr.String())
	}
	plans, err := store.ListArtifactStagingPlansBySession(context.Background(), "session-1")
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans=%+v err=%v", plans, err)
	}
	current, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil || current.Status != artifact.StatusPending || current.ReviewedBy != "" || current.ReviewedAt != nil || current.ReviewReason != "" {
		t.Fatalf("artifact=%+v err=%v", current, err)
	}
}

func TestCLIArtifactCreateValidationAndDuplicate(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "data")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"}, &stdout, &stderr); code != 0 {
		t.Fatalf("project: %s", stderr.String())
	}
	base := []string{"--data-dir", dataDirectory, "artifacts", "create"}
	tests := [][]string{
		{"--name", "Artifact", "--type", "jar", "--project", "project-1", "--actor", "actor-1"},
		{"--id", "artifact-1", "--type", "jar", "--project", "project-1", "--actor", "actor-1"},
		{"--id", "artifact-1", "--name", "Artifact", "--project", "project-1", "--actor", "actor-1"},
		{"--id", "artifact-1", "--name", "Artifact", "--type", "jar", "--project", "project-1"},
	}
	for _, args := range tests {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), args...), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "required") {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	invalid := append(append([]string{}, base...), "--id", "artifact-1", "--name", "Artifact", "--type", "binary", "--project", "project-1", "--actor", "actor-1")
	if code := Run(invalid, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "unsupported artifact type") {
		t.Fatalf("invalid type: code=%d stderr=%q", code, stderr.String())
	}
	valid := append(append([]string{}, base...), "--id", "artifact-1", "--name", "Artifact", "--type", "jar", "--project", "project-1", "--actor", "actor-1")
	stdout.Reset()
	stderr.Reset()
	if code := Run(valid, &stdout, &stderr); code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(valid, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("duplicate: code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLIArtifactInspectIsReadOnlyAndShowsReviewMetadata(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "artifacts")
	payloadPath := filepath.Join(root, "payload.jar")
	if err := os.WriteFile(payloadPath, []byte("artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot}
	var stdout, stderr bytes.Buffer
	commands := [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"artifacts", "create", "--id", "artifact-1", "--name", "Test Artifact", "--type", "jar", "--project", "project-1", "--actor", "creator-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	operationsBefore, err := store.ListOperations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "inspect", "--id", "artifact-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("inspect: code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"id=artifact-1", `name="Test Artifact"`, "type=jar", "project=project-1", "status=pending", "uploadedBy=creator-1", "createdAt=", "payload=metadata-only"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("inspect stdout=%q missing %q", stdout.String(), want)
		}
	}
	for _, absent := range []string{"reviewedBy=", "reviewedAt=", "reviewReason=", "hash=", "size=", "targetVersions=", "loaders="} {
		if strings.Contains(stdout.String(), absent) {
			t.Fatalf("inspect stdout=%q unexpectedly contains %q", stdout.String(), absent)
		}
	}
	after, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	operationsAfter, err := store.ListOperations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) || len(eventsAfter) != len(eventsBefore) || len(operationsAfter) != len(operationsBefore) {
		t.Fatalf("inspect mutated state: before=%+v after=%+v events=%d->%d operations=%d->%d", before, after, len(eventsBefore), len(eventsAfter), len(operationsBefore), len(operationsAfter))
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "import-file", "--id", "artifact-1", "--file", payloadPath, "--actor", "creator-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("import: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "approve", "--id", "artifact-1", "--actor", "reviewer-1", "--reason", "trusted fixture"), &stdout, &stderr); code != 0 {
		t.Fatalf("approve: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "inspect", "--id", "artifact-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("inspect approved: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=approved") || !strings.Contains(stdout.String(), "reviewedBy=reviewer-1") || !strings.Contains(stdout.String(), `reviewReason="trusted fixture"`) || !strings.Contains(stdout.String(), "reviewedAt=202") {
		t.Fatalf("approved inspect stdout=%q", stdout.String())
	}
}

func TestCLIArtifactInspectMissingAndRejected(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "data")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "inspect", "--id", "missing"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), `artifact "missing" not found`) {
		t.Fatalf("missing: code=%d stderr=%q", code, stderr.String())
	}
	commands := [][]string{
		{"--data-dir", dataDirectory, "projects", "create", "--id", "project-1", "--name", "Project"},
		{"--data-dir", dataDirectory, "artifacts", "create", "--id", "artifact-1", "--name", "Rejected Artifact", "--type", "jar", "--project", "project-1", "--actor", "creator-1"},
		{"--data-dir", dataDirectory, "artifacts", "reject", "--id", "artifact-1", "--actor", "reviewer-1", "--reason", "unsafe fixture"},
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
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "inspect", "--id", "artifact-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect rejected: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=rejected") || !strings.Contains(stdout.String(), "reviewedBy=reviewer-1") || !strings.Contains(stdout.String(), `reviewReason="unsafe fixture"`) {
		t.Fatalf("rejected inspect stdout=%q", stdout.String())
	}
}

func TestCLIArtifactInspectShowsAvailablePayloadMetadata(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "data")
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value := artifact.Artifact{
		ID: "artifact-1", Name: "Stored Artifact", Type: artifact.TypeJar, UploaderID: "uploader-1",
		SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8, PayloadStatus: artifact.PayloadAvailable,
		TargetMinecraftVersions: []string{"1.17", "1.20.6"}, LoaderCompatibility: []string{"fabric"},
		Status: artifact.StatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateArtifact(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "inspect", "--id", value.ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect: code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"payload=available", "hash=" + value.SHA256, "size=8", "targetVersions=1.17,1.20.6", "loaders=fabric"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("inspect stdout=%q missing %q", stdout.String(), want)
		}
	}
}

func TestCLIArtifactImportFileStoresAndInspectsPayload(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "artifacts")
	path := filepath.Join(root, "payload.jar")
	if err := os.WriteFile(path, []byte("hello artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot}
	var stdout, stderr bytes.Buffer
	commands := [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"artifacts", "create", "--id", "artifact-1", "--name", "Artifact", "--type", "jar", "--project", "project-1", "--actor", "creator-1"},
		{"artifacts", "import-file", "--id", "artifact-1", "--file", path, "--actor", "importer-1"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command=%v code=%d stderr=%q", command, code, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "payloadAlgorithm=sha256") || !strings.Contains(stdout.String(), "payloadSize=14") || !strings.Contains(stdout.String(), "payloadStatus=available") || !strings.Contains(stdout.String(), "remains pending") {
		t.Fatalf("import stdout=%q", stdout.String())
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := artifactblob.New(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Verify(context.Background(), value.SHA256); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "inspect", "--id", "artifact-1"), &stdout, &stderr); code != 0 {
		t.Fatalf("inspect code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"status=pending", "payload=available", "payloadAlgorithm=sha256", "hash=" + value.SHA256, "size=14", "payloadReference=", "payloadImportedBy=importer-1", "payloadImportedAt="} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("inspect stdout=%q missing %q", stdout.String(), want)
		}
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var imported int
	for _, event := range events {
		if event.Action == "artifact.payload.imported" {
			imported++
		}
	}
	if imported != 1 {
		t.Fatalf("import audit count=%d events=%+v", imported, events)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "import-file", "--id", "artifact-1", "--file", path, "--actor", "importer-2"), &stdout, &stderr); code != 0 {
		t.Fatalf("duplicate import code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "import-file", "--id", "artifact-1", "--file", path), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--actor") {
		t.Fatalf("missing actor code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "import-file", "--id", "missing", "--file", path, "--actor", "importer-1"), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), `artifact "missing" not found`) {
		t.Fatalf("missing artifact code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLIArtifactBlobVerifyIsReadOnly(t *testing.T) {
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "artifacts")
	payloadPath := filepath.Join(root, "payload.jar")
	if err := os.WriteFile(payloadPath, []byte("verified payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot}
	var stdout, stderr bytes.Buffer
	for _, command := range [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"artifacts", "create", "--id", "artifact-1", "--name", "Artifact", "--type", "jar", "--project", "project-1", "--actor", "creator-1"},
		{"artifacts", "import-file", "--id", "artifact-1", "--file", payloadPath, "--actor", "importer-1"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command=%v code=%d stderr=%q", command, code, stderr.String())
		}
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	operationsBefore, err := store.ListOperations(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "blobs", "verify", "--sha256", before.SHA256), &stdout, &stderr); code != 0 {
		t.Fatalf("verify code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"algorithm=sha256", "hash=" + before.SHA256, "size=16", "status=valid", "reference=sha256/"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("verify stdout=%q missing %q", stdout.String(), want)
		}
	}

	stdout.Reset()
	stderr.Reset()
	missingHash := strings.Repeat("a", 64)
	if code := Run(append(append([]string{}, base...), "artifacts", "blobs", "verify", "--sha256", missingHash), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "status=missing") || !strings.Contains(stderr.String(), "blob does not exist") {
		t.Fatalf("missing code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "blobs", "verify", "--sha256", "invalid"), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "validation") {
		t.Fatalf("invalid code=%d stderr=%q", code, stderr.String())
	}

	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	blobPath, err := blobs.Path(before.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobPath, []byte("corrupted"), 0o640); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "blobs", "verify", "--sha256", before.SHA256), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "status=corrupted") || !strings.Contains(stderr.String(), "hash mismatch") {
		t.Fatalf("corrupted code=%d stderr=%q", code, stderr.String())
	}

	after, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	operationsAfter, err := store.ListOperations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) || len(eventsAfter) != len(eventsBefore) || len(operationsAfter) != len(operationsBefore) {
		t.Fatalf("verify mutated control-plane state: artifact=%t events=%d->%d operations=%d->%d", !reflect.DeepEqual(after, before), len(eventsBefore), len(eventsAfter), len(operationsBefore), len(operationsAfter))
	}
}

func TestCLIArtifactStagingPlanListAndInspect(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "artifacts")
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
	blobs, err := artifactblob.New(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := blobs.Put(context.Background(), strings.NewReader("artifact"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifact(context.Background(), artifact.Artifact{ID: "artifact-1", Name: "Test Mod", Type: artifact.TypeJar, UploaderID: "uploader-1", SHA256: payload.Hash, SizeBytes: payload.Size, PayloadStatus: artifact.PayloadAvailable, PayloadAlgorithm: payload.Algorithm, PayloadReference: payload.Reference, Status: artifact.StatusApproved, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot, "artifacts", "staging", "plan", "--session", "session-1", "--artifact", "artifact-1", "--actor", "actor-1", "--name", "mods/test.jar"}, &stdout, &stderr); code != 0 {
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

func TestCLIArtifactStagingMaterialize(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "blobs")
	runtimeRoot := filepath.Join(root, "runtime")
	payloadPath := filepath.Join(root, "artifact.jar")
	if err := os.WriteFile(payloadPath, []byte("artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewServer(runtime, "", nil).Handler())
	defer server.Close()
	base := []string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot}
	commands := [][]string{
		{"projects", "create", "--id", "project-1", "--name", "Project"},
		{"rooms", "create", "--id", "room-1", "--project", "project-1", "--name", "Room"},
		{"sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1", "--type", "shared"},
		{"artifacts", "create", "--id", "artifact-1", "--name", "Artifact", "--type", "jar", "--project", "project-1", "--actor", "creator-1"},
		{"artifacts", "staging", "plan", "--session", "session-1", "--artifact", "artifact-1", "--actor", "actor-1", "--name", "mods/test.jar"},
	}
	for _, command := range commands {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	plans, _ := store.ListArtifactStagingPlans(context.Background())
	if len(plans) != 1 || plans[0].Status != artifactstaging.StatusRejected {
		t.Fatalf("plans=%+v", plans)
	}
	materializeBase := append(append([]string{}, base...), "--agent-url", server.URL, "artifacts", "staging", "materialize")
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, materializeBase...), "--plan", "missing-plan", "--actor", "actor-1"), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "not_found") {
		t.Fatalf("missing plan: code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, materializeBase...), "--plan", plans[0].ID, "--actor", "actor-1"), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "not planned") {
		t.Fatalf("rejected plan: code=%d stderr=%q", code, stderr.String())
	}
	for _, command := range [][]string{
		{"artifacts", "import-file", "--id", "artifact-1", "--file", payloadPath, "--actor", "creator-1"},
		{"artifacts", "approve", "--id", "artifact-1", "--actor", "reviewer-1", "--reason", "verified"},
		{"artifacts", "staging", "plan", "--session", "session-1", "--artifact", "artifact-1", "--actor", "actor-1", "--name", "mods/test.jar"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(append(append([]string{}, base...), command...), &stdout, &stderr); code != 0 {
			t.Fatalf("command %v: code=%d stderr=%q", command, code, stderr.String())
		}
	}
	plans, _ = store.ListArtifactStagingPlans(context.Background())
	var planned artifactstaging.Plan
	for _, value := range plans {
		if value.Status == artifactstaging.StatusPlanned {
			planned = value
		}
	}
	if planned.ID == "" {
		t.Fatalf("planned staging record missing: %+v", plans)
	}
	command := append(append([]string{}, materializeBase...), "--plan", planned.ID, "--actor", "actor-1")
	artifactValue, err := store.GetArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	blobPath, err := blobs.Path(artifactValue.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	originalPayload, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blobPath); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(command, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "blob does not exist") {
		t.Fatalf("missing blob: code=%d stderr=%q", code, stderr.String())
	}
	if err := os.WriteFile(blobPath, []byte("corrupted"), 0o640); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(command, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "hash mismatch") {
		t.Fatalf("corrupted blob: code=%d stderr=%q", code, stderr.String())
	}
	if err := os.WriteFile(blobPath, originalPayload, 0o640); err != nil {
		t.Fatal(err)
	}
	artifactValue.Status = artifact.StatusPending
	if err := store.SaveArtifact(context.Background(), artifactValue); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(command, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "not approved") {
		t.Fatalf("pending artifact: code=%d stderr=%q", code, stderr.String())
	}
	artifactValue.Status = artifact.StatusApproved
	if err := store.SaveArtifact(context.Background(), artifactValue); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(command, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "status=materialized") || !strings.Contains(stdout.String(), "not installed") {
		t.Fatalf("materialize: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	target := filepath.Join(runtimeRoot, "sessions", "session-1", "artifacts", "mods", "test.jar")
	payload, err := os.ReadFile(target)
	if err != nil || string(payload) != "artifact" {
		t.Fatalf("payload=%q err=%v", payload, err)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, "sessions", "session-1", "artifacts", "staged-artifacts.json")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(command, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "idempotent=true") {
		t.Fatalf("idempotent: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	events, _ := store.ListAuditEvents(context.Background())
	materialized := 0
	for _, event := range events {
		if event.Action == "artifact.materialized" && event.Metadata["stagingPlanId"] == planned.ID && event.Metadata["runtimeRelativePath"] == "artifacts/mods/test.jar" {
			materialized++
		}
	}
	if materialized != 2 {
		t.Fatalf("materialization audits=%d events=%+v", materialized, events)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "--agent-url", server.URL, "sessions", "artifacts", "--id", "session-1"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "artifact=artifact-1") || !strings.Contains(stdout.String(), "runtimePath=artifacts/mods/test.jar") || !strings.Contains(stdout.String(), "status=materialized") {
		t.Fatalf("inspect materialized: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLIArtifactStagingMaterializeRequiresActorAndAgentURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	base := []string{"--data-dir", filepath.Join(t.TempDir(), "data"), "artifacts", "staging", "materialize"}
	if code := Run(append(append([]string{}, base...), "--plan", "plan-1"), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--actor") {
		t.Fatalf("actor code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "--plan", "plan-1", "--actor", "actor-1"), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--agent-url") {
		t.Fatalf("agent URL code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLISessionArtifactsRequiresIDAndAgentURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	dataDirectory := filepath.Join(t.TempDir(), "data")
	if code := Run([]string{"--data-dir", dataDirectory, "sessions", "artifacts"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--id") {
		t.Fatalf("id code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "sessions", "artifacts", "--id", "session-1"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "--agent-url") {
		t.Fatalf("agent URL code=%d stderr=%q", code, stderr.String())
	}
}

func TestCLIArtifactApproveReject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	blobRoot := filepath.Join(root, "artifacts")
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	blobs, err := artifactblob.New(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := blobs.Put(context.Background(), strings.NewReader("approve"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifact(context.Background(), artifact.Artifact{
		ID: "approve-me", Name: "Approve Me", Type: artifact.TypeJar, UploaderID: "uploader-1",
		SHA256: payload.Hash, SizeBytes: payload.Size, PayloadStatus: artifact.PayloadAvailable, PayloadAlgorithm: payload.Algorithm,
		PayloadReference: payload.Reference, PayloadImportedBy: "uploader-1", PayloadImportedAt: &now,
		Status: artifact.StatusPending, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifact(context.Background(), artifact.Artifact{ID: "reject-me", Name: "Reject Me", Type: artifact.TypeJar, UploaderID: "uploader-1", SHA256: artifact.HashBytes([]byte("reject")), SizeBytes: 6, Status: artifact.StatusPending, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	base := []string{"--data-dir", dataDirectory, "--artifact-blob-root", blobRoot}
	if code := Run(append(append([]string{}, base...), "artifacts", "approve", "--id", "approve-me", "--actor", "reviewer-1", "--reason", "trusted fixture"), &stdout, &stderr); code != 0 {
		t.Fatalf("approve: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=approved") || !strings.Contains(stdout.String(), "No payload was copied") {
		t.Fatalf("approve stdout=%q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "artifacts", "reject", "--id", "reject-me", "--actor", "reviewer-1", "--reason", "unsafe fixture"), &stdout, &stderr); code != 0 {
		t.Fatalf("reject: code=%d stderr=%q", code, stderr.String())
	}
	approved, err := store.GetArtifact(context.Background(), "approve-me")
	if err != nil || approved.Status != artifact.StatusApproved || approved.ReviewedBy != "reviewer-1" || approved.ReviewedAt == nil || approved.ReviewReason != "trusted fixture" {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	rejected, err := store.GetArtifact(context.Background(), "reject-me")
	if err != nil || rejected.Status != artifact.StatusRejected || rejected.ReviewedBy != "reviewer-1" || rejected.ReviewedAt == nil || rejected.ReviewReason != "unsafe fixture" {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	events, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var approvedEvent, rejectedEvent bool
	for _, event := range events {
		if event.Action == "artifact.approved" && event.Metadata["artifactId"] == "approve-me" && event.Metadata["reason"] == "trusted fixture" {
			approvedEvent = true
		}
		if event.Action == "artifact.rejected" && event.Metadata["artifactId"] == "reject-me" && event.Metadata["reason"] == "unsafe fixture" {
			rejectedEvent = true
		}
	}
	if !approvedEvent || !rejectedEvent {
		t.Fatalf("events=%+v", events)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "artifacts", "list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("list: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "approve-me\tApprove Me\tjar\tapproved\treviewedBy=reviewer-1") || !strings.Contains(stdout.String(), "reviewReason=trusted fixture") {
		t.Fatalf("list stdout=%q", stdout.String())
	}
}

func TestCLIArtifactReviewRequiresFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--data-dir", filepath.Join(t.TempDir(), "data"), "artifacts", "approve", "--id", "artifact-1", "--actor", "reviewer-1"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "--id, --actor, and --reason are required") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
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
