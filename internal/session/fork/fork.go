package fork

import "time"

type SourceType string

const (
	SourceTypeRoom       SourceType = "room"
	SourceTypeSession    SourceType = "session"
	SourceTypeCheckpoint SourceType = "checkpoint"
)

type ForkOptions struct {
	ID                    string
	ProjectID             string
	RoomID                string
	SourceType            SourceType
	SourceID              string
	SourceCheckpointID    string
	CreatorID             string
	Reason                string
	EnvironmentID         string
	RuntimeProfileID      string
	InheritedArtifactIDs  []string
	InheritedServerConfig map[string]string
	TTL                   *time.Duration
	ActorID               string
}
