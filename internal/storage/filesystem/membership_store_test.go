package filesystem

import (
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/membership"
	"github.com/stratummc/stratum/internal/project"
)

func TestMembershipStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)

	if err := store.CreateMembership(m); err != nil {
		t.Fatalf("CreateMembership failed: %v", err)
	}

	got, err := store.GetMembership("mem_123")
	if err != nil {
		t.Fatalf("GetMembership failed: %v", err)
	}

	if got.ID != m.ID || got.UserID != m.UserID {
		t.Errorf("got %+v, want %+v", got, m)
	}
}

func TestMembershipStore_ListByProject(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	m1, _ := membership.NewMembership("mem_1", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	m2, _ := membership.NewMembership("mem_2", "user_2", "proj_1", project.RoleMaintainer, "admin", now)
	m3, _ := membership.NewMembership("mem_3", "user_3", "proj_2", project.RoleOwner, "admin", now)

	store.CreateMembership(m1)
	store.CreateMembership(m2)
	store.CreateMembership(m3)

	memberships, err := store.ListMembershipsByProject("proj_1")
	if err != nil {
		t.Fatalf("ListMembershipsByProject failed: %v", err)
	}
	if len(memberships) != 2 {
		t.Errorf("got %d memberships, want 2", len(memberships))
	}
}

func TestMembershipStore_ListByUser(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	m1, _ := membership.NewMembership("mem_1", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	m2, _ := membership.NewMembership("mem_2", "user_1", "proj_2", project.RoleMaintainer, "admin", now)

	store.CreateMembership(m1)
	store.CreateMembership(m2)

	memberships, err := store.ListMembershipsByUser("user_1")
	if err != nil {
		t.Fatalf("ListMembershipsByUser failed: %v", err)
	}
	if len(memberships) != 2 {
		t.Errorf("got %d memberships, want 2", len(memberships))
	}
}

func TestMembershipStore_GetByUserAndProject(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	m, _ := membership.NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	store.CreateMembership(m)

	got, err := store.GetMembershipByUserAndProject("user_1", "proj_1")
	if err != nil {
		t.Fatalf("GetMembershipByUserAndProject failed: %v", err)
	}
	if got.ID != "mem_123" {
		t.Errorf("got ID %s, want mem_123", got.ID)
	}
}

func TestMembershipStore_Update(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	m, _ := membership.NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	store.CreateMembership(m)

	m.ChangeRole(project.RoleMaintainer, "admin", now.Add(time.Hour))
	if err := store.UpdateMembership(m); err != nil {
		t.Fatalf("UpdateMembership failed: %v", err)
	}

	got, _ := store.GetMembership("mem_123")
	if got.Role != project.RoleMaintainer {
		t.Errorf("got role %s, want maintainer", got.Role)
	}
}

func TestMembershipStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	m, _ := membership.NewMembership("mem_123", "user_1", "proj_1", project.RoleResearcher, "admin", now)
	store.CreateMembership(m)

	if err := store.DeleteMembership("mem_123"); err != nil {
		t.Fatalf("DeleteMembership failed: %v", err)
	}

	_, err := store.GetMembership("mem_123")
	if err == nil {
		t.Error("expected error getting deleted membership")
	}
}
