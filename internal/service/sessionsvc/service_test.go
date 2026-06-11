package sessionsvc

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	if len(events) != 4 {
		t.Fatalf("audit events = %d, want 4", len(events))
	}
	for _, event := range events {
		if event.Metadata["result"] != "success" {
			t.Fatalf("event = %+v", event)
		}
	}
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
	if len(events) != 4 || events[2].Metadata["reason"] != "manual test" {
		t.Fatalf("events = %+v", events)
	}
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
