package checkpoint

import (
	"errors"
	"time"

	"github.com/stratummc/stratum/internal/checkpoint/consistency"
)

type Kind string

const (
	KindManual       Kind = "manual"
	KindPreOperation Kind = "pre-operation"
	KindMilestone    Kind = "milestone"
)

type Status string

const (
	StatusMetadataOnly Status = "metadata_only"
	StatusComplete     Status = "complete"
)

type Operation struct {
	Name      string    `json:"name"`
	ActorID   string    `json:"actorId"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

type RuntimeStatusSnapshot struct {
	CapturedAt                 time.Time `json:"capturedAt"`
	SessionID                  string    `json:"sessionId"`
	RuntimeRootExists          bool      `json:"runtimeRootExists"`
	SessionRootExists          bool      `json:"sessionRootExists"`
	EnvironmentManifestExists  bool      `json:"environmentManifestExists"`
	EnvironmentID              string    `json:"environmentId,omitempty"`
	MinecraftVersion           string    `json:"minecraftVersion,omitempty"`
	LoaderType                 string    `json:"loaderType,omitempty"`
	ServerCore                 string    `json:"serverCore,omitempty"`
	RuntimeProfileID           string    `json:"runtimeProfileId,omitempty"`
	MCDRRootExists             bool      `json:"mcdrRootExists"`
	MCDRLayoutManifestExists   bool      `json:"mcdrLayoutManifestExists"`
	MaterializedArtifactsCount int       `json:"materializedArtifactsCount"`
	AppliedArtifactsCount      int       `json:"appliedArtifactsCount"`
	ProcessState               string    `json:"processState"`
	PID                        int       `json:"pid,omitempty"`
	OverallStatus              string    `json:"overallStatus"`
	Issues                     []string  `json:"issues,omitempty"`
}

type Checkpoint struct {
	ID                                    string                 `json:"id"`
	ProjectID                             string                 `json:"projectId"`
	RoomID                                string                 `json:"roomId"`
	SourceSessionID                       string                 `json:"sourceSessionId"`
	CreatorID                             string                 `json:"creatorId"`
	Kind                                  Kind                   `json:"kind"`
	Status                                Status                 `json:"status"`
	ConsistencyLevel                      consistency.Level      `json:"consistencyLevel"`
	ConsistencyMetadata                   map[string]string      `json:"consistencyMetadata,omitempty"`
	EnvironmentID                         string                 `json:"environmentId"`
	RuntimeProfileID                      string                 `json:"runtimeProfileId,omitempty"`
	WorldStateRef                         string                 `json:"worldStateRef,omitempty"`
	LucyLockHash                          string                 `json:"lucyLockHash,omitempty"`
	ArtifactIDs                           []string               `json:"artifactIds,omitempty"`
	AppliedArtifactRefs                   []string               `json:"appliedArtifactRefs,omitempty"`
	EnvironmentMaterializationManifestRef string                 `json:"environmentMaterializationManifestRef,omitempty"`
	RuntimeStatusSummary                  string                 `json:"runtimeStatusSummary,omitempty"`
	RuntimeStatusSnapshot                 *RuntimeStatusSnapshot `json:"runtimeStatusSnapshot,omitempty"`
	ServerConfig                          map[string]string      `json:"serverConfig,omitempty"`
	CarpetRules                           map[string]string      `json:"carpetRules,omitempty"`
	Seed                                  string                 `json:"seed,omitempty"`
	GeneratorSettings                     map[string]string      `json:"generatorSettings,omitempty"`
	Notes                                 string                 `json:"notes,omitempty"`
	OperationHistory                      []Operation            `json:"operationHistory,omitempty"`
	Metadata                              map[string]string      `json:"metadata,omitempty"`
	CreatedAt                             time.Time              `json:"createdAt"`
}

type CreateParams struct {
	ID                                    string
	ProjectID                             string
	RoomID                                string
	SourceSessionID                       string
	CreatorID                             string
	Kind                                  Kind
	Status                                Status
	ConsistencyLevel                      consistency.Level
	ConsistencyMetadata                   map[string]string
	EnvironmentID                         string
	RuntimeProfileID                      string
	WorldStateRef                         string
	LucyLockHash                          string
	ArtifactIDs                           []string
	AppliedArtifactRefs                   []string
	EnvironmentMaterializationManifestRef string
	RuntimeStatusSummary                  string
	RuntimeStatusSnapshot                 *RuntimeStatusSnapshot
	ServerConfig                          map[string]string
	CarpetRules                           map[string]string
	Seed                                  string
	GeneratorSettings                     map[string]string
	Notes                                 string
	Metadata                              map[string]string
	CreatedAt                             time.Time
}

func New(params CreateParams) (Checkpoint, error) {
	if params.ID == "" || params.SourceSessionID == "" || params.CreatorID == "" {
		return Checkpoint{}, errors.New("checkpoint requires id, source session, and creator")
	}
	if params.Status == StatusMetadataOnly && params.EnvironmentID == "" {
		return Checkpoint{}, errors.New("metadata-only checkpoint requires environment id")
	}
	consistencyLevel := params.ConsistencyLevel
	if consistencyLevel == "" {
		consistencyLevel = consistency.LevelMetadataOnly
	}
	if err := consistencyLevel.Validate(); err != nil {
		return Checkpoint{}, err
	}
	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return Checkpoint{
		ID: params.ID, ProjectID: params.ProjectID, RoomID: params.RoomID,
		SourceSessionID: params.SourceSessionID, CreatorID: params.CreatorID,
		Kind: params.Kind, Status: params.Status, ConsistencyLevel: consistencyLevel,
		ConsistencyMetadata: cloneMap(params.ConsistencyMetadata), EnvironmentID: params.EnvironmentID,
		RuntimeProfileID: params.RuntimeProfileID, WorldStateRef: params.WorldStateRef,
		LucyLockHash: params.LucyLockHash, ArtifactIDs: cloneSlice(params.ArtifactIDs),
		AppliedArtifactRefs:                   cloneSlice(params.AppliedArtifactRefs),
		EnvironmentMaterializationManifestRef: params.EnvironmentMaterializationManifestRef,
		RuntimeStatusSummary:                  params.RuntimeStatusSummary,
		RuntimeStatusSnapshot:                 cloneRuntimeStatusSnapshot(params.RuntimeStatusSnapshot),
		ServerConfig:                          cloneMap(params.ServerConfig), CarpetRules: cloneMap(params.CarpetRules),
		Seed: params.Seed, GeneratorSettings: cloneMap(params.GeneratorSettings), Notes: params.Notes,
		Metadata: cloneMap(params.Metadata), OperationHistory: []Operation{}, CreatedAt: createdAt,
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

func cloneRuntimeStatusSnapshot(source *RuntimeStatusSnapshot) *RuntimeStatusSnapshot {
	if source == nil {
		return nil
	}
	result := *source
	result.Issues = cloneSlice(source.Issues)
	return &result
}
