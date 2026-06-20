package serverproperties

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/stratummc/stratum/internal/checkpoint"
)

type WorldConfig struct {
	LevelSeed          string
	LevelType          string
	GeneratorSettings  string
	GenerateStructures bool
	Difficulty         string
	SpawnProtection    int
	ViewDistance       int
}

func Parse(r io.Reader) (WorldConfig, error) {
	cfg := WorldConfig{
		GenerateStructures: true,
		SpawnProtection:    16,
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "level-seed":
			cfg.LevelSeed = value
		case "level-type":
			cfg.LevelType = value
		case "generator-settings":
			cfg.GeneratorSettings = value
		case "generate-structures":
			cfg.GenerateStructures = value == "true"
		case "difficulty":
			cfg.Difficulty = value
		case "spawn-protection":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.SpawnProtection = v
			}
		case "view-distance":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.ViewDistance = v
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return WorldConfig{}, err
	}

	return cfg, nil
}

func ToWorldProfileSnapshot(cfg WorldConfig, minecraftVersion string) *checkpoint.WorldProfileSnapshot {
	levelType := cfg.LevelType
	if levelType == "" {
		levelType = "default"
	}

	difficulty := cfg.Difficulty
	if difficulty == "" {
		difficulty = "normal"
	}

	return &checkpoint.WorldProfileSnapshot{
		Seed:               cfg.LevelSeed,
		LevelType:          levelType,
		GeneratorSettings:  cfg.GeneratorSettings,
		GenerateStructures: cfg.GenerateStructures,
		SpawnRadius:        cfg.SpawnProtection,
		Difficulty:         difficulty,
		ViewDistance:       cfg.ViewDistance,
		MinecraftVersion:   minecraftVersion,
		CapturedFrom:       "server.properties",
	}
}

func FromWorldProfileSnapshot(snapshot *checkpoint.WorldProfileSnapshot) string {
	var b strings.Builder
	b.WriteString("# Minecraft server properties\n")
	b.WriteString("# Applied from checkpoint world profile snapshot\n")
	writeSnapshotFields(&b, snapshot, nil)
	return b.String()
}

func MergeWithWorldProfileSnapshot(current []byte, snapshot *checkpoint.WorldProfileSnapshot, fields []string) string {
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[f] = true
	}
	var b strings.Builder
	b.WriteString("# Minecraft server properties\n")
	b.WriteString("# Merged with checkpoint world profile fields\n")
	b.WriteString(string(current))
	if fieldSet["seed"] {
		if snapshot.Seed != "" {
			b.WriteString(fmt.Sprintf("level-seed=%s\n", snapshot.Seed))
		}
	}
	if fieldSet["level-type"] {
		b.WriteString(fmt.Sprintf("level-type=%s\n", snapshot.LevelType))
	}
	if fieldSet["generator-settings"] {
		if snapshot.GeneratorSettings != "" {
			b.WriteString(fmt.Sprintf("generator-settings=%s\n", snapshot.GeneratorSettings))
		}
	}
	if fieldSet["generate-structures"] {
		b.WriteString(fmt.Sprintf("generate-structures=%t\n", snapshot.GenerateStructures))
	}
	if fieldSet["spawn-radius"] {
		b.WriteString(fmt.Sprintf("spawn-protection=%d\n", snapshot.SpawnRadius))
	}
	if fieldSet["difficulty"] {
		b.WriteString(fmt.Sprintf("difficulty=%s\n", snapshot.Difficulty))
	}
	if fieldSet["view-distance"] {
		if snapshot.ViewDistance > 0 {
			b.WriteString(fmt.Sprintf("view-distance=%d\n", snapshot.ViewDistance))
		}
	}
	return b.String()
}

func writeSnapshotFields(b *strings.Builder, snapshot *checkpoint.WorldProfileSnapshot, fields map[string]bool) {
	if fields == nil || fields["seed"] {
		if snapshot.Seed != "" {
			b.WriteString(fmt.Sprintf("level-seed=%s\n", snapshot.Seed))
		}
	}
	if fields == nil || fields["level-type"] {
		b.WriteString(fmt.Sprintf("level-type=%s\n", snapshot.LevelType))
	}
	if fields == nil || fields["generator-settings"] {
		if snapshot.GeneratorSettings != "" {
			b.WriteString(fmt.Sprintf("generator-settings=%s\n", snapshot.GeneratorSettings))
		}
	}
	if fields == nil || fields["generate-structures"] {
		b.WriteString(fmt.Sprintf("generate-structures=%t\n", snapshot.GenerateStructures))
	}
	if fields == nil || fields["spawn-radius"] {
		b.WriteString(fmt.Sprintf("spawn-protection=%d\n", snapshot.SpawnRadius))
	}
	if fields == nil || fields["difficulty"] {
		b.WriteString(fmt.Sprintf("difficulty=%s\n", snapshot.Difficulty))
	}
	if fields == nil || fields["view-distance"] {
		if snapshot.ViewDistance > 0 {
			b.WriteString(fmt.Sprintf("view-distance=%d\n", snapshot.ViewDistance))
		}
	}
}
