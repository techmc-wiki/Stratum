package filesystem

import (
	"context"
	"testing"

	"github.com/stratummc/stratum/internal/domain/environment"
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
