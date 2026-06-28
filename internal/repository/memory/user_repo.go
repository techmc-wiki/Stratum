package memory

import (
	"errors"
	"sync"

	"github.com/stratummc/stratum/internal/user"
)

type UserRepository struct {
	mu     sync.RWMutex
	users  map[string]user.User
	byName map[string]string
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		users:  make(map[string]user.User),
		byName: make(map[string]string),
	}
}

func (r *UserRepository) Create(u user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[u.ID]; exists {
		return errors.New("user already exists")
	}
	if _, exists := r.byName[u.Username]; exists {
		return errors.New("username already taken")
	}
	r.users[u.ID] = u
	r.byName[u.Username] = u.ID
	return nil
}

func (r *UserRepository) GetByID(id string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, exists := r.users[id]
	if !exists {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

func (r *UserRepository) GetByUsername(username string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, exists := r.byName[username]
	if !exists {
		return user.User{}, errors.New("user not found")
	}
	return r.users[id], nil
}

func (r *UserRepository) Update(u user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, exists := r.users[u.ID]
	if !exists {
		return errors.New("user not found")
	}
	if existing.Username != u.Username {
		if _, taken := r.byName[u.Username]; taken {
			return errors.New("username already taken")
		}
		delete(r.byName, existing.Username)
		r.byName[u.Username] = u.ID
	}
	r.users[u.ID] = u
	return nil
}

func (r *UserRepository) List() ([]user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := make([]user.User, 0, len(r.users))
	for _, u := range r.users {
		users = append(users, u)
	}
	return users, nil
}
