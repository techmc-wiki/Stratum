package project

import "time"

type Role string

const (
	RoleViewer     Role = "viewer"
	RoleResearcher Role = "researcher"
	RoleMaintainer Role = "maintainer"
	RoleOwner      Role = "owner"
)

type Member struct {
	UserID string `json:"userId"`
	Role   Role   `json:"role"`
}

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Members     []Member  `json:"members"`
	CreatedAt   time.Time `json:"createdAt"`
}
