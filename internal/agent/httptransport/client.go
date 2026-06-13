package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type HTTPError struct {
	StatusCode int
	RequestID  string
	AgentID    string
	Operation  string
	Message    string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("agent HTTP %s failed with status %d (request %s): %s", e.Operation, e.StatusCode, e.RequestID, e.Message)
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

var _ agent.AgentClient = (*Client)(nil)

func NewClient(rawURL, token string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid agent URL %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("agent URL scheme must be http or https")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{baseURL: parsed, token: token, httpClient: &http.Client{Timeout: timeout}}, nil
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return agent.WithRequestID(ctx, requestID)
}

func (c *Client) Info(ctx context.Context) (agent.AgentInfo, error) {
	var response AgentInfoResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent", nil, &response); err != nil {
		return agent.AgentInfo{}, err
	}
	return agent.AgentInfo{ID: response.ID, Status: response.Status, RuntimeEndpoint: c.baseURL.String(), Capabilities: response.Capabilities, Mode: "http"}, nil
}

func (c *Client) RuntimeProfiles(ctx context.Context) ([]runtimeprofile.Profile, error) {
	var response RuntimeProfilesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent/runtime-profiles", nil, &response); err != nil {
		return nil, err
	}
	return response.Profiles, nil
}

func (c *Client) PrepareSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "prepare")
}
func (c *Client) StartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "start")
}
func (c *Client) StopSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "stop")
}
func (c *Client) RestartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "restart")
}
func (c *Client) FreezeSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "freeze")
}
func (c *Client) UnfreezeSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	return c.sessionOperation(ctx, request, "unfreeze")
}

func (c *Client) InspectSession(ctx context.Context, sessionID string) (agent.SessionStatus, error) {
	var response SessionInspectResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID)+"/inspect", nil, &response); err != nil {
		return agent.SessionStatus{}, err
	}
	return agent.SessionStatus{AgentID: response.AgentID, SessionID: response.SessionID, Status: response.Status, Running: response.Running, Frozen: response.Frozen, RuntimeEndpoint: response.RuntimeEndpoint, ProcessID: response.ProcessID, PID: response.PID, RuntimeMode: response.RuntimeMode, RuntimeProfileID: response.RuntimeProfileID, RuntimeType: response.RuntimeType, Crashed: response.Crashed, StartedAt: response.StartedAt, StoppedAt: response.StoppedAt, ExitCode: response.ExitCode, LastError: response.LastError, ObservedAt: response.ObservedAt, SessionRoot: response.SessionRoot, WorkDir: response.WorkDir, LogsDir: response.LogsDir}, nil
}

func (c *Client) CollectLogs(ctx context.Context, sessionID string) (agent.LogBatch, error) {
	var response LogsResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/logs"
	if maxBytes := agent.LogMaxBytesFromContext(ctx); maxBytes > 0 {
		path += "?maxBytes=" + strconv.Itoa(maxBytes)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.LogBatch{}, err
	}
	return agent.LogBatch{AgentID: response.AgentID, SessionID: response.SessionID, Lines: response.Lines}, nil
}

func (c *Client) ReportResources(ctx context.Context) (agent.ResourceReport, error) {
	var response ResourceReportResponse
	if err := c.do(ctx, http.MethodGet, "/v1/agent/resources", nil, &response); err != nil {
		return agent.ResourceReport{}, err
	}
	return agent.ResourceReport{AgentID: response.AgentID, CPUCapacity: response.CPUCapacity, MemoryTotalMB: response.MemoryTotalMB, MemoryUsedMB: response.MemoryUsedMB, DiskTotalMB: response.DiskTotalMB, DiskUsedMB: response.DiskUsedMB, RunningSessions: response.RunningSessions, ReportedAt: response.ReportedAt}, nil
}

func (c *Client) CreateCheckpointStub(ctx context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return c.checkpointOperation(ctx, request, "create-stub")
}

func (c *Client) RestoreCheckpointStub(ctx context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return c.checkpointOperation(ctx, request, "restore-stub")
}

func (c *Client) MaterializeArtifact(ctx context.Context, request agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	body := ArtifactMaterializationRequest{SessionID: request.SessionID, ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, ArtifactName: request.ArtifactName, ArtifactType: request.ArtifactType, TargetName: request.TargetName, PayloadAlgorithm: request.PayloadAlgorithm, PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, ActorID: request.ActorID, Payload: request.Payload}
	var response ArtifactMaterializationResponse
	if err := c.do(ctx, http.MethodPost, "/v1/artifacts/materialize", body, &response); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	return agent.ArtifactMaterializationResult{AgentID: response.AgentID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, TargetName: response.TargetName, RuntimeRelativePath: response.RuntimeRelativePath, PayloadHash: response.PayloadHash, PayloadSize: response.PayloadSize, MaterializedAt: response.MaterializedAt, Idempotent: response.Idempotent, Status: response.Status}, nil
}

