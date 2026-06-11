package checkpoint

import (
	"errors"
	"time"
)

type Kind string

const (
	KindManual       Kind = "manual"
	KindPreOperation Kind = "pre-operation"
	KindMilestone    Kind = "milestone"
)

type Operation struct {
	Name      string    `json:"name"`
	ActorID   string    `json:"actorId"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

type Checkpoint struct {
	ID                string            `json:"id"`
	ProjectID         string            `json:"projectId"`
	RoomID            string            `json:"roomId"`
	SourceSessionID   string            `json:"sourceSessionId"`
	CreatorID         string            `json:"creatorId"`
	Kind              Kind              `json:"kind"`
	WorldStateRef     string            `json:"worldStateRef"`
	EnvironmentID     string            `json:"environmentId"`
	LucyLockHash      string            `json:"lucyLockHash"`
	ArtifactIDs       []string          `json:"artifactIds"`
	ServerConfig      map[string]string `json:"serverConfig"`
	CarpetRules       map[string]string `json:"carpetRules"`
	Seed              string            `json:"seed,omitempty"`
	GeneratorSettings map[string]string `json:"generatorSettings,omitempty"`
	Notes             string            `json:"notes,omitempty"`
	OperationHistory  []Operation       `json:"operationHistory"`
	CreatedAt         time.Time         `json:"createdAt"`
}

type CreateParams struct {
	ID                string
	ProjectID         string
	RoomID            string
	SourceSessionID   string
	CreatorID         string
	Kind              Kind
	WorldStateRef     string
	EnvironmentID     string
	LucyLockHash      string
	ArtifactIDs       []string
	ServerConfig      map[string]string
	CarpetRules       map[string]string
	Seed              string
	GeneratorSettings map[string]string
	Notes             string
	CreatedAt         time.Time
}

func New(params CreateParams) (Checkpoint, error) {
	if params.ID == "" || params.ProjectID == "" || params.SourceSessionID == "" || params.CreatorID == "" {
		return Checkpoint{}, errors.New("checkpoint requires id, project, source session, and creator")
	}
	if params.WorldStateRef == "" || params.EnvironmentID == "" {
		return Checkpoint{}, errors.New("checkpoint requires world state and environment references")
	}
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return Checkpoint{
		ID: params.ID, ProjectID: params.ProjectID, RoomID: params.RoomID,
		SourceSessionID: params.SourceSessionID, CreatorID: params.CreatorID,
		Kind: params.Kind, WorldStateRef: params.WorldStateRef, EnvironmentID: params.EnvironmentID,
		LucyLockHash: params.LucyLockHash, ArtifactIDs: cloneSlice(params.ArtifactIDs),
		ServerConfig: cloneMap(params.ServerConfig), CarpetRules: cloneMap(params.CarpetRules),
		Seed: params.Seed, GeneratorSettings: cloneMap(params.GeneratorSettings), Notes: params.Notes,
		OperationHistory: []Operation{}, CreatedAt: createdAt,
	}, nil
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneSlice(source []string) []string { return append([]string(nil), source...) }
