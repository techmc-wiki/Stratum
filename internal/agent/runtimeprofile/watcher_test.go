package runtimeprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDetectsNewProfile(t *testing.T) {
	dir := t.TempDir()
	registry := Builtins()

	writeProfileJSON(t, dir, "test-watch.json", []profileConfig{{
		ID:           "watched-profile",
		Name:         "Watched",
		RuntimeType:  TypeDummy,
		StopStrategy: StopNone,
		LogMode:      LogMemory,
		Enabled:      true,
	}})

	watcher := NewWatcher(registry, dir, 100*time.Millisecond)
	watcher.Start()
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	profile, err := registry.Get("watched-profile")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "Watched" {
		t.Fatalf("name = %q", profile.Name)
	}
}

func TestWatcherDetectsModifiedProfile(t *testing.T) {
	dir := t.TempDir()
	registry := Builtins()

	writeProfileJSON(t, dir, "test-change.json", []profileConfig{{
		ID:           "change-me",
		Name:         "Before",
		RuntimeType:  TypeDummy,
		StopStrategy: StopNone,
		LogMode:      LogMemory,
		Enabled:      true,
	}})

	watcher := NewWatcher(registry, dir, 100*time.Millisecond)
	watcher.Start()
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	writeProfileJSON(t, dir, "test-change.json", []profileConfig{{
		ID:           "change-me",
		Name:         "After",
		RuntimeType:  TypeDummy,
		StopStrategy: StopNone,
		LogMode:      LogMemory,
		Enabled:      true,
	}})

	time.Sleep(200 * time.Millisecond)

	profile, err := registry.Get("change-me")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "After" {
		t.Fatalf("name = %q, want After", profile.Name)
	}
}

func TestWatcherDetectsRemovedProfile(t *testing.T) {
	dir := t.TempDir()
	registry := Builtins()

	writeProfileJSON(t, dir, "remove-me.json", []profileConfig{{
		ID:           "remove-me",
		Name:         "Gone",
		RuntimeType:  TypeDummy,
		StopStrategy: StopNone,
		LogMode:      LogMemory,
		Enabled:      true,
	}})

	watcher := NewWatcher(registry, dir, 100*time.Millisecond)
	watcher.Start()
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)

	if _, err := registry.Get("remove-me"); err != nil {
		t.Fatal(err)
	}

	os.Remove(filepath.Join(dir, "remove-me.json"))
	time.Sleep(200 * time.Millisecond)

	if _, err := registry.Get("remove-me"); err == nil {
		t.Fatal("expected profile to be removed")
	}
}

func TestWatcherSkipsNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	registry := Builtins()

	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)

	watcher := NewWatcher(registry, dir, 100*time.Millisecond)
	watcher.Start()
	defer watcher.Stop()

	time.Sleep(200 * time.Millisecond)
}

func writeProfileJSON(t *testing.T, dir, name string, profiles []profileConfig) {
	t.Helper()
	doc := configDocument{RuntimeProfiles: profiles}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
