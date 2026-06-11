package room

import "time"

type Room struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	Name            string    `json:"name"`
	EnvironmentID   string    `json:"environmentId"`
	BaseWorldRef    string    `json:"baseWorldRef"`
	SharedSessionID string    `json:"sharedSessionId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}
