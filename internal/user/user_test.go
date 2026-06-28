package user

import (
	"testing"
	"time"
)

func TestNewUser_Success(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	u, err := NewUser("user_123", "alice", "Alice Smith", "alice@example.com", "hashed_password", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != "user_123" {
		t.Errorf("expected ID user_123, got %s", u.ID)
	}
	if u.Username != "alice" {
		t.Errorf("expected username alice, got %s", u.Username)
	}
	if u.Status != StatusActive {
		t.Errorf("expected status active, got %s", u.Status)
	}
	if !u.CreatedAt.Equal(now) {
		t.Errorf("expected createdAt %v, got %v", now, u.CreatedAt)
	}
}

func TestNewUser_InvalidUsername(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name     string
		username string
	}{
		{"empty", ""},
		{"too short", "ab"},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567"},
		{"special chars", "alice@domain"},
		{"spaces", "alice smith"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUser("user_123", tt.username, "Alice", "", "hash", now)
			if err == nil {
				t.Errorf("expected error for username %q", tt.username)
			}
		})
	}
}

func TestNewUser_MissingFields(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name         string
		id           string
		username     string
		displayName  string
		passwordHash string
		expectErr    string
	}{
		{"no id", "", "alice", "Alice", "hash", "id is required"},
		{"no username", "user_123", "", "Alice", "hash", "username is required"},
		{"no display name", "user_123", "alice", "", "hash", "display name is required"},
		{"no password", "user_123", "alice", "Alice", "", "password hash is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUser(tt.id, tt.username, tt.displayName, "", tt.passwordHash, now)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.expectErr)
			}
		})
	}
}

func TestUser_Suspend(t *testing.T) {
	now := time.Now().UTC()
	u, _ := NewUser("user_123", "alice", "Alice", "", "hash", now)
	suspendTime := now.Add(time.Hour)
	u.Suspend(suspendTime)
	if u.Status != StatusSuspended {
		t.Errorf("expected suspended, got %s", u.Status)
	}
	if !u.UpdatedAt.Equal(suspendTime) {
		t.Errorf("expected updatedAt %v, got %v", suspendTime, u.UpdatedAt)
	}
}

func TestUser_Activate(t *testing.T) {
	now := time.Now().UTC()
	u, _ := NewUser("user_123", "alice", "Alice", "", "hash", now)
	u.Suspend(now)
	activateTime := now.Add(time.Hour)
	u.Activate(activateTime)
	if u.Status != StatusActive {
		t.Errorf("expected active, got %s", u.Status)
	}
	if !u.UpdatedAt.Equal(activateTime) {
		t.Errorf("expected updatedAt %v, got %v", activateTime, u.UpdatedAt)
	}
}

func TestNewUser_ZeroTime(t *testing.T) {
	u, err := NewUser("user_123", "alice", "Alice", "", "hash", time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.CreatedAt.IsZero() {
		t.Error("expected non-zero createdAt when passing zero time")
	}
}

func TestUser_SuspendZeroTime(t *testing.T) {
	u, _ := NewUser("user_123", "alice", "Alice", "", "hash", time.Now().UTC())
	before := time.Now().UTC()
	u.Suspend(time.Time{})
	if u.UpdatedAt.Before(before) {
		t.Error("expected updatedAt to be set when passing zero time")
	}
}
