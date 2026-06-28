package membership

import (
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/project"
)

func TestNewMembership_Success(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	m, err := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "user_admin", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "mem_123" {
		t.Errorf("expected ID mem_123, got %s", m.ID)
	}
	if m.Role != project.RoleResearcher {
		t.Errorf("expected role researcher, got %s", m.Role)
	}
	if !m.AddedAt.Equal(now) {
		t.Errorf("expected addedAt %v, got %v", now, m.AddedAt)
	}
}

func TestNewMembership_MissingFields(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name      string
		id        string
		userID    string
		projectID string
		role      project.Role
		addedBy   string
	}{
		{"no id", "", "user_1", "proj_1", project.RoleResearcher, "admin"},
		{"no userID", "mem_123", "", "proj_1", project.RoleResearcher, "admin"},
		{"no projectID", "mem_123", "user_1", "", project.RoleResearcher, "admin"},
		{"no role", "mem_123", "user_1", "proj_1", "", "admin"},
		{"no addedBy", "mem_123", "user_1", "proj_1", project.RoleResearcher, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMembership(tt.id, tt.userID, tt.projectID, tt.role, tt.addedBy, now)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestNewMembership_InvalidRole(t *testing.T) {
	now := time.Now().UTC()
	_, err := NewMembership("mem_123", "user_1", "proj_1", "invalid", "admin", now)
	if err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestMembership_ChangeRole(t *testing.T) {
	now := time.Now().UTC()
	m, _ := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	changeTime := now.Add(time.Hour)
	err := m.ChangeRole(project.RoleMaintainer, "admin", changeTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Role != project.RoleMaintainer {
		t.Errorf("expected role maintainer, got %s", m.Role)
	}
	if !m.UpdatedAt.Equal(changeTime) {
		t.Errorf("expected updatedAt %v, got %v", changeTime, m.UpdatedAt)
	}
}

func TestMembership_ChangeRole_Invalid(t *testing.T) {
	now := time.Now().UTC()
	m, _ := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	err := m.ChangeRole("invalid", "admin", now)
	if err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestMembership_ChangeRole_NoChangedBy(t *testing.T) {
	now := time.Now().UTC()
	m, _ := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	err := m.ChangeRole(project.RoleMaintainer, "", now)
	if err == nil {
		t.Error("expected error for missing changedBy")
	}
}

func TestNewMembership_ZeroTime(t *testing.T) {
	m, err := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.AddedAt.IsZero() {
		t.Error("expected non-zero addedAt when passing zero time")
	}
}

func TestMembership_ChangeRole_ZeroTime(t *testing.T) {
	m, _ := NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", time.Now().UTC())
	before := time.Now().UTC()
	m.ChangeRole(project.RoleMaintainer, "admin", time.Time{})
	if m.UpdatedAt.Before(before) {
		t.Error("expected updatedAt to be set when passing zero time")
	}
}
