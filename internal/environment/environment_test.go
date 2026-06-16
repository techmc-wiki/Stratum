package environment

import (
	"testing"
	"time"
)

func TestEnvironmentValidation(t *testing.T) {
	valid := Environment{
		ID:               "env-1-17-fabric",
		Name:             "1.17 Fabric Carpet",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "16",
		LoaderType:       LoaderFabric,
		ServerCore:       ServerCarpet,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid environment: %v", err)
	}
}

func TestEnvironmentRequiresID(t *testing.T) {
	env := Environment{Name: "Test", MinecraftVersion: "1.17", LoaderType: LoaderFabric, ServerCore: ServerVanilla}
	if err := env.Validate(); err == nil {
		t.Fatal("missing id should fail")
	}
}

func TestEnvironmentRequiresName(t *testing.T) {
	env := Environment{ID: "test-env", MinecraftVersion: "1.17", LoaderType: LoaderFabric, ServerCore: ServerVanilla}
	if err := env.Validate(); err == nil {
		t.Fatal("missing name should fail")
	}
}

func TestEnvironmentRequiresMinecraftVersion(t *testing.T) {
	env := Environment{ID: "test-env", Name: "Test", LoaderType: LoaderFabric, ServerCore: ServerVanilla}
	if err := env.Validate(); err == nil {
		t.Fatal("missing minecraft version should fail")
	}
}

func TestEnvironmentRejectsInvalidLoaderType(t *testing.T) {
	env := Environment{ID: "test-env", Name: "Test", MinecraftVersion: "1.17", LoaderType: "invalid", ServerCore: ServerVanilla}
	if err := env.Validate(); err == nil {
		t.Fatal("invalid loader type should fail")
	}
}

func TestEnvironmentRejectsInvalidServerCore(t *testing.T) {
	env := Environment{ID: "test-env", Name: "Test", MinecraftVersion: "1.17", LoaderType: LoaderFabric, ServerCore: "invalid"}
	if err := env.Validate(); err == nil {
		t.Fatal("invalid server core should fail")
	}
}

func TestEnvironmentRejectsUnsafeID(t *testing.T) {
	env := Environment{ID: "../escape", Name: "Test", MinecraftVersion: "1.17", LoaderType: LoaderFabric, ServerCore: ServerVanilla}
	if err := env.Validate(); err == nil {
		t.Fatal("unsafe id should fail")
	}
}

func TestEnvironmentWithRuntimeProfileID(t *testing.T) {
	env := Environment{
		ID:               "env-test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       LoaderFabric,
		ServerCore:       ServerCarpet,
		RuntimeProfileID: "dummy-process",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("environment with runtime profile id: %v", err)
	}
}

func TestEnvironmentRejectsUnsafeRuntimeProfileID(t *testing.T) {
	env := Environment{
		ID:               "env-test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       LoaderFabric,
		ServerCore:       ServerVanilla,
		RuntimeProfileID: "../escape",
	}
	if err := env.Validate(); err == nil {
		t.Fatal("unsafe runtime profile id should fail")
	}
}

func TestEnvironmentWithoutRuntimeProfileIDStillValidates(t *testing.T) {
	env := Environment{
		ID:               "env-test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       LoaderFabric,
		ServerCore:       ServerVanilla,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("environment without runtime profile id: %v", err)
	}
}
