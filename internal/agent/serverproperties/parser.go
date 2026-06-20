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
	if snapshot.Seed != "" {
		b.WriteString(fmt.Sprintf("level-seed=%s\n", snapshot.Seed))
	}
	b.WriteString(fmt.Sprintf("level-type=%s\n", snapshot.LevelType))
	if snapshot.GeneratorSettings != "" {
		b.WriteString(fmt.Sprintf("generator-settings=%s\n", snapshot.GeneratorSettings))
	}
	b.WriteString(fmt.Sprintf("generate-structures=%t\n", snapshot.GenerateStructures))
	b.WriteString(fmt.Sprintf("spawn-protection=%d\n", snapshot.SpawnRadius))
	b.WriteString(fmt.Sprintf("difficulty=%s\n", snapshot.Difficulty))
	if snapshot.ViewDistance > 0 {
		b.WriteString(fmt.Sprintf("view-distance=%d\n", snapshot.ViewDistance))
	}
	return b.String()
}
