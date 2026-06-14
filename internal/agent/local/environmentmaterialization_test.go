package local

import (
	"context"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
)

func TestFakeMaterializeEnvironment(t *testing.T) {
	fake := NewFake()
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "session-fake",
		EnvironmentID:          "env-fake",
		EnvironmentName:        "Fake Environment",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.12.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
	result, err := fake.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.SessionID != "session-fake" {
		t.Errorf("session id: got %q, want %q", result.SessionID, "session-fake")
	}
	if result.EnvironmentID != "env-fake" {
		t.Errorf("environment id: got %q, want %q", result.EnvironmentID, "env-fake")
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want %q", result.Status, "prepared")
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1", len(calls))
	}
	if calls[0] != agent.OperationMaterializeEnvironment {
		t.Errorf("operation: got %q, want %q", calls[0], agent.OperationMaterializeEnvironment)
	}
}