func (c *Client) InspectMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifacts, error) {
	var response MaterializedArtifactsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID)+"/artifacts", nil, &response); err != nil {
		return agent.MaterializedArtifacts{}, err
	}
	items := make([]agent.MaterializedArtifact, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, agent.MaterializedArtifact{ArtifactID: item.ArtifactID, StagingPlanID: item.StagingPlanID, ArtifactName: item.ArtifactName, ArtifactType: item.ArtifactType, TargetName: item.TargetName, PayloadAlgorithm: item.PayloadAlgorithm, PayloadHash: item.PayloadHash, PayloadSize: item.PayloadSize, RuntimeRelativePath: item.RuntimeRelativePath, MaterializedAt: item.MaterializedAt, ActorID: item.ActorID, Status: item.Status, Metadata: item.Metadata})
	}
	return agent.MaterializedArtifacts{AgentID: response.AgentID, SessionID: response.SessionID, Status: response.Status, Items: items}, nil
}

func (c *Client) InspectMaterializedArtifact(ctx context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifact, error) {
	var response MaterializedArtifactResponse
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/artifacts/" + url.PathEscape(stagingPlanID)
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return agent.MaterializedArtifact{}, err
	}
	return agent.MaterializedArtifact{AgentID: response.AgentID, SessionID: response.SessionID, ArtifactID: response.ArtifactID, StagingPlanID: response.StagingPlanID, ArtifactName: response.ArtifactName, ArtifactType: response.ArtifactType, TargetName: response.TargetName, PayloadAlgorithm: response.PayloadAlgorithm, PayloadHash: response.PayloadHash, PayloadSize: response.PayloadSize, RuntimeRelativePath: response.RuntimeRelativePath, MaterializedAt: response.MaterializedAt, ActorID: response.ActorID, Status: response.Status, Metadata: response.Metadata}, nil
}

func (c *Client) sessionOperation(ctx context.Context, request agent.SessionRequest, operation string) (agent.OperationResult, error) {
	var response SessionOperationResponse
	body := SessionOperationRequest{ProjectID: request.ProjectID, EnvironmentID: request.EnvironmentID, RuntimeProfileID: request.RuntimeProfileID}
	path := "/v1/sessions/" + url.PathEscape(request.SessionID) + "/" + operation
	if err := c.do(ctx, http.MethodPost, path, body, &response); err != nil {
		return agent.OperationResult{}, err
	}
	return agent.OperationResult{AgentID: response.AgentID, Status: response.Status, Message: response.Message, Mode: "http"}, nil
}

func (c *Client) checkpointOperation(ctx context.Context, request agent.CheckpointRequest, operation string) (agent.OperationResult, error) {
	var response CheckpointStubResponse
	body := CheckpointStubRequest{SessionID: request.SessionID, CheckpointID: request.CheckpointID}
	if err := c.do(ctx, http.MethodPost, "/v1/checkpoints/"+operation, body, &response); err != nil {
		return agent.OperationResult{}, err
	}
	return agent.OperationResult{AgentID: response.AgentID, Status: response.Status, Message: response.Message, Mode: "http"}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, response any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode agent request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	requestURL := *c.baseURL
	requestPath, rawQuery, _ := strings.Cut(path, "?")
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + requestPath
	requestURL.RawQuery = rawQuery
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return fmt.Errorf("create agent request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	requestID := agent.RequestIDFromContext(ctx)
	if requestID == "" {
		requestID = newRequestID()
	}
	req.Header.Set(requestIDHeader, requestID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call agent %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp, path)
	}
	if response == nil {
		return nil
	}
	if err := decodeResponse(resp.Body, response); err != nil {
		return fmt.Errorf("decode agent response for %s: %w", path, err)
	}
	return nil
}

func decodeHTTPError(response *http.Response, operation string) error {
	var payload ErrorResponse
	if err := decodeResponse(response.Body, &payload); err != nil {
		return HTTPError{StatusCode: response.StatusCode, RequestID: response.Header.Get(requestIDHeader), Operation: operation, Message: "malformed error response: " + err.Error()}
	}
	if payload.RequestID == "" {
		payload.RequestID = response.Header.Get(requestIDHeader)
	}
	return HTTPError{StatusCode: response.StatusCode, RequestID: payload.RequestID, AgentID: payload.AgentID, Operation: payload.Operation, Message: payload.Error}
}

func decodeResponse(reader io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("response contains multiple JSON values")
	}
	return nil
}
