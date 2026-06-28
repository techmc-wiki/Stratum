package service

import (
	"context"
	"fmt"

	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/membership"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/session"
)

const SystemUserID = "system"

const (
	ActionProjectView          = "project:view"
	ActionProjectModify        = "project:modify"
	ActionProjectDelete        = "project:delete"
	ActionProjectManageMembers = "project:manage_members"

	ActionSessionView    = "session:view"
	ActionSessionCreate  = "session:create"
	ActionSessionStart   = "session:start"
	ActionSessionStop    = "session:stop"
	ActionSessionRestart = "session:restart"
	ActionSessionDelete  = "session:delete"
	ActionSessionCommand = "session:command"

	ActionCheckpointCreate  = "checkpoint:create"
	ActionCheckpointRestore = "checkpoint:restore"
	ActionCheckpointDelete  = "checkpoint:delete"

	ActionArtifactUpload  = "artifact:upload"
	ActionArtifactApprove = "artifact:approve"
	ActionArtifactReject  = "artifact:reject"
	ActionArtifactApply   = "artifact:apply"
	ActionArtifactDelete  = "artifact:delete"

	ActionRoomView   = "room:view"
	ActionRoomModify = "room:modify"
	ActionRoomDelete = "room:delete"

	ActionEnvironmentView   = "environment:view"
	ActionEnvironmentModify = "environment:modify"
	ActionEnvironmentDelete = "environment:delete"
)

type Service struct {
	membershipRepo membership.Repository
	sessionRepo    SessionRepository
}

type SessionRepository interface {
	Get(id string) (session.Session, error)
}

func New(membershipRepo membership.Repository, sessionRepo SessionRepository) *Service {
	return &Service{
		membershipRepo: membershipRepo,
		sessionRepo:    sessionRepo,
	}
}

func (s *Service) CheckProjectAccess(ctx context.Context, userID, projectID, action string) error {
	if userID == SystemUserID {
		return nil
	}
	m, err := s.membershipRepo.GetByUserAndProject(userID, projectID)
	if err != nil {
		return fmt.Errorf("access denied: not a project member")
	}

	switch action {
	case ActionProjectView, ActionSessionView, ActionRoomView, ActionEnvironmentView,
		ActionCheckpointCreate, ActionArtifactUpload:
		return s.requireRole(m.Role, project.RoleViewer)
	case ActionSessionCreate, ActionArtifactApply:
		return s.requireRole(m.Role, project.RoleResearcher)
	case ActionSessionStart, ActionSessionStop, ActionSessionRestart, ActionSessionCommand,
		ActionCheckpointRestore, ActionArtifactApprove, ActionArtifactReject,
		ActionRoomModify, ActionEnvironmentModify:
		return s.requireRole(m.Role, project.RoleMaintainer)
	case ActionProjectModify, ActionProjectDelete, ActionProjectManageMembers,
		ActionSessionDelete, ActionCheckpointDelete, ActionArtifactDelete,
		ActionRoomDelete, ActionEnvironmentDelete:
		return s.requireRole(m.Role, project.RoleOwner)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func (s *Service) CheckSessionAccess(ctx context.Context, userID, sessionID, action string) error {
	if userID == SystemUserID {
		return nil
	}
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found")
	}

	m, err := s.membershipRepo.GetByUserAndProject(userID, sess.ProjectID)
	if err != nil {
		return fmt.Errorf("access denied: not a project member")
	}

	switch action {
	case ActionSessionView:
		return s.requireRole(m.Role, project.RoleViewer)
	case ActionSessionCreate:
		if sess.Type == session.TypeShared {
			return s.requireRole(m.Role, project.RoleMaintainer)
		}
		return s.requireRole(m.Role, project.RoleResearcher)
	case ActionSessionStart, ActionSessionStop, ActionSessionRestart, ActionSessionCommand:
		if sess.Type == session.TypeShared {
			return s.requireRole(m.Role, project.RoleMaintainer)
		}
		if sess.OwnerUserID == userID {
			return s.requireRole(m.Role, project.RoleResearcher)
		}
		return s.requireRole(m.Role, project.RoleMaintainer)
	case ActionSessionDelete:
		if sess.Type == session.TypeShared {
			return s.requireRole(m.Role, project.RoleOwner)
		}
		if sess.OwnerUserID == userID {
			return s.requireRole(m.Role, project.RoleResearcher)
		}
		return s.requireRole(m.Role, project.RoleMaintainer)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

func (s *Service) CheckRoomAccess(ctx context.Context, userID, projectID, action string) error {
	return s.CheckProjectAccess(ctx, userID, projectID, action)
}

func (s *Service) CheckArtifactAccess(ctx context.Context, userID, projectID, action string) error {
	return s.CheckProjectAccess(ctx, userID, projectID, action)
}

func (s *Service) CheckCheckpointAccess(ctx context.Context, userID, projectID, action string) error {
	return s.CheckProjectAccess(ctx, userID, projectID, action)
}

func (s *Service) CheckEnvironmentAccess(ctx context.Context, userID, projectID, action string) error {
	return s.CheckProjectAccess(ctx, userID, projectID, action)
}

func (s *Service) requireRole(userRole, minRole project.Role) error {
	if !s.hasRole(userRole, minRole) {
		return fmt.Errorf("requires %s role or higher", minRole)
	}
	return nil
}

func (s *Service) hasRole(userRole, minRole project.Role) bool {
	roleLevel := map[project.Role]int{
		project.RoleViewer:     1,
		project.RoleResearcher: 2,
		project.RoleMaintainer: 3,
		project.RoleOwner:      4,
	}
	return roleLevel[userRole] >= roleLevel[minRole]
}

// Legacy functions for backward compatibility
func CanCreateSession(role project.Role, sessionType session.Type) error {
	if role == project.RoleViewer {
		return fmt.Errorf("role %q cannot create sessions", role)
	}
	if sessionType == session.TypeShared && role != project.RoleMaintainer && role != project.RoleOwner {
		return fmt.Errorf("shared sessions require maintainer or owner role")
	}
	return nil
}

func CanAttachArtifact(sessionType session.Type, status artifact.Status) error {
	if sessionType == session.TypeShared && status != artifact.StatusApproved {
		return fmt.Errorf("artifact status %q cannot be attached to a shared session", status)
	}
	return nil
}
