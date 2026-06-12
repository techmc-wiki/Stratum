package sessionsvc

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/filesystem"
)

var lifecycleTime = time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

func TestValidLifecycleAndPersistence(t *testing.T) {
	ctx, store, service, root := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))

	if err := service.Prepare(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Stop(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Archive(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateArchived {
		t.Fatalf("state = %s, want archived", got.State)
	}
	events, err := reloaded.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 4 {
		t.Fatalf("audit events = %d, want 4", len(events))
	}
	for _, event := range events {
		if event.Metadata["result"] != "success" {
			t.Fatalf("event = %+v", event)
		}
	}
}

func TestAgentStartSuccessPersistsMetadataAndAudit(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateRunning || got.AssignedAgentID != local.DefaultAgentID || got.LastAgentStatus != "success" || got.LastRuntimeMessage != "running" || got.RuntimeEndpoint == "" {
		t.Fatalf("session = %+v", got)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["agentResult"] != "success" || events[0].Metadata["agentId"] != local.DefaultAgentID || events[0].Metadata["agentMode"] != "local" {
		t.Fatalf("events = %+v", events)
	}
}

func TestLifecycleUsesHTTPAgentClient(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	server := httptest.NewServer(httptransport.NewServer(local.NewFake(), "", nil).Handler())
	defer server.Close()
	client, err := httptransport.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, resourcepolicy.MVPDefault(), client)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-http", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateRunning || got.RuntimeEndpoint != server.URL {
		t.Fatalf("session = %+v", got)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["agentMode"] != "http" {
		t.Fatalf("events = %+v", events)
	}
}

func TestHTTPAgentFailureDoesNotPersistState(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	fake.SetFailure(agent.OperationStart, "HTTP planned failure")
	server := httptest.NewServer(httptransport.NewServer(fake, "", nil).Handler())
	defer server.Close()
	client, err := httptransport.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, resourcepolicy.MVPDefault(), client)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-http", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("expected HTTP agent failure")
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateCreated {
		t.Fatalf("state = %s", got.State)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["agentResult"] != "failure" || events[0].Metadata["agentMode"] != "http" {
		t.Fatalf("events = %+v", events)
	}
}

func TestAgentStartFailureLeavesStateUnchanged(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	fake.SetFailure(agent.OperationStart, "start blocked")
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("expected agent failure")
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateCreated || got.AssignedAgentID != "" {
		t.Fatalf("session mutated: %+v", got)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["agentResult"] != "failure" || events[0].Metadata["agentId"] != local.DefaultAgentID || events[0].Metadata["agentMessage"] == "" {
		t.Fatalf("events = %+v", events)
	}
}

func TestAgentStopFailureLeavesStateAndRecordsFailedOperation(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-stop-failure", session.TypeShared, session.StateRunning))
	fake := local.NewFake()
	fake.SetFailure(agent.OperationStop, "stop blocked")
	service := New(store, resourcepolicy.MVPDefault(), fake)
	value, _, err := service.StopWithOptions(ctx, "session-stop-failure", "actor-1", OperationOptions{RequestID: "request-stop-failure"})
	if err == nil || value.Status != operation.StatusFailed {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	got, loadErr := store.GetSession(ctx, "session-stop-failure")
	if loadErr != nil || got.State != session.StateRunning {
		t.Fatalf("session=%+v err=%v", got, loadErr)
	}
	operations, listErr := store.ListOperations(ctx)
	if listErr != nil || len(operations) != 1 || operations[0].Status != operation.StatusFailed {
		t.Fatalf("operations=%+v err=%v", operations, listErr)
	}
	events, auditErr := store.ListAuditEvents(ctx)
	if auditErr != nil {
		t.Fatal(auditErr)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["operationId"] != value.ID || events[0].Metadata["requestId"] != value.RequestID {
		t.Fatalf("events=%+v", events)
	}
}

func TestStopAndRestartCallAgent(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateRunning))
	if err := service.Restart(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Stop(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls()
	if !containsOperation(calls, agent.OperationRestart) || !containsOperation(calls, agent.OperationStop) {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestFreezeAndUnfreezeCallDistinctAgentOperations(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateRunning))
	if err := service.Freeze(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Unfreeze(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls()
	if !containsOperation(calls, agent.OperationFreeze) || !containsOperation(calls, agent.OperationUnfreeze) {
		t.Fatalf("calls = %+v", calls)
	}
}

func containsOperation(values []agent.Operation, target agent.Operation) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestInvalidTransitionWritesFailureAudit(t *testing.T) {
	ctx, store, service, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateStopped))

	if err := service.Freeze(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("expected freeze to fail")
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateStopped {
		t.Fatalf("state mutated to %s", got.State)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["result"] != "failure" || events[0].Metadata["reason"] == "" {
		t.Fatalf("events = %+v", events)
	}
}

func TestStartDeniedByResourcePolicyDoesNotMutate(t *testing.T) {
	policy := resourcepolicy.MVPDefault()
	policy.GlobalMaxRunning = 1
	ctx, store, service, _ := newLifecycleTest(t, policy)
	createTestSession(t, store, testSession("running", session.TypeShared, session.StateRunning))
	target := testSession("target", session.TypeShared, session.StateCreated)
	target.ProjectID = "project-2"
	createTestSession(t, store, target)

	err := service.Start(ctx, "target", "actor-1")
	var denied DeniedError
	if !errors.As(err, &denied) || denied.Reason != resourcepolicy.DeniedGlobalLimit {
		t.Fatalf("error = %v", err)
	}
	got, getErr := store.GetSession(ctx, "target")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.State != session.StateCreated {
		t.Fatalf("state = %s, want created", got.State)
	}
	events, auditErr := store.ListAuditEvents(ctx)
	if auditErr != nil {
		t.Fatal(auditErr)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Metadata["result"] != "failure" {
		t.Fatalf("events = %+v", events)
	}
}

func TestStartSuccessUsesPrivateOwnerLimitOnlyForPrivateAndFork(t *testing.T) {
	policy := resourcepolicy.MVPDefault()
	policy.PerUserMax = 1
	ctx, store, service, _ := newLifecycleTest(t, policy)
	running := testSession("private-running", session.TypePrivate, session.StateRunning)
	createTestSession(t, store, running)
	privateTarget := testSession("private-target", session.TypePrivate, session.StateCreated)
	createTestSession(t, store, privateTarget)
	sharedTarget := testSession("shared-target", session.TypeShared, session.StateCreated)
	createTestSession(t, store, sharedTarget)

	if err := service.Start(ctx, "private-target", "actor-1"); err == nil {
		t.Fatal("expected private owner limit denial")
	}
	if err := service.Start(ctx, "shared-target", "actor-1"); err != nil {
		t.Fatalf("shared start: %v", err)
	}
	got, err := store.GetSession(ctx, "shared-target")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateRunning {
		t.Fatalf("state = %s", got.State)
	}
}

func TestRestartBehavior(t *testing.T) {
	ctx, store, service, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateRunning))
	if err := service.Restart(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateRunning {
		t.Fatalf("state = %s", got.State)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 1 || events[0].Action != "session.restart" || events[0].Metadata["previousState"] != "running" || events[0].Metadata["nextState"] != "running" {
		t.Fatalf("events = %+v", events)
	}
}

func TestFreezeUnfreezeAndCrashHandling(t *testing.T) {
	ctx, store, service, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateRunning))
	if err := service.Freeze(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Unfreeze(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.MarkCrashed(ctx, "session-1", "actor-1", "manual test"); err != nil {
		t.Fatal(err)
	}
	if err := service.Stop(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateStopped {
		t.Fatalf("state = %s", got.State)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events = onlySessionAudit(events)
	if len(events) != 4 || events[2].Metadata["reason"] != "manual test" {
		t.Fatalf("events = %+v", events)
	}
}

func onlySessionAudit(values []audit.Event) []audit.Event {
	result := make([]audit.Event, 0, len(values))
	for _, value := range values {
		if value.TargetType == "session" {
			result = append(result, value)
		}
	}
	return result
}

func TestDeleteRequiresArchivedAndRemovesMetadata(t *testing.T) {
	ctx, store, service, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateStopped))
	if err := service.Delete(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("stopped session should require archive before delete")
	}
	if err := service.Archive(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, "session-1", "actor-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession(ctx, "session-1"); !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		t.Fatalf("get error = %v", err)
	}
}

type timeoutAgent struct{ agent.AgentClient }

func (a timeoutAgent) StartSession(ctx context.Context, _ agent.SessionRequest) (agent.OperationResult, error) {
	<-ctx.Done()
	return agent.OperationResult{}, ctx.Err()
}

func TestOperationTimeoutDoesNotMutateSession(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-timeout", session.TypeShared, session.StateCreated))
	service := New(store, resourcepolicy.MVPDefault(), timeoutAgent{AgentClient: local.NewFake()})
	value, _, err := service.StartWithOptions(ctx, "session-timeout", "actor-1", OperationOptions{Timeout: 10 * time.Millisecond})
	if err == nil || value.Status != operation.StatusTimedOut {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	sessionValue, err := store.GetSession(ctx, "session-timeout")
	if err != nil || sessionValue.State != session.StateCreated {
		t.Fatalf("session=%+v err=%v", sessionValue, err)
	}
}

type countingAgent struct {
	agent.AgentClient
	starts int
}

func (a *countingAgent) StartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	a.starts++
	return a.AgentClient.StartSession(ctx, request)
}

func TestOperationIdempotencyDoesNotRepeatAgentCall(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-idempotent", session.TypeShared, session.StateCreated))
	agentClient := &countingAgent{AgentClient: local.NewFake()}
	service := New(store, resourcepolicy.MVPDefault(), agentClient)
	options := OperationOptions{IdempotencyKey: "start-once", RequestID: "request-fixed"}
	first, replay, err := service.StartWithOptions(ctx, "session-idempotent", "actor-1", options)
	if err != nil || replay {
		t.Fatalf("first=%+v replay=%t err=%v", first, replay, err)
	}
	second, replay, err := service.StartWithOptions(ctx, "session-idempotent", "actor-1", options)
	if err != nil || !replay || first.ID != second.ID || agentClient.starts != 1 {
		t.Fatalf("second=%+v replay=%t starts=%d err=%v", second, replay, agentClient.starts, err)
	}
}

func TestRuntimeProfileMetadataAndUnknownProfileFailure(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-profile", session.TypeShared, session.StateCreated))
	service := New(store, resourcepolicy.MVPDefault(), local.NewFake())
	failed, _, err := service.StartWithOptions(ctx, "session-profile", "actor-1", OperationOptions{RuntimeProfileID: "unknown-profile"})
	if err == nil || failed.Status != operation.StatusFailed || failed.Metadata["runtimeProfileId"] != "unknown-profile" {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	got, loadErr := store.GetSession(ctx, "session-profile")
	if loadErr != nil || got.State != session.StateCreated {
		t.Fatalf("session=%+v err=%v", got, loadErr)
	}
	createTestSession(t, store, testSession("session-default-profile", session.TypeShared, session.StateCreated))
	succeeded, _, err := service.StartWithOptions(ctx, "session-default-profile", "actor-1", OperationOptions{})
	if err != nil || succeeded.Metadata["runtimeProfileId"] != runtimeprofile.DefaultProfileID {
		t.Fatalf("succeeded=%+v err=%v", succeeded, err)
	}
	events, auditErr := store.ListAuditEvents(ctx)
	if auditErr != nil {
		t.Fatal(auditErr)
	}
	found := false
	for _, event := range events {
		if event.TargetType == "session" && event.TargetID == "session-default-profile" && event.Metadata["runtimeProfileId"] == runtimeprofile.DefaultProfileID {
			found = true
		}
	}
	if !found {
		t.Fatalf("runtime profile missing from audit events: %+v", events)
	}
}

func TestTerminalStartFailureDoesNotPersistRunningSession(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-terminal-failure", session.TypeShared, session.StateCreated))
	profile := runtimeprofile.Profile{ID: "broken-terminal", Name: "Broken terminal", RuntimeType: runtimeprofile.TypeTerminal, CommandArgv: []string{"stratum-definitely-missing-executable"}, WorkingDir: ".", StopStrategy: runtimeprofile.StopTerminate, GracefulStopTimeout: time.Millisecond, ForceKillTimeout: time.Millisecond, LogMode: runtimeprofile.LogMemory, Enabled: true}
	registry, err := runtimeprofile.NewRegistry(runtimeprofile.DummyProcess(), profile)
	if err != nil {
		t.Fatal(err)
	}
	agentClient, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, registry, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, resourcepolicy.MVPDefault(), agentClient)
	value, _, err := service.StartWithOptions(ctx, "session-terminal-failure", "actor-1", OperationOptions{RuntimeProfileID: profile.ID})
	if err == nil || value.Status != operation.StatusFailed {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	got, loadErr := store.GetSession(ctx, "session-terminal-failure")
	if loadErr != nil || got.State != session.StateCreated {
		t.Fatalf("session=%+v err=%v", got, loadErr)
	}
}

func TestDisabledRuntimeProfileDoesNotPersistRunningSession(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-disabled-profile", session.TypeShared, session.StateCreated))
	disabled := runtimeprofile.Profile{ID: "disabled-profile", Name: "Disabled profile", RuntimeType: runtimeprofile.TypeDummy, StopStrategy: runtimeprofile.StopNone, LogMode: runtimeprofile.LogMemory}
	registry, err := runtimeprofile.NewRegistry(runtimeprofile.DummyProcess(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, resourcepolicy.MVPDefault(), local.NewProcessAgentWithRegistry(local.DefaultAgentID, registry))
	value, _, err := service.StartWithOptions(ctx, "session-disabled-profile", "actor-1", OperationOptions{RuntimeProfileID: disabled.ID})
	if err == nil || value.Status != operation.StatusFailed {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	got, loadErr := store.GetSession(ctx, "session-disabled-profile")
	if loadErr != nil || got.State != session.StateCreated {
		t.Fatalf("session=%+v err=%v", got, loadErr)
	}
}

type blockingAgent struct {
	agent.AgentClient
	started chan struct{}
	release chan struct{}
}

func (a blockingAgent) StartSession(ctx context.Context, _ agent.SessionRequest) (agent.OperationResult, error) {
	close(a.started)
	select {
	case <-a.release:
		return agent.OperationResult{AgentID: "blocking", Status: "success", Mode: "test"}, nil
	case <-ctx.Done():
		return agent.OperationResult{}, ctx.Err()
	}
}

func TestConcurrentSessionOperationReturnsConflict(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-conflict", session.TypeShared, session.StateCreated))
	started, release := make(chan struct{}), make(chan struct{})
	service := New(store, resourcepolicy.MVPDefault(), blockingAgent{AgentClient: local.NewFake(), started: started, release: release})
	done := make(chan error, 1)
	go func() { done <- service.Start(ctx, "session-conflict", "actor-1") }()
	<-started
	if err := service.Stop(ctx, "session-conflict", "actor-2"); !stratumerrors.IsKind(err, stratumerrors.KindConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newLifecycleTest(t *testing.T, policy resourcepolicy.Policy) (context.Context, *filesystem.Store, *Service, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "data")
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, policy)
	service.now = func() time.Time { return lifecycleTime }
	sequence := 0
	service.newID = func(prefix string) (string, error) {
		sequence++
		return prefix + "-" + time.Duration(sequence).String(), nil
	}
	return context.Background(), store, service, root
}

func testSession(id string, kind session.Type, state session.State) session.Session {
	return session.Session{ID: id, ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: kind, State: state, EnvironmentID: "environment-1", CreatedAt: lifecycleTime, LastActiveAt: lifecycleTime}
}

func createTestSession(t *testing.T, store *filesystem.Store, value session.Session) {
	t.Helper()
	if err := store.CreateSession(context.Background(), value); err != nil {
		t.Fatal(err)
	}
}
