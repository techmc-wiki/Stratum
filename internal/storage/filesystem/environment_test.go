package filesystem

import (
	"context"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/environment"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func TestEnvironmentRepository(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	env := environment.Environment{
		ID:               "env-1-17-fabric",
		Name:             "1.17 Fabric Carpet",
		MinecraftVersion: "1.17.1",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerCarpet,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != env.ID || got.Name != env.Name {
		t.Fatalf("got=%+v", got)
	}
	list, err := store.ListEnvironments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != env.ID {
		t.Fatalf("list=%+v", list)
	}
}

func TestEnvironmentDuplicateIDFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	env := environment.Environment{
		ID:               "test-env",
		Name:             "Test",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
	}
	_ = store.CreateEnvironment(ctx, env)
	if err := store.CreateEnvironment(ctx, env); err == nil {
		t.Fatal("duplicate environment id should fail")
	}
}

func TestEnvironmentPersistsAfterReload(t *testing.T) {
	root := t.TempDir()
	store1, _ := New(root)
	ctx := context.Background()
	env := environment.Environment{
		ID:               "env-persist",
		Name:             "Persist Test",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
	}
	if err := store1.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	store2, _ := New(root)
	got, err := store2.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != env.ID {
		t.Fatalf("got=%+v", got)
	}
}

func TestUpdateEnvironmentWithMatchingTimestamp(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	now := time.Now().UTC()
	env := environment.Environment{
		ID:               "env-update",
		Name:             "Original",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	env.Name = "Updated"
	env.UpdatedAt = time.Now().UTC()
	if err := store.UpdateEnvironment(ctx, env, now); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, err := store.GetEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Updated" {
		t.Errorf("name = %q, want Updated", got.Name)
	}
	if got.CreatedAt != now {
		t.Errorf("created_at changed")
	}
	if got.UpdatedAt == now {
		t.Errorf("updated_at not changed")
	}
}

func TestUpdateEnvironmentWithStaleTimestampFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	now := time.Now().UTC()
	env := environment.Environment{
		ID:               "env-conflict",
		Name:             "Original",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-1 * time.Hour)
	env.Name = "Updated"
	err := store.UpdateEnvironment(ctx, env, stale)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindConflict) {
		t.Errorf("error kind = %v, want conflict", err)
	}
	got, _ := store.GetEnvironment(ctx, env.ID)
	if got.Name != "Original" {
		t.Errorf("name should not be updated on conflict")
	}
}

func TestUpdateEnvironmentWithInvalidLoaderFails(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	now := time.Now().UTC()
	env := environment.Environment{
		ID:               "env-invalid",
		Name:             "Test",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		t.Fatal(err)
	}
	env.LoaderType = environment.LoaderType("invalid-loader")
	err := store.UpdateEnvironment(ctx, env, now)
	if err == nil {
		t.Fatal("expected validation error")
	}
	t.Logf("got error: %v", err)
	got, _ := store.GetEnvironment(ctx, env.ID)
	if got.LoaderType != environment.LoaderFabric {
		t.Errorf("loader should not be updated on validation failure")
	}
}

func TestUpdateEnvironmentNotFound(t *testing.T) {
	store, _ := New(t.TempDir())
	ctx := context.Background()
	env := environment.Environment{
		ID:               "nonexistent",
		Name:             "Test",
		MinecraftVersion: "1.17",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerVanilla,
	}
	err := store.UpdateEnvironment(ctx, env, time.Now().UTC())
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		t.Errorf("error kind = %v, want not found", err)
	}
}
