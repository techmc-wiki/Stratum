package httptransport

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestClientSessionReadyForStartThroughHTTP(t *testing.T) {
	runtime, err := local.NewProcessAgentWithRegistryAndRoot(local.DefaultAgentID, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{SessionID: "session-1", EnvironmentID: "environment-1", EnvironmentName: "Test", MinecraftVersion: "1.17.1", JavaVersion: "17", LoaderType: "fabric", ServerCore: "carpet", RuntimeProfileID: runtimeprofile.DefaultProfileID, RuntimeProfileRequired: true, ActorID: "actor-1"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(runtime, "", nil).Handler())
	defer server.Close()
	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SessionReadyForStart(context.Background(), "session-1")
	if err != nil || !result.Ready || result.Status != "ready" || result.RuntimeStatusSummary.EnvironmentManifestStatus != "prepared" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
