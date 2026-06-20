package worldprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type LevelType string

const (
	LevelDefault     LevelType = "default"
	LevelFlat        LevelType = "flat"
	LevelLargeBiomes LevelType = "largeBiomes"
	LevelAmplified   LevelType = "amplified"
	LevelBuffet      LevelType = "buffet"
	LevelCustomized  LevelType = "customized"
)

type Difficulty string

const (
	DifficultyPeaceful Difficulty = "peaceful"
	DifficultyEasy     Difficulty = "easy"
	DifficultyNormal   Difficulty = "normal"
	DifficultyHard     Difficulty = "hard"
)

type WorldProfile struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description,omitempty"`
	Seed               string            `json:"seed,omitempty"`
	LevelType          LevelType         `json:"levelType"`
	GeneratorSettings  string            `json:"generatorSettings,omitempty"`
	GenerateStructures bool              `json:"generateStructures"`
	SpawnRadius        int               `json:"spawnRadius"`
	Difficulty         Difficulty        `json:"difficulty"`
	ViewDistance       int               `json:"viewDistance,omitempty"`
	MinecraftVersion   string            `json:"minecraftVersion,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type CreateParams struct {
	ID                 string
	Name               string
	Description        string
	Seed               string
	LevelType          LevelType
	GeneratorSettings  string
	GenerateStructures bool
	SpawnRadius        int
	Difficulty         Difficulty
	ViewDistance       int
	MinecraftVersion   string
	Metadata           map[string]string
}

func New(params CreateParams) (WorldProfile, error) {
	if params.ID == "" {
		return WorldProfile{}, errors.New("id required")
	}
	if params.Name == "" {
		return WorldProfile{}, errors.New("name required")
	}

	spawnRadius := params.SpawnRadius
	if spawnRadius == 0 {
		spawnRadius = 10
	}

	wp := WorldProfile{
		ID:                 params.ID,
		Name:               params.Name,
		Description:        params.Description,
		Seed:               params.Seed,
		LevelType:          params.LevelType,
		GeneratorSettings:  params.GeneratorSettings,
		GenerateStructures: params.GenerateStructures,
		SpawnRadius:        spawnRadius,
		Difficulty:         params.Difficulty,
		ViewDistance:       params.ViewDistance,
		MinecraftVersion:   params.MinecraftVersion,
		CreatedAt:          time.Now().UTC(),
		Metadata:           cloneMap(params.Metadata),
	}

	if err := wp.Validate(); err != nil {
		return WorldProfile{}, err
	}

	return wp, nil
}

func (w WorldProfile) Validate() error {
	if w.ID == "" {
		return errors.New("id required")
	}
	if w.Name == "" {
		return errors.New("name required")
	}
	if !isValidLevelType(w.LevelType) {
		return fmt.Errorf("invalid level type: %q", w.LevelType)
	}
	if !isValidDifficulty(w.Difficulty) {
		return fmt.Errorf("invalid difficulty: %q", w.Difficulty)
	}

	if w.GeneratorSettings != "" {
		var tmp interface{}
		if err := json.Unmarshal([]byte(w.GeneratorSettings), &tmp); err != nil {
			return fmt.Errorf("generator settings must be valid JSON: %w", err)
		}
	}

	if w.SpawnRadius < 0 {
		return errors.New("spawn radius must be non-negative")
	}

	return nil
}

func isValidLevelType(lt LevelType) bool {
	switch lt {
	case LevelDefault, LevelFlat, LevelLargeBiomes, LevelAmplified, LevelBuffet, LevelCustomized:
		return true
	}
	return false
}

func isValidDifficulty(d Difficulty) bool {
	switch d {
	case DifficultyPeaceful, DifficultyEasy, DifficultyNormal, DifficultyHard:
		return true
	}
	return false
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
