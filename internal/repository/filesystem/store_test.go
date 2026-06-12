package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
)

var testTime = time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

func TestProjectCreateGetListRoundTripAndReload(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	store := newTestStore(t, root)
	want := project.Project{ID: "project-1", Name: "Project One", Members: []project.Member{}, CreatedAt: testTime}
	if err := store.CreateProject(ctx, want); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetProject(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("get = %+v, want %+v", got, want)
	}

	reloaded := newTestStore(t, root)
	values, err := reloaded.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || !reflect.DeepEqual(values[0], want) {
		t.Fatalf("reloaded projects = %+v", values)
	}
}

func TestSessionCreateGetListRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, filepath.Join(t.TempDir(), "data"))
	want := session.Session{
		ID: "session-1", ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "user-1",
		Type: session.TypeShared, State: session.StateCreated, EnvironmentID: "environment-1",
		CreatedAt: testTime, LastActiveAt: testTime,
	}
	if err := store.CreateSession(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	values, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || len(values) != 1 || !reflect.DeepEqual(values[0], want) {
		t.Fatalf("get = %+v, list = %+v", got, values)
	}
}

func TestCheckpointAndArtifactPersistence(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, filepath.Join(t.TempDir(), "data"))
	cp := checkpoint.Checkpoint{
		ID: "checkpoint-1", ProjectID: "project-1", RoomID: "room-1", SourceSessionID: "session-1",
		CreatorID: "user-1", Kind: checkpoint.KindManual, WorldStateRef: "metadata-only://session/session-1",
		EnvironmentID: "environment-1", ArtifactIDs: []string{}, ServerConfig: map[string]string{},
		CarpetRules: map[string]string{}, OperationHistory: []checkpoint.Operation{}, CreatedAt: testTime,
	}
	if err := store.CreateCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetCheckpoint(ctx, cp.ID); err != nil || !reflect.DeepEqual(got, cp) {
		t.Fatalf("checkpoint = %+v, err = %v", got, err)
	}

	artifactValue := artifact.Artifact{
		ID: "artifact-1", Name: "Test Mod", Type: artifact.TypeJar, UploaderID: "user-1",
		SHA256: artifact.HashBytes([]byte("artifact")), SizeBytes: 8,
		TargetMinecraftVersions: []string{"1.17"}, LoaderCompatibility: []string{"fabric"},
		Status: artifact.StatusPending, CreatedAt: testTime,
	}
	if err := store.CreateArtifact(ctx, artifactValue); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetArtifact(ctx, artifactValue.ID); err != nil || !reflect.DeepEqual(got, artifactValue) {
		t.Fatalf("artifact = %+v, err = %v", got, err)
	}
}

func TestRuntimeObservationCreateGetListRoundTripAndReload(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	store := newTestStore(t, root)
	exitCode := 1
	want := runtimeobservation.Observation{
		ID:                     "runtime-observation-1",
		SessionID:              "session-1",
		ProjectID:              "project-1",
		RoomID:                 "room-1",
		ObservedAt:             testTime,
		ObserverAgentID:        "agent-1",
		ControllerSessionState: "running",
		AgentRuntimeStatus:     "crashed",
		RuntimeProfileID:       "dummy-process",
		ProcessID:              "process-1",
		PID:                    42,
		ExitCode:               &exitCode,
		Crashed:                true,
		LastError:              "runtime crashed",
		LogsAvailable:          true,
		MismatchDetected:       true,
		MismatchType:           runtimeobservation.MismatchControllerRunningAgentCrashed,
		Severity:               runtimeobservation.SeverityCritical,
		RecommendedAction:      runtimeobservation.ActionMarkCrashed,
		Metadata:               map[string]string{"source": "test"},
	}
	if err := store.CreateRuntimeObservation(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRuntimeObservation(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("get = %+v, want %+v", got, want)
	}

	reloaded := newTestStore(t, root)
	values, err := reloaded.ListRuntimeObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bySession, err := reloaded.ListRuntimeObservationsBySession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || !reflect.DeepEqual(values[0], want) || len(bySession) != 1 || !reflect.DeepEqual(bySession[0], want) {
		t.Fatalf("list=%+v bySession=%+v want=%+v", values, bySession, want)
	}
}

func TestUpdatePersistence(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	store := newTestStore(t, root)
	value := project.Project{ID: "project-1", Name: "Before", Members: []project.Member{}, CreatedAt: testTime}
	if err := store.CreateProject(ctx, value); err != nil {
		t.Fatal(err)
	}
	value.Name = "After"
	if err := store.UpdateProject(ctx, value); err != nil {
		t.Fatal(err)
	}
	reloaded := newTestStore(t, root)
	got, err := reloaded.GetProject(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "After" {
		t.Fatalf("name = %q, want After", got.Name)
	}
}

func TestDeleteBehavior(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, filepath.Join(t.TempDir(), "data"))
	value := session.Session{ID: "session-1", ProjectID: "project-1", OwnerUserID: "user-1", Type: session.TypeFork, State: session.StateCreated, EnvironmentID: "environment-1", CreatedAt: testTime, LastActiveAt: testTime}
	if err := store.CreateSession(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSession(ctx, value.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession(ctx, value.ID); !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		t.Fatalf("get after delete error = %v", err)
	}
	if err := store.DeleteSession(ctx, value.ID); !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}

func TestAuditJSONLAppend(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "data")
	store := newTestStore(t, root)
	events := []audit.Event{
		{ID: "event-1", ActorID: "user-1", Action: "project.create", TargetType: "project", TargetID: "project-1", CreatedAt: testTime},
		{ID: "event-2", ActorID: "user-1", Action: "room.create", TargetType: "room", TargetID: "room-1", CreatedAt: testTime.Add(time.Second)},
	}
	for _, event := range events {
		if err := store.AppendAuditEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	reloaded := newTestStore(t, root)
	got, err := reloaded.ListAuditEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("events = %+v, want %+v", got, events)
	}
}

func TestAtomicWritePreservesExistingFileOnEncodeFailure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "value.json")
	want := []byte("{\"stable\":true}\n")
	if err := os.WriteFile(path, want, 0o640); err != nil {
		t.Fatal(err)
	}
	badValue := struct {
		Unsupported chan int `json:"unsupported"`
	}{Unsupported: make(chan int)}
	if err := writeJSONAtomic(path, "test.atomic", badValue); err == nil {
		t.Fatal("expected encoding to fail")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("file changed after failed write: %q", got)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "value.json" {
		t.Fatalf("temporary files remain: %+v", entries)
	}
}

func TestMetadataIDRejectsPathTraversal(t *testing.T) {
	store := newTestStore(t, filepath.Join(t.TempDir(), "data"))
	value := project.Project{ID: "../outside", Name: "Outside", CreatedAt: testTime}
	if err := store.CreateProject(context.Background(), value); !stratumerrors.IsKind(err, stratumerrors.KindValidation) {
		t.Fatalf("error = %v", err)
	}
}

func newTestStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
