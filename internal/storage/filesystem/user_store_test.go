package filesystem

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/user"
)

func TestUserStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	now := time.Now().UTC()
	u, _ := user.NewUser("user_123", "alice", "Alice Smith", "alice@example.com", "hashed", now)

	if err := store.CreateUser(u); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	got, err := store.GetUser("user_123")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if got.ID != u.ID || got.Username != u.Username {
		t.Errorf("got %+v, want %+v", got, u)
	}
}

func TestUserStore_CreateDuplicate(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()
	u, _ := user.NewUser("user_123", "alice", "Alice", "", "hash", now)

	store.CreateUser(u)
	err := store.CreateUser(u)
	if err == nil {
		t.Error("expected error creating duplicate user")
	}
}

func TestUserStore_GetByUsername(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()
	u, _ := user.NewUser("user_123", "alice", "Alice", "", "hash", now)
	store.CreateUser(u)

	got, err := store.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if got.ID != "user_123" {
		t.Errorf("got ID %s, want user_123", got.ID)
	}
}

func TestUserStore_GetByUsername_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)

	_, err := store.GetUserByUsername("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent username")
	}
}

func TestUserStore_Update(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()
	u, _ := user.NewUser("user_123", "alice", "Alice", "", "hash", now)
	store.CreateUser(u)

	u.DisplayName = "Alice Updated"
	u.UpdatedAt = now.Add(time.Hour)
	if err := store.UpdateUser(u); err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	got, _ := store.GetUser("user_123")
	if got.DisplayName != "Alice Updated" {
		t.Errorf("got displayName %s, want Alice Updated", got.DisplayName)
	}
}

func TestUserStore_List(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	u1, _ := user.NewUser("user_1", "alice", "Alice", "", "hash", now)
	u2, _ := user.NewUser("user_2", "bob", "Bob", "", "hash", now)
	store.CreateUser(u1)
	store.CreateUser(u2)

	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("got %d users, want 2", len(users))
	}
}

func TestUserStore_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()

	u, _ := user.NewUser("user_123", "alice", "Alice", "", "hash", now)
	store.CreateUser(u)

	done := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func(n int) {
			u.DisplayName = "Concurrent " + string(rune('0'+n))
			store.UpdateUser(u)
			done <- true
		}(i)
	}
	<-done
	<-done

	got, _ := store.GetUser("user_123")
	if got.DisplayName == "" {
		t.Error("expected non-empty displayName after concurrent writes")
	}
}

func TestUserStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(dir)
	now := time.Now().UTC()
	u, _ := user.NewUser("user_123", "alice", "Alice", "", "hash", now)
	store.CreateUser(u)

	tempFiles, _ := filepath.Glob(filepath.Join(dir, "users", ".stratum-*.tmp"))
	if len(tempFiles) > 0 {
		t.Errorf("found %d temp files after create, want 0", len(tempFiles))
	}
}
