package apply

import "time"

type (
	Kind       string
	TargetRoot string
	Status     string
)

const (
	KindMod          Kind = "mod"
	KindConfig       Kind = "config"
	KindDatapack     Kind = "datapack"
	KindMCDRPlugin   Kind = "mcdr_plugin"
	KindSchematic    Kind = "schematic"
	KindWorldArchive Kind = "world_archive"
	KindOther        Kind = "other"
)

const (
	TargetRootMods        TargetRoot = "mods"
	TargetRootConfig      TargetRoot = "config"
	TargetRootDatapacks   TargetRoot = "datapacks"
	TargetRootPlugins     TargetRoot = "plugins"
	TargetRootMCDRPlugins TargetRoot = "mcdr_plugins"
	TargetRootSchematics  TargetRoot = "schematics"
	TargetRootWorlds      TargetRoot = "worlds"
	TargetRootCustom      TargetRoot = "custom"
)

const (
	StatusPlanned    Status = "planned"
	StatusRejected   Status = "rejected"
	StatusDeprecated Status = "deprecated"
)

type Plan struct {
	ID                       string            `json:"id"`
	SessionID                string            `json:"sessionId"`
	ProjectID                string            `json:"projectId"`
	ActorID                  string            `json:"actorId"`
	SourceStagingPlanID      string            `json:"sourceStagingPlanId"`
	ArtifactID               string            `json:"artifactId"`
	MaterializedArtifactHash string            `json:"materializedArtifactHash"`
	MaterializedArtifactName string            `json:"materializedArtifactName"`
	ApplyKind                Kind              `json:"applyKind"`
	TargetRoot               TargetRoot        `json:"targetRoot"`
	TargetRelativePath       string            `json:"targetRelativePath"`
	Status                   Status            `json:"status"`
	ValidationStatus         string            `json:"validationStatus,omitempty"`
	RejectionReason          string            `json:"rejectionReason,omitempty"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	CreatedAt                time.Time         `json:"createdAt"`
}
