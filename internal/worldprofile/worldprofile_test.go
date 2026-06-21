package worldprofile

import (
	"strings"
	"testing"
)

func TestNewRequiresIDAndName(t *testing.T) {
	_, err := New(CreateParams{})
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Fatalf("expected id required, got %v", err)
	}

	_, err = New(CreateParams{ID: "wp1"})
	if err == nil || !strings.Contains(err.Error(), "name required") {
		t.Fatalf("expected name required, got %v", err)
	}
}

func TestNewValidatesLevelType(t *testing.T) {
	_, err := New(CreateParams{
		ID:         "wp1",
		Name:       "Test",
		LevelType:  "invalid",
		Difficulty: DifficultyNormal,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid level type") {
		t.Fatalf("expected level type validation, got %v", err)
	}
}

func TestNewValidatesDifficulty(t *testing.T) {
	_, err := New(CreateParams{
		ID:         "wp1",
		Name:       "Test",
		LevelType:  LevelDefault,
		Difficulty: "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid difficulty") {
		t.Fatalf("expected difficulty validation, got %v", err)
	}
}

func TestNewValidatesGeneratorSettingsJSON(t *testing.T) {
	_, err := New(CreateParams{
		ID:                "wp1",
		Name:              "Test",
		LevelType:         LevelFlat,
		Difficulty:        DifficultyNormal,
		GeneratorSettings: "not-json",
	})
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("expected JSON validation, got %v", err)
	}
}

func TestNewDefaultSpawnRadius(t *testing.T) {
	wp, err := New(CreateParams{
		ID:         "wp1",
		Name:       "Test",
		LevelType:  LevelDefault,
		Difficulty: DifficultyNormal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if wp.SpawnRadius != 10 {
		t.Fatalf("SpawnRadius = %d, want 10", wp.SpawnRadius)
	}
}
