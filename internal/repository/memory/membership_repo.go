package memory

import (
	"errors"
	"sync"

	"github.com/stratummc/stratum/internal/membership"
)

type MembershipRepository struct {
	mu          sync.RWMutex
	memberships map[string]membership.Membership
	byUser      map[string][]string
	byProject   map[string][]string
}

func NewMembershipRepository() *MembershipRepository {
	return &MembershipRepository{
		memberships: make(map[string]membership.Membership),
		byUser:      make(map[string][]string),
		byProject:   make(map[string][]string),
	}
}

func (r *MembershipRepository) Create(m membership.Membership) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.memberships[m.ID]; exists {
		return errors.New("membership already exists")
	}
	r.memberships[m.ID] = m
	r.byUser[m.UserID] = append(r.byUser[m.UserID], m.ID)
	r.byProject[m.ProjectID] = append(r.byProject[m.ProjectID], m.ID)
	return nil
}

func (r *MembershipRepository) GetByID(id string) (membership.Membership, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, exists := r.memberships[id]
	if !exists {
		return membership.Membership{}, errors.New("membership not found")
	}
	return m, nil
}

func (r *MembershipRepository) GetByUserAndProject(userID, projectID string) (membership.Membership, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.memberships {
		if m.UserID == userID && m.ProjectID == projectID {
			return m, nil
		}
	}
	return membership.Membership{}, errors.New("membership not found")
}

func (r *MembershipRepository) ListByProject(projectID string) ([]membership.Membership, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byProject[projectID]
	memberships := make([]membership.Membership, 0, len(ids))
	for _, id := range ids {
		memberships = append(memberships, r.memberships[id])
	}
	return memberships, nil
}

func (r *MembershipRepository) ListByUser(userID string) ([]membership.Membership, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byUser[userID]
	memberships := make([]membership.Membership, 0, len(ids))
	for _, id := range ids {
		memberships = append(memberships, r.memberships[id])
	}
	return memberships, nil
}

func (r *MembershipRepository) Update(m membership.Membership) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.memberships[m.ID]; !exists {
		return errors.New("membership not found")
	}
	r.memberships[m.ID] = m
	return nil
}

func (r *MembershipRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, exists := r.memberships[id]
	if !exists {
		return errors.New("membership not found")
	}
	delete(r.memberships, id)
	r.byUser[m.UserID] = removeID(r.byUser[m.UserID], id)
	r.byProject[m.ProjectID] = removeID(r.byProject[m.ProjectID], id)
	return nil
}

func removeID(ids []string, id string) []string {
	for i, v := range ids {
		if v == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}
