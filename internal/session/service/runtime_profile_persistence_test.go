package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	checkpointsvc "github.com/stratummc/stratum/internal/checkpoint/service"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/resourcepolicy"
	"github.com/stratummc/stratum/internal/session"
)

type acceptingProfileAgent struct {
	agent.AgentClient
	startErr       error
	startedProfile string
}

func (a *acceptingProfileAgent) StartSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	a.startedProfile = request.RuntimeProfileID
	if a.startErr != nil {
		return agent.OperationResult{}, a.startErr
	}
	return agent.OperationResult{AgentID: "profile-agent", Status: "success", Mode: "test"}, nil
}

func TestStartPersistsExplicitRuntimeProfile(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-explicit", session.TypeShared, session.StateCreated))
	service := New(store, resourcepolicy.MVPDefault(), local.NewFake())

	if _, _, err := service.StartWithOptions(ctx, "session-explicit", "actor-1", OperationOptions{RuntimeProfileID: "dummy-process"}); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSession(ctx, "session-explicit")
	if err != nil {
		t.Fatal(err)
	}
	if stored.RuntimeProfileID != "dummy-process" {
		t.Fatalf("runtime profile = %q", stored.RuntimeProfileID)
	}
}

func TestStartPersistsEnvironmentDefaultRuntimeProfile(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	env := environment.Environment{
		ID: "environment-profile", Name: "Profile Environment", MinecraftVersion: "1.17.1",
		LoaderType: environment.LoaderFabric, ServerCore: environment.ServerCarpet,
		RuntimeProfileID: "environment-profile", CreatedAt: lifecycleTime, UpdatedAt: lifecycleTime,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	value := testSession("session-environment", session.TypeShared, session.StateCreated)
	value.EnvironmentID = env.ID
	createTestSession(t, store, value)
	agentClient := &acceptingProfileAgent{AgentClient: local.NewFake()}
	service := New(store, resourcepolicy.MVPDefault(), agentClient)

	op, _, err := service.StartWithOptions(ctx, value.ID, "actor-1", OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSession(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RuntimeProfileID != env.RuntimeProfileID || agentClient.startedProfile != env.RuntimeProfileID {
		t.Fatalf("stored=%q started=%q", stored.RuntimeProfileID, agentClient.startedProfile)
	}
	if op.Metadata["selectedRuntimeProfileId"] != env.RuntimeProfileID || op.Metadata["runtimeProfileId"] != env.RuntimeProfileID {
		t.Fatalf("operation metadata = %+v", op.Metadata)
	}
	events, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundSelectedProfile := false
	for _, event := range events {
		if event.Action == "operation.completed" && event.Metadata["selectedRuntimeProfileId"] == env.RuntimeProfileID {
			foundSelectedProfile = true
			break
		}
	}
	if !foundSelectedProfile {
		t.Fatalf("selected runtime profile missing from operation audit: %+v", events)
	}
}

func TestStartFailuresDoNotPersistSelectedRuntimeProfile(t *testing.T) {
	for _, test := range []struct {
		name        string
		agentClient agent.AgentClient
	}{
		{
			name: "before agent start",
			agentClient: &orderedReadinessAgent{
				AgentClient: local.NewFake(), readiness: notReadyRuntime(),
			},
		},
		{
			name: "agent start",
			agentClient: &acceptingProfileAgent{
				AgentClient: local.NewFake(), startErr: errors.New("start failed"),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
			value := testSession("session-failure", session.TypeShared, session.StateCreated)
			value.RuntimeProfileID = "existing-profile"
			createTestSession(t, store, value)
			service := New(store, resourcepolicy.MVPDefault(), test.agentClient)

			if _, _, err := service.StartWithOptions(ctx, value.ID, "actor-1", OperationOptions{RuntimeProfileID: "dummy-process"}); err == nil {
				t.Fatal("expected start failure")
			}
			stored, err := store.GetSession(ctx, value.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.RuntimeProfileID != "existing-profile" {
				t.Fatalf("runtime profile = %q", stored.RuntimeProfileID)
			}
		})
	}
}

func TestRestartPersistsProfileOnlyAfterFinalStart(t *testing.T) {
	for _, test := range []struct {
		name        string
		agentClient agent.AgentClient
		wantProfile string
		wantError   bool
	}{
		{name: "success", agentClient: local.NewFake(), wantProfile: "dummy-process"},
		{
			name: "readiness failure",
			agentClient: &orderedReadinessAgent{
				AgentClient: local.NewFake(), readiness: notReadyRuntime(),
			},
			wantProfile: "existing-profile", wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
			value := testSession("session-restart", session.TypeShared, session.StateRunning)
			value.RuntimeProfileID = "existing-profile"
			createTestSession(t, store, value)
			service := New(store, resourcepolicy.MVPDefault(), test.agentClient)

			_, _, err := service.RestartWithOptions(ctx, value.ID, "actor-1", OperationOptions{RuntimeProfileID: "dummy-process"})
			if (err != nil) != test.wantError {
				t.Fatalf("restart error = %v", err)
			}
			stored, loadErr := store.GetSession(ctx, value.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if stored.RuntimeProfileID != test.wantProfile {
				t.Fatalf("runtime profile = %q, want %q", stored.RuntimeProfileID, test.wantProfile)
			}
		})
	}
}

func TestCheckpointAfterStartCapturesRuntimeProfile(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-checkpoint", session.TypeShared, session.StateCreated))
	service := New(store, resourcepolicy.MVPDefault(), local.NewFake())

	if _, _, err := service.StartWithOptions(ctx, "session-checkpoint", "actor-1", OperationOptions{RuntimeProfileID: "dummy-process"}); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpointsvc.Create(ctx, store, checkpointsvc.CreateRequest{
		ID: "checkpoint-runtime-profile", SessionID: "session-checkpoint", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.RuntimeProfileID != "dummy-process" {
		t.Fatalf("checkpoint runtime profile = %q", cp.RuntimeProfileID)
	}
}

func TestRestartWithPreOpCheckpoint(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	value := testSession("session-preop", session.TypeShared, session.StateRunning)
	value.EnvironmentID = "env-1"
	createTestSession(t, store, value)

	checkpointCreated := false
	service := New(store, resourcepolicy.MVPDefault(), local.NewFake())
	service.WithPreOpCheckpoint(func(ctx context.Context, sessionID, actorID string) error {
		if sessionID != "session-preop" || actorID != "actor-1" {
			t.Errorf("pre-op checkpoint called with session=%q actor=%q", sessionID, actorID)
		}
		checkpointCreated = true
		return nil
	})

	_, _, err := service.RestartWithOptions(ctx, "session-preop", "actor-1", OperationOptions{CreatePreOpCheckpoint: true, RuntimeProfileID: "dummy-process"})
	if err != nil {
		t.Fatalf("restart error: %v", err)
	}
	if !checkpointCreated {
		t.Fatal("pre-op checkpoint function was not called")
	}
}

func TestRestartWithoutPreOpCheckpointDoesNotCallCallback(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	value := testSession("session-no-preop", session.TypeShared, session.StateRunning)
	value.EnvironmentID = "env-1"
	createTestSession(t, store, value)

	called := false
	service := New(store, resourcepolicy.MVPDefault(), local.NewFake())
	service.WithPreOpCheckpoint(func(ctx context.Context, sessionID, actorID string) error {
		called = true
		return nil
	})

	_, _, err := service.RestartWithOptions(ctx, "session-no-preop", "actor-1", OperationOptions{CreatePreOpCheckpoint: false, RuntimeProfileID: "dummy-process"})
	if err != nil {
		t.Fatalf("restart error: %v", err)
	}
	if called {
		t.Fatal("pre-op checkpoint should not be called when CreatePreOpCheckpoint=false")
	}
}
