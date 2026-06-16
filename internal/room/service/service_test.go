package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/room"
)

type fakeRepo struct {
	rooms        map[string]room.Room
	environments map[string]environment.Environment
}

func (f *fakeRepo) CreateRoom(ctx context.Context, rm room.Room) error {
	f.rooms[rm.ID] = rm
	return nil
}

func (f *fakeRepo) ListRooms(ctx context.Context) ([]room.Room, error) {
	result := []room.Room{}
	for _, r := range f.rooms {
		result = append(result, r)
	}
	return result, nil
}

func (f *fakeRepo) GetEnvironment(ctx context.Context, id string) (environment.Environment, error) {
	env, ok := f.environments[id]
	if !ok {
		return environment.Environment{}, errors.New("not found")
	}
	return env, nil
}

func TestCreateRoomWithExistingEnvironment(t *testing.T) {
	repo := &fakeRepo{
		rooms: map[string]room.Room{},
		environments: map[string]environment.Environment{
			"test-env": {ID: "test-env", Name: "Test Environment"},
		},
	}
	svc := New(repo, repo)
	rm := room.Room{
		ID:            "room-1",
		ProjectID:     "proj-1",
		Name:          "Test Room",
		EnvironmentID: "test-env",
		CreatedAt:     time.Now().UTC(),
	}
	if err := svc.CreateRoom(context.Background(), rm, "test"); err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if _, ok := repo.rooms["room-1"]; !ok {
		t.Error("room was not persisted")
	}
}

func TestCreateRoomWithMissingEnvironment(t *testing.T) {
	repo := &fakeRepo{
		rooms:        map[string]room.Room{},
		environments: map[string]environment.Environment{},
	}
	svc := New(repo, repo)
	rm := room.Room{
		ID:            "room-1",
		ProjectID:     "proj-1",
		Name:          "Test Room",
		EnvironmentID: "nonexistent",
		CreatedAt:     time.Now().UTC(),
	}
	err := svc.CreateRoom(context.Background(), rm, "test")
	if err == nil {
		t.Fatal("expected error when environment does not exist")
	}
	if _, ok := repo.rooms["room-1"]; ok {
		t.Error("room should not be persisted on validation failure")
	}
}

func TestCreateRoomWithEmptyEnvironment(t *testing.T) {
	repo := &fakeRepo{
		rooms:        map[string]room.Room{},
		environments: map[string]environment.Environment{},
	}
	svc := New(repo, repo)
	rm := room.Room{
		ID:            "room-1",
		ProjectID:     "proj-1",
		Name:          "Test Room",
		EnvironmentID: "",
		CreatedAt:     time.Now().UTC(),
	}
	if err := svc.CreateRoom(context.Background(), rm, "test"); err != nil {
		t.Fatalf("CreateRoom with empty environment should succeed: %v", err)
	}
	if _, ok := repo.rooms["room-1"]; !ok {
		t.Error("room was not persisted")
	}
}
