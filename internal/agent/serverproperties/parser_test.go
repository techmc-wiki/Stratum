package serverproperties

import (
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/checkpoint"
)

func TestParseBasicProperties(t *testing.T) {
	input := `# Minecraft server properties
level-seed=12345
level-type=flat
difficulty=peaceful
generate-structures=false
spawn-protection=5
view-distance=12
`
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LevelSeed != "12345" {
		t.Errorf("LevelSeed = %q, want 12345", cfg.LevelSeed)
	}
	if cfg.LevelType != "flat" {
		t.Errorf("LevelType = %q, want flat", cfg.LevelType)
	}
	if cfg.Difficulty != "peaceful" {
		t.Errorf("Difficulty = %q, want peaceful", cfg.Difficulty)
	}
	if cfg.GenerateStructures != false {
		t.Errorf("GenerateStructures = %v, want false", cfg.GenerateStructures)
	}
	if cfg.SpawnProtection != 5 {
		t.Errorf("SpawnProtection = %d, want 5", cfg.SpawnProtection)
	}
	if cfg.ViewDistance != 12 {
		t.Errorf("ViewDistance = %d, want 12", cfg.ViewDistance)
	}
}

func TestParseDefaults(t *testing.T) {
	input := `# Empty properties
`
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GenerateStructures != true {
		t.Errorf("GenerateStructures default = %v, want true", cfg.GenerateStructures)
	}
	if cfg.SpawnProtection != 16 {
		t.Errorf("SpawnProtection default = %d, want 16", cfg.SpawnProtection)
	}
}

func TestParseGeneratorSettings(t *testing.T) {
	input := `generator-settings={"layers":[{"block":"stone","height":1}]}
level-type=flat
`
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	expected := `{"layers":[{"block":"stone","height":1}]}`
	if cfg.GeneratorSettings != expected {
		t.Errorf("GeneratorSettings = %q, want %q", cfg.GeneratorSettings, expected)
	}
}

func TestParseIgnoresComments(t *testing.T) {
	input := `# This is a comment
level-seed=abc
# level-type=ignored
level-type=amplified
`
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LevelSeed != "abc" {
		t.Errorf("LevelSeed = %q", cfg.LevelSeed)
	}
	if cfg.LevelType != "amplified" {
		t.Errorf("LevelType = %q", cfg.LevelType)
	}
}

func TestToWorldProfileSnapshot(t *testing.T) {
	cfg := WorldConfig{
		LevelSeed:          "54321",
		LevelType:          "amplified",
		GeneratorSettings:  `{"test":"value"}`,
		GenerateStructures: true,
		Difficulty:         "hard",
		SpawnProtection:    20,
		ViewDistance:       16,
	}

	snapshot := ToWorldProfileSnapshot(cfg, "1.17.1")

	if snapshot.Seed != "54321" {
		t.Errorf("Seed = %q", snapshot.Seed)
	}
	if snapshot.LevelType != "amplified" {
		t.Errorf("LevelType = %q", snapshot.LevelType)
	}
	if snapshot.GeneratorSettings != `{"test":"value"}` {
		t.Errorf("GeneratorSettings = %q", snapshot.GeneratorSettings)
	}
	if snapshot.GenerateStructures != true {
		t.Errorf("GenerateStructures = %v", snapshot.GenerateStructures)
	}
	if snapshot.Difficulty != "hard" {
		t.Errorf("Difficulty = %q", snapshot.Difficulty)
	}
	if snapshot.SpawnRadius != 20 {
		t.Errorf("SpawnRadius = %d", snapshot.SpawnRadius)
	}
	if snapshot.ViewDistance != 16 {
		t.Errorf("ViewDistance = %d", snapshot.ViewDistance)
	}
	if snapshot.MinecraftVersion != "1.17.1" {
		t.Errorf("MinecraftVersion = %q", snapshot.MinecraftVersion)
	}
	if snapshot.CapturedFrom != "server.properties" {
		t.Errorf("CapturedFrom = %q", snapshot.CapturedFrom)
	}
}

func TestToWorldProfileSnapshotDefaults(t *testing.T) {
	cfg := WorldConfig{}
	snapshot := ToWorldProfileSnapshot(cfg, "")

	if snapshot.LevelType != "default" {
		t.Errorf("LevelType default = %q, want default", snapshot.LevelType)
	}
	if snapshot.Difficulty != "normal" {
		t.Errorf("Difficulty default = %q, want normal", snapshot.Difficulty)
	}
}

func TestFromWorldProfileSnapshot(t *testing.T) {
	snapshot := &checkpoint.WorldProfileSnapshot{
		Seed:               "12345",
		LevelType:          "flat",
		GeneratorSettings:  `{"layers":[{"block":"stone","height":1}]}`,
		GenerateStructures: false,
		SpawnRadius:        8,
		Difficulty:         "hard",
		ViewDistance:       12,
		MinecraftVersion:   "1.17.1",
	}
	props := FromWorldProfileSnapshot(snapshot)
	if !strings.Contains(props, "level-seed=12345") {
		t.Errorf("missing level-seed")
	}
	if !strings.Contains(props, "level-type=flat") {
		t.Errorf("missing level-type")
	}
	if !strings.Contains(props, `generator-settings={"layers":[{"block":"stone","height":1}]}`) {
		t.Errorf("missing generator-settings")
	}
	if !strings.Contains(props, "generate-structures=false") {
		t.Errorf("missing generate-structures")
	}
	if !strings.Contains(props, "spawn-protection=8") {
		t.Errorf("missing spawn-protection")
	}
	if !strings.Contains(props, "difficulty=hard") {
		t.Errorf("missing difficulty")
	}
	if !strings.Contains(props, "view-distance=12") {
		t.Errorf("missing view-distance")
	}
}
