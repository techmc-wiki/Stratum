package service

import (
	"context"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/membership"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/repository/memory"
	"github.com/stratummc/stratum/internal/session"
)

type mockSessionRepo struct {
	sessions map[string]session.Session
}

func (m *mockSessionRepo) Get(id string) (session.Session, error) {
	s, exists := m.sessions[id]
	if !exists {
		return session.Session{}, nil
	}
	return s, nil
}

func TestCheckProjectAccess_Owner(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{sessions: make(map[string]session.Session)}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_owner", "proj_1", project.RoleOwner, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()
	actions := []string{
		ActionProjectView, ActionProjectModify, ActionProjectDelete,
		ActionProjectManageMembers, ActionSessionCreate, ActionSessionStart,
		ActionArtifactApprove, ActionCheckpointDelete,
	}
	for _, action := range actions {
		err := svc.CheckProjectAccess(ctx, "user_owner", "proj_1", action)
		if err != nil {
			t.Errorf("Owner should have access to %s, got error: %v", action, err)
		}
	}
}

func TestCheckProjectAccess_Viewer(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{sessions: make(map[string]session.Session)}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_viewer", "proj_1", project.RoleViewer, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()

	if err := svc.CheckProjectAccess(ctx, "user_viewer", "proj_1", ActionProjectView); err != nil {
		t.Errorf("Viewer should have access to view, got error: %v", err)
	}

	err := svc.CheckProjectAccess(ctx, "user_viewer", "proj_1", ActionSessionStart)
	if err == nil {
		t.Error("Viewer should not have access to start sessions")
	}
}

func TestCheckProjectAccess_NotMember(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{sessions: make(map[string]session.Session)}
	svc := New(membershipRepo, sessionRepo)

	ctx := context.Background()
	err := svc.CheckProjectAccess(ctx, "non_member", "proj_1", ActionProjectView)
	if err == nil {
		t.Error("Non-member should not have access")
	}
}

func TestCheckProjectAccess_SystemUser(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{sessions: make(map[string]session.Session)}
	svc := New(membershipRepo, sessionRepo)

	ctx := context.Background()
	err := svc.CheckProjectAccess(ctx, SystemUserID, "proj_1", ActionProjectDelete)
	if err != nil {
		t.Errorf("System user should bypass all checks, got error: %v", err)
	}
}

func TestCheckSessionAccess_SharedSession_Maintainer(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{
		sessions: map[string]session.Session{
			"sess_shared": {
				ID:          "sess_shared",
				ProjectID:   "proj_1",
				Type:        session.TypeShared,
				OwnerUserID: "user_admin",
			},
		},
	}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_maintainer", "proj_1", project.RoleMaintainer, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()
	err := svc.CheckSessionAccess(ctx, "user_maintainer", "sess_shared", ActionSessionStart)
	if err != nil {
		t.Errorf("Maintainer should start shared session, got error: %v", err)
	}
}

func TestCheckSessionAccess_SharedSession_Researcher(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{
		sessions: map[string]session.Session{
			"sess_shared": {
				ID:          "sess_shared",
				ProjectID:   "proj_1",
				Type:        session.TypeShared,
				OwnerUserID: "user_admin",
			},
		},
	}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_researcher", "proj_1", project.RoleResearcher, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()
	err := svc.CheckSessionAccess(ctx, "user_researcher", "sess_shared", ActionSessionStart)
	if err == nil {
		t.Error("Researcher should not start shared session")
	}
}

func TestCheckSessionAccess_OwnFork_Researcher(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{
		sessions: map[string]session.Session{
			"sess_fork": {
				ID:          "sess_fork",
				ProjectID:   "proj_1",
				Type:        session.TypeFork,
				OwnerUserID: "user_researcher",
			},
		},
	}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_researcher", "proj_1", project.RoleResearcher, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()
	err := svc.CheckSessionAccess(ctx, "user_researcher", "sess_fork", ActionSessionStart)
	if err != nil {
		t.Errorf("Researcher should start own fork, got error: %v", err)
	}
}

func TestCheckSessionAccess_OthersFork_Researcher(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{
		sessions: map[string]session.Session{
			"sess_fork": {
				ID:          "sess_fork",
				ProjectID:   "proj_1",
				Type:        session.TypeFork,
				OwnerUserID: "user_other",
			},
		},
	}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_researcher", "proj_1", project.RoleResearcher, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()
	err := svc.CheckSessionAccess(ctx, "user_researcher", "sess_fork", ActionSessionStart)
	if err == nil {
		t.Error("Researcher should not start others' fork")
	}
}

func TestRoleHierarchy(t *testing.T) {
	membershipRepo := memory.NewMembershipRepository()
	sessionRepo := &mockSessionRepo{sessions: make(map[string]session.Session)}
	svc := New(membershipRepo, sessionRepo)

	now := time.Now().UTC()
	m, _ := membership.NewMembership("mem_1", "user_maintainer", "proj_1", project.RoleMaintainer, "admin", now)
	membershipRepo.Create(m)

	ctx := context.Background()

	// Maintainer inherits Researcher permissions
	err := svc.CheckProjectAccess(ctx, "user_maintainer", "proj_1", ActionSessionCreate)
	if err != nil {
		t.Errorf("Maintainer should inherit Researcher permissions, got error: %v", err)
	}
}

func TestSharedSessionPermissions(t *testing.T) {
	if err := CanCreateSession(project.RoleResearcher, session.TypeShared); err == nil {
		t.Fatal("researcher should not create shared session")
	}
	if err := CanCreateSession(project.RoleMaintainer, session.TypeShared); err != nil {
		t.Fatal(err)
	}
}

func TestUnapprovedArtifactCannotAttachToSharedSession(t *testing.T) {
	if err := CanAttachArtifact(session.TypeShared, artifact.StatusPending); err == nil {
		t.Fatal("pending artifact should be rejected")
	}
	if err := CanAttachArtifact(session.TypeReview, artifact.StatusPending); err != nil {
		t.Fatal(err)
	}
}
