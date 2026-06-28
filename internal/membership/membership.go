package membership

import (
	"errors"
	"time"

	"github.com/stratummc/stratum/internal/project"
)

type Membership struct {
	ID        string       `json:"id"`
	UserID    string       `json:"userId"`
	ProjectID string       `json:"projectId"`
	Role      project.Role `json:"role"`
	AddedBy   string       `json:"addedBy"`
	AddedAt   time.Time    `json:"addedAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

func NewMembership(id, userID, projectID string, role project.Role, addedBy string, now time.Time) (Membership, error) {
	if id == "" {
		return Membership{}, errors.New("membership id is required")
	}
	if userID == "" {
		return Membership{}, errors.New("user id is required")
	}
	if projectID == "" {
		return Membership{}, errors.New("project id is required")
	}
	if role == "" {
		return Membership{}, errors.New("role is required")
	}
	if !isValidRole(role) {
		return Membership{}, errors.New("invalid role")
	}
	if addedBy == "" {
		return Membership{}, errors.New("addedBy is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Membership{
		ID:        id,
		UserID:    userID,
		ProjectID: projectID,
		Role:      role,
		AddedBy:   addedBy,
		AddedAt:   now,
		UpdatedAt: now,
	}, nil
}

func (m *Membership) ChangeRole(newRole project.Role, changedBy string, now time.Time) error {
	if !isValidRole(newRole) {
		return errors.New("invalid role")
	}
	if changedBy == "" {
		return errors.New("changedBy is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.Role = newRole
	m.UpdatedAt = now
	return nil
}

func isValidRole(role project.Role) bool {
	return role == project.RoleViewer ||
		role == project.RoleResearcher ||
		role == project.RoleMaintainer ||
		role == project.RoleOwner
}
