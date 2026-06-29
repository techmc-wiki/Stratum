package service

import (
	"context"
	"errors"
	"testing"

	permissionsvc "github.com/stratummc/stratum/internal/permission/service"
	"github.com/stratummc/stratum/internal/resourcepolicy"
	"github.com/stratummc/stratum/internal/session"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

type recordingPermissionChecker struct {
	err   error
	calls []permissionCall
}

type permissionCall struct {
	actor     string
	sessionID string
	action    string
}

func (c *recordingPermissionChecker) CheckSessionAccess(_ context.Context, actor, sessionID, action string) error {
	c.calls = append(c.calls, permissionCall{actor: actor, sessionID: sessionID, action: action})
	return c.err
}

func TestSessionPermissionCheckerNilPreservesExistingBehavior(t *testing.T) {
	ctx, store, svc, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))

	if err := svc.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("Start with nil permission checker: %v", err)
	}
}

func TestSessionPermissionCheckerDeniesStartBeforeOperationCreation(t *testing.T) {
	ctx, store, svc, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	checker := &recordingPermissionChecker{err: errors.New("requires maintainer")}
	svc.WithPermissionChecker(checker)

	err := svc.Start(ctx, "session-1", "actor-1")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindForbidden) {
		t.Fatalf("error kind = %v, want forbidden", err)
	}
	assertPermissionCall(t, checker, permissionCall{actor: "actor-1", sessionID: "session-1", action: permissionsvc.ActionSessionStart})

	operations, err := store.ListOperations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(operations))
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != session.StateCreated {
		t.Fatalf("state = %s, want created", got.State)
	}
}

func TestSessionPermissionCheckerAllowsOwnedForkRestart(t *testing.T) {
	ctx, store, svc, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	value := testSession("fork-1", session.TypeFork, session.StateStopped)
	value.OwnerUserID = "actor-1"
	createTestSession(t, store, value)
	checker := &recordingPermissionChecker{}
	svc.WithPermissionChecker(checker)

	if err := svc.Restart(ctx, "fork-1", "actor-1"); err != nil {
		t.Fatalf("Restart allowed fork: %v", err)
	}
	assertPermissionCall(t, checker, permissionCall{actor: "actor-1", sessionID: "fork-1", action: permissionsvc.ActionSessionRestart})
}

func TestSessionPermissionCheckerDeniesSendCommandBeforeOperationCreation(t *testing.T) {
	ctx, store, svc, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateRunning))
	checker := &recordingPermissionChecker{err: errors.New("requires maintainer")}
	svc.WithPermissionChecker(checker)

	_, err := svc.SendCommand(ctx, "session-1", "actor-1", "say hello")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindForbidden) {
		t.Fatalf("error kind = %v, want forbidden", err)
	}
	assertPermissionCall(t, checker, permissionCall{actor: "actor-1", sessionID: "session-1", action: permissionsvc.ActionSessionCommand})

	operations, err := store.ListOperations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 0 {
		t.Fatalf("operations = %d, want 0", len(operations))
	}
}

func TestSessionPermissionActionMapping(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{name: "prepare", action: "prepare", want: permissionsvc.ActionSessionStart},
		{name: "start", action: "start", want: permissionsvc.ActionSessionStart},
		{name: "stop", action: "stop", want: permissionsvc.ActionSessionStop},
		{name: "restart", action: "restart", want: permissionsvc.ActionSessionRestart},
		{name: "freeze", action: "freeze", want: permissionsvc.ActionSessionStop},
		{name: "unfreeze", action: "unfreeze", want: permissionsvc.ActionSessionStart},
		{name: "archive", action: "archive", want: permissionsvc.ActionSessionStop},
		{name: "delete", action: "delete", want: permissionsvc.ActionSessionDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sessionPermissionAction(tt.action)
			if err != nil {
				t.Fatalf("sessionPermissionAction(%q): %v", tt.action, err)
			}
			if got != tt.want {
				t.Fatalf("action = %s, want %s", got, tt.want)
			}
		})
	}
}

func assertPermissionCall(t *testing.T, checker *recordingPermissionChecker, want permissionCall) {
	t.Helper()
	if len(checker.calls) != 1 {
		t.Fatalf("permission calls = %d, want 1: %+v", len(checker.calls), checker.calls)
	}
	if checker.calls[0] != want {
		t.Fatalf("permission call = %+v, want %+v", checker.calls[0], want)
	}
}
