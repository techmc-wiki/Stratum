package user

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	Email        string    `json:"email,omitempty"`
	PasswordHash string    `json:"passwordHash"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func NewUser(id, username, displayName, email, passwordHash string, now time.Time) (User, error) {
	if id == "" {
		return User{}, errors.New("user id is required")
	}
	if username == "" {
		return User{}, errors.New("username is required")
	}
	if !validUsername.MatchString(username) {
		return User{}, fmt.Errorf("username must be 3-32 characters, alphanumeric, underscore, or hyphen")
	}
	if displayName == "" {
		return User{}, errors.New("display name is required")
	}
	if passwordHash == "" {
		return User{}, errors.New("password hash is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return User{
		ID:           id,
		Username:     username,
		DisplayName:  displayName,
		Email:        email,
		PasswordHash: passwordHash,
		Status:       StatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (u *User) Suspend(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	u.Status = StatusSuspended
	u.UpdatedAt = now
}

func (u *User) Activate(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	u.Status = StatusActive
	u.UpdatedAt = now
}
