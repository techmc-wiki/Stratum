package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
	checkpointsvc "github.com/stratummc/stratum/internal/checkpoint/service"
	"github.com/stratummc/stratum/internal/controller/agentregistry"
)

type Server struct {
	mux          *http.ServeMux
	repo         checkpointsvc.Repository
	agentClient  agent.AgentClient
	agentService *agentregistry.Service
}

type CheckpointRestoreRequest struct {
	CheckpointID    string `json:"checkpointId"`
	TargetSessionID string `json:"targetSessionId"`
	WorldDirRel     string `json:"worldDirRel,omitempty"`
	ActorID         string `json:"actorId"`
	Notes           string `json:"notes,omitempty"`
}

type CheckpointRestoreResponse struct {
	CheckpointID  string `json:"checkpointId"`
	WorldStateRef string `json:"worldStateRef"`
	RequestID     string `json:"requestId"`
}

func NewServer() *Server {
	return NewServerWithServices(nil, nil)
}

func NewServerWithServices(repo checkpointsvc.Repository, agentClient agent.AgentClient) *Server {
	mux := http.NewServeMux()
	s := &Server{mux: mux, repo: repo, agentClient: agentClient}
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/checkpoints/restore", s.restoreCheckpoint)
	mux.HandleFunc("POST /v1/agents/register", s.registerAgent)
	mux.HandleFunc("POST /v1/agents/heartbeat", s.agentHeartbeat)
	mux.HandleFunc("GET /v1/agents", s.listAgents)
	return s
}

func (s *Server) WithAgentRegistry(svc *agentregistry.Service) *Server {
	s.agentService = svc
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) restoreCheckpoint(writer http.ResponseWriter, request *http.Request) {
	if s.repo == nil || s.agentClient == nil {
		writeError(writer, http.StatusServiceUnavailable, "checkpoint restore service unavailable")
		return
	}

	var body CheckpointRestoreRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "decode request body: "+err.Error())
		return
	}

	if err := validateCheckpointRestoreRequest(body); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	restored, err := checkpointsvc.Restore(request.Context(), s.repo, checkpointsvc.RestoreRequest{
		CheckpointID:    body.CheckpointID,
		TargetSessionID: body.TargetSessionID,
		WorldDirRel:     body.WorldDirRel,
		ActorID:         body.ActorID,
		Notes:           body.Notes,
		AgentClient:     s.agentClient,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(writer, http.StatusOK, CheckpointRestoreResponse{
		CheckpointID:  restored.ID,
		WorldStateRef: restored.WorldStateRef,
		RequestID:     agent.RequestIDFromContext(request.Context()),
	})
}

func validateCheckpointRestoreRequest(request CheckpointRestoreRequest) error {
	var missing []string
	if strings.TrimSpace(request.CheckpointID) == "" {
		missing = append(missing, "checkpointId")
	}
	if strings.TrimSpace(request.TargetSessionID) == "" {
		missing = append(missing, "targetSessionId")
	}
	if strings.TrimSpace(request.ActorID) == "" {
		missing = append(missing, "actorId")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required field(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

// TODO: add authenticated project, room, session, checkpoint, and artifact routes.

func (s *Server) registerAgent(writer http.ResponseWriter, request *http.Request) {
	if s.agentService == nil {
		writeError(writer, http.StatusServiceUnavailable, "agent registry not configured")
		return
	}
	listen := request.Header.Get("X-Agent-Listen")
	mode := request.Header.Get("X-Agent-Mode")
	info := agent.AgentInfo{
		ID:              "agent-" + strings.ReplaceAll(listen, ":", "-"),
		RuntimeEndpoint: "http://" + listen,
		Mode:            mode,
		Capabilities:    []string{"start-session", "stop-session", "send-command"},
	}
	if err := s.agentService.Register(request.Context(), info); err != nil {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "registered", "id": info.ID})
}

func (s *Server) agentHeartbeat(writer http.ResponseWriter, request *http.Request) {
	if s.agentService == nil {
		writeError(writer, http.StatusServiceUnavailable, "agent registry not configured")
		return
	}
	listen := request.Header.Get("X-Agent-Listen")
	agentID := "agent-" + strings.ReplaceAll(listen, ":", "-")
	if err := s.agentService.Heartbeat(request.Context(), agentID); err != nil {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listAgents(writer http.ResponseWriter, request *http.Request) {
	if s.agentService == nil {
		writeError(writer, http.StatusServiceUnavailable, "agent registry not configured")
		return
	}
	agents, err := s.agentService.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, agents)
}
