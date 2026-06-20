package serverproperties

import (
	"bufio"
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
		MinecraftVersion:   minecraftVersion,
		CapturedFrom:       "server.properties",
	}
}
