package staging

import "time"

type (
	Kind   string
	Status string
)

const (
	KindArtifact Kind = "artifact"
	KindConfig   Kind = "config"

	StatusPlanned     Status = "planned"
	StatusRejected    Status = "rejected"
	StatusAppliedStub Status = "applied_stub"
	StatusFailed      Status = "failed"
)

type Plan struct {
	ID                string            `json:"id"`
	SessionID         string            `json:"sessionId"`
	ProjectID         string            `json:"projectId"`
	RoomID            string            `json:"roomId,omitempty"`
	ArtifactID        string            `json:"artifactId"`
	ArtifactName      string            `json:"artifactName"`
	ArtifactType      string            `json:"artifactType"`
	ArtifactStatus    string            `json:"artifactStatus"`
	ArtifactHash      string            `json:"artifactHash"`
	TargetStagingName string            `json:"targetStagingName"`
	StagingKind       Kind              `json:"stagingKind"`
	ActorID           string            `json:"actorId"`
	CreatedAt         time.Time         `json:"createdAt"`
	Status            Status            `json:"status"`
	RejectionReason   string            `json:"rejectionReason,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}
