package room

import (
	"time"

	"github.com/stratummc/stratum/internal/worldprofile"
)

type Room struct {
	ID                  string                     `json:"id"`
	ProjectID           string                     `json:"projectId"`
	Name                string                     `json:"name"`
	EnvironmentID       string                     `json:"environmentId"`
	BaseWorldRef        string                     `json:"baseWorldRef"`
	DefaultWorldProfile *worldprofile.WorldProfile `json:"defaultWorldProfile,omitempty"`
	SharedSessionID     string                     `json:"sharedSessionId,omitempty"`
	CreatedAt           time.Time                  `json:"createdAt"`
}
