package service

import (
	"testing"

	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/session"
)

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
