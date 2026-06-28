package filesystem

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/stratummc/stratum/internal/membership"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func (s *Store) CreateMembership(m membership.Membership) error {
	const operation = "filesystem.CreateMembership"
	if err := validateID(operation, m.ID); err != nil {
		return err
	}
	path := s.entityPath("memberships", m.ID)
	if err := createJSON(path, operation, m); err != nil {
		return err
	}
	if err := s.indexMembership(m); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func (s *Store) GetMembership(id string) (membership.Membership, error) {
	const operation = "filesystem.GetMembership"
	if err := validateID(operation, id); err != nil {
		return membership.Membership{}, err
	}
	path := s.entityPath("memberships", id)
	return readJSON[membership.Membership](path, operation)
}

func (s *Store) GetMembershipByUserAndProject(userID, projectID string) (membership.Membership, error) {
	const operation = "filesystem.GetMembershipByUserAndProject"
	if userID == "" || projectID == "" {
		return membership.Membership{}, validationError(operation, "userID and projectID are required")
	}
	memberships, err := s.ListMembershipsByProject(projectID)
	if err != nil {
		return membership.Membership{}, err
	}
	for _, m := range memberships {
		if m.UserID == userID {
			return m, nil
		}
	}
	return membership.Membership{}, repositoryError(stratumerrors.KindNotFound, operation, "membership not found", nil)
}

func (s *Store) ListMembershipsByProject(projectID string) ([]membership.Membership, error) {
	const operation = "filesystem.ListMembershipsByProject"
	if projectID == "" {
		return nil, validationError(operation, "projectID is required")
	}
	indexDir := filepath.Join(s.Root, "memberships", "by-project", projectID)
	return s.readMembershipIndex(indexDir, operation)
}

func (s *Store) ListMembershipsByUser(userID string) ([]membership.Membership, error) {
	const operation = "filesystem.ListMembershipsByUser"
	if userID == "" {
		return nil, validationError(operation, "userID is required")
	}
	indexDir := filepath.Join(s.Root, "memberships", "by-user", userID)
	return s.readMembershipIndex(indexDir, operation)
}

func (s *Store) UpdateMembership(m membership.Membership) error {
	const operation = "filesystem.UpdateMembership"
	if err := validateID(operation, m.ID); err != nil {
		return err
	}
	path := s.entityPath("memberships", m.ID)
	return updateJSON(path, operation, m)
}

func (s *Store) DeleteMembership(id string) error {
	const operation = "filesystem.DeleteMembership"
	if err := validateID(operation, id); err != nil {
		return err
	}
	m, err := s.GetMembership(id)
	if err != nil {
		return err
	}
	path := s.entityPath("memberships", id)
	if err := os.Remove(path); err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "delete membership", err)
	}
	s.unindexMembership(m)
	return nil
}

func (s *Store) indexMembership(m membership.Membership) error {
	userIndexDir := filepath.Join(s.Root, "memberships", "by-user", m.UserID)
	projectIndexDir := filepath.Join(s.Root, "memberships", "by-project", m.ProjectID)
	for _, dir := range []string{userIndexDir, projectIndexDir} {
		if err := os.MkdirAll(dir, directoryPermissions); err != nil {
			return err
		}
		linkPath := filepath.Join(dir, m.ID+".json")
		targetPath := filepath.Join("..", "..", m.ID+".json")
		if err := writeJSONAtomic(linkPath, "indexMembership", map[string]string{"ref": targetPath}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) unindexMembership(m membership.Membership) {
	userLink := filepath.Join(s.Root, "memberships", "by-user", m.UserID, m.ID+".json")
	projectLink := filepath.Join(s.Root, "memberships", "by-project", m.ProjectID, m.ID+".json")
	os.Remove(userLink)
	os.Remove(projectLink)
}

func (s *Store) readMembershipIndex(indexDir, operation string) ([]membership.Membership, error) {
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []membership.Membership{}, nil
		}
		return nil, repositoryError(stratumerrors.KindConflict, operation, "read index", err)
	}
	var memberships []membership.Membership
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5]
		m, err := s.GetMembership(id)
		if err != nil {
			continue
		}
		memberships = append(memberships, m)
	}
	return memberships, nil
}
