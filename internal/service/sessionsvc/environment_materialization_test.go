package sessionsvc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
)

func TestStartCallsEnvironmentMaterialization(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	calls := fake.Calls()
	found := false
	for _, call := range calls {
		if call == agent.OperationMaterializeEnvironment {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MaterializeEnvironment call, got calls: %v", calls)
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.State != session.StateRunning {
		t.Errorf("state: got %q, want %q", got.State, session.StateRunning)
	}
}

func TestMaterializationFailureBlocksStart(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	fake.SetFailure(agent.OperationMaterializeEnvironment, "materialization failed")
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.State != session.StateCreated {
		t.Errorf("state: got %q, want %q", got.State, session.StateCreated)
	}
}

func TestRestartCallsEnvironmentMaterialization(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := service.Stop(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := service.Restart(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	calls := fake.Calls()
	count := 0
	for _, call := range calls {
		if call == agent.OperationMaterializeEnvironment {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 MaterializeEnvironment calls (start + restart), got %d", count)
	}
}

func TestMaterializationFailureBlocksRestart(t *testing.T) {
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	fake := local.NewFake()
	service := New(store, resourcepolicy.MVPDefault(), fake)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := service.Stop(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	fake.SetFailure(agent.OperationMaterializeEnvironment, "materialization failed")
	if err := service.Restart(ctx, "session-1", "actor-1"); err == nil {
		t.Fatal("expected error, got nil")
	}
	got, err := store.GetSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.State != session.StateStopped {
		t.Errorf("state: got %q, want %q", got.State, session.StateStopped)
	}
}

func TestStartWritesManifest(t *testing.T) {
	ctx, store, _, root := newLifecycleTest(t, resourcepolicy.MVPDefault())
	runtimeRoot := filepath.Join(root, "runtime")
	processAgent, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, runtimeRoot)
	if err != nil {
		t.Fatalf("NewProcessAgentWithRegistryAndRoot: %v", err)
	}
	service := New(store, resourcepolicy.MVPDefault(), processAgent)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-1", nil }
	createTestSession(t, store, testSession("session-1", session.TypeShared, session.StateCreated))
	if err := service.Start(ctx, "session-1", "actor-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	manifestPath := filepath.Join(runtimeRoot, "sessions", "session-1", "config", "environment-materialization.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest not found: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["session_id"] != "session-1" {
		t.Errorf("session_id: got %v, want session-1", manifest["session_id"])
	}
	if manifest["status"] != "prepared" {
		t.Errorf("status: got %v, want prepared", manifest["status"])
	}
}
