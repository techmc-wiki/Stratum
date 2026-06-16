package service

import (
	"fmt"

	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/session"
)

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
