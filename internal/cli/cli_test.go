package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

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
	if len(events) != 3 {
		t.Fatalf("events = %d", len(events))
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
