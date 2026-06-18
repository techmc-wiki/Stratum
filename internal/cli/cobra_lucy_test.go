package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func TestLucyCommandGroupRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"lucy", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"plan", "lock", "status", "verify", "install"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q: %q", want, output)
		}
	}
}

func TestLucyPlanOutputsActions(t *testing.T) {
	store := testLucyStore(t)
	cmd, stdout, stderr := testLucyCommand(store, lucyFakeAgent{})
	cmd.SetArgs([]string{"lucy", "plan", "env-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%q", err, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "actions=2") || !strings.Contains(output, "warnings=1") {
		t.Fatalf("stdout=%q", output)
	}
}

func TestLucyVerifyOutputsIntegrity(t *testing.T) {
	store := testLucyStore(t)
	cmd, stdout, stderr := testLucyCommand(store, lucyFakeAgent{})
	cmd.SetArgs([]string{"lucy", "verify", "session-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%q", err, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "status=ok") || !strings.Contains(output, "ok=true") {
		t.Fatalf("stdout=%q", output)
	}
}

func TestLucyInstallSkipsNoop(t *testing.T) {
	store := testLucyStore(t)
	cmd, stdout, stderr := testLucyCommand(store, lucyNoopInstallAgent{})
	cmd.SetArgs([]string{"lucy", "install", "session-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v stderr=%q", err, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "status=not_capable") {
		t.Fatalf("stdout=%q", output)
	}
}

type lucyFakeAgent struct{ *local.Fake }

func (a lucyFakeAgent) base() *local.Fake {
	if a.Fake != nil {
		return a.Fake
	}
	return local.NewFake()
}

func (a lucyFakeAgent) MaterializeEnvironment(_ context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	return agent.EnvironmentMaterializationResult{
		SessionID:            request.SessionID,
		EnvironmentID:        request.EnvironmentID,
		EnvironmentName:      request.EnvironmentName,
		MinecraftVersion:     request.MinecraftVersion,
		LoaderType:           request.LoaderType,
		ServerCore:           request.ServerCore,
		LucyResolutionStatus: "resolved",
		LucyLockHash:         "hash-1",
		LucyLockPath:         "sessions/session-1/config/lucy-lock.yaml",
		LucyIntegrityStatus:  "ok",
		MaterializedAt:       time.Now().UTC(),
		Status:               "prepared",
		Directories:          []string{"config", "mods"},
		Metadata: map[string]string{
			"lucyPlanActionCount":        "2",
			"lucyPlanWarningCount":       "1",
			"lucyPlanErrorCount":         "0",
			"lucyPlanRequiresLockUpdate": "true",
			"lucyLockPackageCount":       "1",
			"lucyLockArtifactCount":      "0",
			"lucyIntegrityStatus":        "ok",
			"lucyIntegrityMissing":       "0",
			"lucyIntegrityCorrupt":       "0",
			"lucyIntegrityChecked":       "1",
		},
	}, nil
}

func (a lucyFakeAgent) GetSessionRuntimeStatus(_ context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	return agent.SessionRuntimeStatus{SessionID: sessionID, EnvironmentManifest: &agent.EnvironmentManifestStatus{Exists: true, RuntimeRelativePath: "sessions/session-1/config/environment-materialization.json", Status: "prepared", LucyLockHash: "hash-1"}}, nil
}

type lucyNoopInstallAgent struct{ lucyFakeAgent }

func (a lucyNoopInstallAgent) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	result, err := a.lucyFakeAgent.MaterializeEnvironment(ctx, request)
	result.LucyInstallStatus = ""
	return result, err
}

func testLucyCommand(store *filesystem.Store, agentClient agent.AgentClient) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runtime := &commandRuntime{stdout: stdout, stderr: stderr, store: store, agentClient: agentClient}
	cmd := newRootCommand(runtime)
	cmd.SetContext(runtime.context())
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}

func testLucyStore(t *testing.T) *filesystem.Store {
	t.Helper()
	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.CreateProject(ctx, project.Project{ID: "project-1", Name: "Project", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEnvironment(ctx, environment.Environment{ID: "env-1", Name: "Environment", MinecraftVersion: "1.17.1", LoaderType: environment.LoaderFabric, ServerCore: environment.ServerCarpet, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRoom(ctx, room.Room{ID: "room-1", ProjectID: "project-1", Name: "Room", EnvironmentID: "env-1", BaseWorldRef: "base-world", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "alice", Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "env-1", CreatedAt: now, LastActiveAt: now}); err != nil {
		t.Fatal(err)
	}
	return store
}
