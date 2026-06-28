package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
	"github.com/stratummc/stratum/internal/user"
)

func (s *Store) CreateUser(u user.User) error {
	const operation = "filesystem.CreateUser"
	if err := validateID(operation, u.ID); err != nil {
		return err
	}
	path := s.entityPath("users", u.ID)
	return createJSON(path, operation, u)
}

func (s *Store) GetUser(id string) (user.User, error) {
	const operation = "filesystem.GetUser"
	if err := validateID(operation, id); err != nil {
		return user.User{}, err
	}
	path := s.entityPath("users", id)
	return readJSON[user.User](path, operation)
}

func (s *Store) GetUserByUsername(username string) (user.User, error) {
	const operation = "filesystem.GetUserByUsername"
	if username == "" {
		return user.User{}, validationError(operation, "username is required")
	}
	users, err := s.ListUsers()
	if err != nil {
		return user.User{}, err
	}
	for _, u := range users {
		if u.Username == username {
			return u, nil
		}
	}
	return user.User{}, repositoryError(stratumerrors.KindNotFound, operation, "user not found", nil)
}

func (s *Store) UpdateUser(u user.User) error {
	const operation = "filesystem.UpdateUser"
	if err := validateID(operation, u.ID); err != nil {
		return err
	}
	path := s.entityPath("users", u.ID)
	return updateJSON(path, operation, u)
}

func (s *Store) ListUsers() ([]user.User, error) {
	const operation = "filesystem.ListUsers"
	directory := filepath.Join(s.Root, "users")
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []user.User{}, nil
		}
		return nil, repositoryError(stratumerrors.KindConflict, operation, "read users directory", err)
	}
	var users []user.User
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		u, err := readJSON[user.User](path, operation)
		if err != nil {
			return nil, fmt.Errorf("%s: read user %s: %w", operation, entry.Name(), err)
		}
		users = append(users, u)
	}
	return users, nil
}
