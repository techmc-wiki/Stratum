package filesystem

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactapply"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/room"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
)

const directoryPermissions = 0o750

type Store struct {
	Root    string
	auditMu sync.Mutex
}

func New(root string) (*Store, error) {
	const operation = "filesystem.New"
	if strings.TrimSpace(root) == "" {
		return nil, validationError(operation, "filesystem repository root is required")
	}
	root = filepath.Clean(root)
	for _, directory := range []string{
		"projects", "rooms", "sessions", "checkpoints", "artifacts",
		"artifact-staging-plans", "artifact-apply-plans", "environments", "resource-policies", "operations", "runtime-observations", "audit",
	} {
		if err := os.MkdirAll(filepath.Join(root, directory), directoryPermissions); err != nil {
			return nil, repositoryError(stratumerrors.KindConflict, operation, "create metadata directory", err)
		}
	}
	return &Store{Root: root}, nil
}

func validateID(operation, id string) error {
	if id == "" || id == "." || id == ".." {
		return validationError(operation, "metadata id is required")
	}
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return validationError(operation, fmt.Sprintf("metadata id %q contains unsupported characters", id))
	}
	return nil
}

func (s *Store) entityPath(directory, id string) string {
	return filepath.Join(s.Root, directory, id+".json")
}

func createJSON[T any](path, operation string, value T) error {
	if _, err := os.Stat(path); err == nil {
		return repositoryError(stratumerrors.KindConflict, operation, "metadata already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return repositoryError(stratumerrors.KindConflict, operation, "inspect existing metadata", err)
	}
	return writeJSONAtomic(path, operation, value)
}

func updateJSON[T any](path, operation string, value T) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repositoryError(stratumerrors.KindNotFound, operation, "metadata does not exist", nil)
		}
		return repositoryError(stratumerrors.KindConflict, operation, "inspect metadata", err)
	}
	return writeJSONAtomic(path, operation, value)
}

func writeJSONAtomic[T any](path, operation string, value T) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryPermissions); err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "create metadata directory", err)
	}
	temporary, err := os.CreateTemp(directory, ".stratum-*.tmp")
	if err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "create temporary metadata file", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temporary.Close()
		return repositoryError(stratumerrors.KindConflict, operation, "encode metadata", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return repositoryError(stratumerrors.KindConflict, operation, "sync metadata", err)
	}
	if err := temporary.Close(); err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "close metadata", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "replace metadata", err)
	}
	return nil
}

func readJSON[T any](path, operation string) (T, error) {
	var value T
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return value, repositoryError(stratumerrors.KindNotFound, operation, "metadata does not exist", nil)
		}
		return value, repositoryError(stratumerrors.KindConflict, operation, "open metadata", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, repositoryError(stratumerrors.KindConflict, operation, "decode metadata", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return value, repositoryError(stratumerrors.KindConflict, operation, "decode metadata", err)
	}
	return value, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("metadata contains multiple JSON values")
}

func listJSON[T any](directory, operation string) ([]T, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, repositoryError(stratumerrors.KindConflict, operation, "list metadata directory", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	values := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		value, err := readJSON[T](filepath.Join(directory, entry.Name()), operation)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func deleteJSON(path, operation string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repositoryError(stratumerrors.KindNotFound, operation, "metadata does not exist", nil)
		}
		return repositoryError(stratumerrors.KindConflict, operation, "delete metadata", err)
	}
	return nil
}

func repositoryError(kind stratumerrors.Kind, operation, message string, cause error) error {
	return stratumerrors.Error{Kind: kind, Operation: operation, Message: message, Cause: cause}
}

func validationError(operation, message string) error {
	return repositoryError(stratumerrors.KindValidation, operation, message, nil)
}

func (s *Store) CreateProject(_ context.Context, value project.Project) error {
	const operation = "filesystem.CreateProject"
	if err := validateProject(operation, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("projects", value.ID), operation, value)
}
func (s *Store) GetProject(_ context.Context, id string) (project.Project, error) {
	const operation = "filesystem.GetProject"
	if err := validateID(operation, id); err != nil {
		return project.Project{}, err
	}
	return readJSON[project.Project](s.entityPath("projects", id), operation)
}
func (s *Store) ListProjects(_ context.Context) ([]project.Project, error) {
	return listJSON[project.Project](filepath.Join(s.Root, "projects"), "filesystem.ListProjects")
}
func (s *Store) UpdateProject(_ context.Context, value project.Project) error {
	const operation = "filesystem.UpdateProject"
	if err := validateProject(operation, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("projects", value.ID), operation, value)
}
func (s *Store) DeleteProject(_ context.Context, id string) error {
	const op = "filesystem.DeleteProject"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("projects", id), op)
}

func (s *Store) CreateRoom(_ context.Context, value room.Room) error {
	const op = "filesystem.CreateRoom"
	if err := validateRoom(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("rooms", value.ID), op, value)
}
func (s *Store) GetRoom(_ context.Context, id string) (room.Room, error) {
	const op = "filesystem.GetRoom"
	if err := validateID(op, id); err != nil {
		return room.Room{}, err
	}
	return readJSON[room.Room](s.entityPath("rooms", id), op)
}
func (s *Store) ListRooms(_ context.Context) ([]room.Room, error) {
	return listJSON[room.Room](filepath.Join(s.Root, "rooms"), "filesystem.ListRooms")
}
func (s *Store) UpdateRoom(_ context.Context, value room.Room) error {
	const op = "filesystem.UpdateRoom"
	if err := validateRoom(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("rooms", value.ID), op, value)
}
func (s *Store) DeleteRoom(_ context.Context, id string) error {
	const op = "filesystem.DeleteRoom"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("rooms", id), op)
}

func (s *Store) CreateSession(_ context.Context, value session.Session) error {
	const op = "filesystem.CreateSession"
	if err := validateSession(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("sessions", value.ID), op, value)
}
func (s *Store) SaveSession(ctx context.Context, value session.Session) error {
	if _, err := s.GetSession(ctx, value.ID); err == nil {
		return s.UpdateSession(ctx, value)
	} else if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return err
	}
	return s.CreateSession(ctx, value)
}
func (s *Store) GetSession(_ context.Context, id string) (session.Session, error) {
	const op = "filesystem.GetSession"
	if err := validateID(op, id); err != nil {
		return session.Session{}, err
	}
	return readJSON[session.Session](s.entityPath("sessions", id), op)
}
func (s *Store) ListSessions(_ context.Context) ([]session.Session, error) {
	return listJSON[session.Session](filepath.Join(s.Root, "sessions"), "filesystem.ListSessions")
}
func (s *Store) UpdateSession(_ context.Context, value session.Session) error {
	const op = "filesystem.UpdateSession"
	if err := validateSession(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("sessions", value.ID), op, value)
}
func (s *Store) DeleteSession(_ context.Context, id string) error {
	const op = "filesystem.DeleteSession"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("sessions", id), op)
}

func (s *Store) CreateOperation(_ context.Context, value operation.Operation) error {
	const op = "filesystem.CreateOperation"
	if err := validateOperation(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("operations", value.ID), op, value)
}

func (s *Store) GetOperation(_ context.Context, id string) (operation.Operation, error) {
	const op = "filesystem.GetOperation"
	if err := validateID(op, id); err != nil {
		return operation.Operation{}, err
	}
	return readJSON[operation.Operation](s.entityPath("operations", id), op)
}

func (s *Store) ListOperations(_ context.Context) ([]operation.Operation, error) {
	return listJSON[operation.Operation](filepath.Join(s.Root, "operations"), "filesystem.ListOperations")
}

func (s *Store) UpdateOperation(_ context.Context, value operation.Operation) error {
	const op = "filesystem.UpdateOperation"
	if err := validateOperation(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("operations", value.ID), op, value)
}

func (s *Store) ListOperationsBySession(ctx context.Context, sessionID string) ([]operation.Operation, error) {
	values, err := s.ListOperations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]operation.Operation, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) ListActiveOperationsBySession(ctx context.Context, sessionID string) ([]operation.Operation, error) {
	values, err := s.ListOperationsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]operation.Operation, 0)
	for _, value := range values {
		if value.Active() {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateRuntimeObservation(_ context.Context, value runtimeobservation.Observation) error {
	const op = "filesystem.CreateRuntimeObservation"
	if err := validateRuntimeObservation(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("runtime-observations", value.ID), op, value)
}

func (s *Store) GetRuntimeObservation(_ context.Context, id string) (runtimeobservation.Observation, error) {
	const op = "filesystem.GetRuntimeObservation"
	if err := validateID(op, id); err != nil {
		return runtimeobservation.Observation{}, err
	}
	return readJSON[runtimeobservation.Observation](s.entityPath("runtime-observations", id), op)
}

func (s *Store) ListRuntimeObservations(_ context.Context) ([]runtimeobservation.Observation, error) {
	return listJSON[runtimeobservation.Observation](filepath.Join(s.Root, "runtime-observations"), "filesystem.ListRuntimeObservations")
}

func (s *Store) ListRuntimeObservationsBySession(ctx context.Context, sessionID string) ([]runtimeobservation.Observation, error) {
	values, err := s.ListRuntimeObservations(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]runtimeobservation.Observation, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateCheckpoint(_ context.Context, value checkpoint.Checkpoint) error {
	const op = "filesystem.CreateCheckpoint"
	if err := validateCheckpoint(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("checkpoints", value.ID), op, value)
}
func (s *Store) SaveCheckpoint(ctx context.Context, value checkpoint.Checkpoint) error {
	if _, err := s.GetCheckpoint(ctx, value.ID); err == nil {
		return s.UpdateCheckpoint(ctx, value)
	} else if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return err
	}
	return s.CreateCheckpoint(ctx, value)
}
func (s *Store) GetCheckpoint(_ context.Context, id string) (checkpoint.Checkpoint, error) {
	const op = "filesystem.GetCheckpoint"
	if err := validateID(op, id); err != nil {
		return checkpoint.Checkpoint{}, err
	}
	return readJSON[checkpoint.Checkpoint](s.entityPath("checkpoints", id), op)
}
func (s *Store) ListCheckpoints(_ context.Context) ([]checkpoint.Checkpoint, error) {
	return listJSON[checkpoint.Checkpoint](filepath.Join(s.Root, "checkpoints"), "filesystem.ListCheckpoints")
}
func (s *Store) UpdateCheckpoint(_ context.Context, value checkpoint.Checkpoint) error {
	const op = "filesystem.UpdateCheckpoint"
	if err := validateCheckpoint(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("checkpoints", value.ID), op, value)
}
func (s *Store) DeleteCheckpoint(_ context.Context, id string) error {
	const op = "filesystem.DeleteCheckpoint"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("checkpoints", id), op)
}

func (s *Store) CreateArtifact(_ context.Context, value artifact.Artifact) error {
	const op = "filesystem.CreateArtifact"
	if err := validateArtifact(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("artifacts", value.ID), op, value)
}
func (s *Store) SaveArtifact(ctx context.Context, value artifact.Artifact) error {
	if _, err := s.GetArtifact(ctx, value.ID); err == nil {
		return s.UpdateArtifact(ctx, value)
	} else if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return err
	}
	return s.CreateArtifact(ctx, value)
}
func (s *Store) GetArtifact(_ context.Context, id string) (artifact.Artifact, error) {
	const op = "filesystem.GetArtifact"
	if err := validateID(op, id); err != nil {
		return artifact.Artifact{}, err
	}
	return readJSON[artifact.Artifact](s.entityPath("artifacts", id), op)
}
func (s *Store) ListArtifacts(_ context.Context) ([]artifact.Artifact, error) {
	return listJSON[artifact.Artifact](filepath.Join(s.Root, "artifacts"), "filesystem.ListArtifacts")
}
func (s *Store) UpdateArtifact(_ context.Context, value artifact.Artifact) error {
	const op = "filesystem.UpdateArtifact"
	if err := validateArtifact(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("artifacts", value.ID), op, value)
}
func (s *Store) DeleteArtifact(_ context.Context, id string) error {
	const op = "filesystem.DeleteArtifact"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("artifacts", id), op)
}

func (s *Store) CreateArtifactStagingPlan(_ context.Context, value artifactstaging.Plan) error {
	const op = "filesystem.CreateArtifactStagingPlan"
	if err := validateArtifactStagingPlan(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("artifact-staging-plans", value.ID), op, value)
}

func (s *Store) GetArtifactStagingPlan(_ context.Context, id string) (artifactstaging.Plan, error) {
	const op = "filesystem.GetArtifactStagingPlan"
	if err := validateID(op, id); err != nil {
		return artifactstaging.Plan{}, err
	}
	return readJSON[artifactstaging.Plan](s.entityPath("artifact-staging-plans", id), op)
}

func (s *Store) ListArtifactStagingPlans(_ context.Context) ([]artifactstaging.Plan, error) {
	return listJSON[artifactstaging.Plan](filepath.Join(s.Root, "artifact-staging-plans"), "filesystem.ListArtifactStagingPlans")
}

func (s *Store) ListArtifactStagingPlansBySession(ctx context.Context, sessionID string) ([]artifactstaging.Plan, error) {
	values, err := s.ListArtifactStagingPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactstaging.Plan, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) ListArtifactStagingPlansByArtifact(ctx context.Context, artifactID string) ([]artifactstaging.Plan, error) {
	values, err := s.ListArtifactStagingPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactstaging.Plan, 0)
	for _, value := range values {
		if value.ArtifactID == artifactID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateArtifactApplyPlan(_ context.Context, value artifactapply.Plan) error {
	const op = "filesystem.CreateArtifactApplyPlan"
	if err := validateArtifactApplyPlan(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("artifact-apply-plans", value.ID), op, value)
}

func (s *Store) GetArtifactApplyPlan(_ context.Context, id string) (artifactapply.Plan, error) {
	const op = "filesystem.GetArtifactApplyPlan"
	if err := validateID(op, id); err != nil {
		return artifactapply.Plan{}, err
	}
	return readJSON[artifactapply.Plan](s.entityPath("artifact-apply-plans", id), op)
}

func (s *Store) ListArtifactApplyPlans(_ context.Context) ([]artifactapply.Plan, error) {
	return listJSON[artifactapply.Plan](filepath.Join(s.Root, "artifact-apply-plans"), "filesystem.ListArtifactApplyPlans")
}

func (s *Store) ListArtifactApplyPlansBySession(ctx context.Context, sessionID string) ([]artifactapply.Plan, error) {
	values, err := s.ListArtifactApplyPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactapply.Plan, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateEnvironment(_ context.Context, value environment.Environment) error {
	const op = "filesystem.CreateEnvironment"
	if err := validateEnvironment(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("environments", value.ID), op, value)
}
func (s *Store) GetEnvironment(_ context.Context, id string) (environment.Environment, error) {
	const op = "filesystem.GetEnvironment"
	if err := validateID(op, id); err != nil {
		return environment.Environment{}, err
	}
	return readJSON[environment.Environment](s.entityPath("environments", id), op)
}
func (s *Store) ListEnvironments(_ context.Context) ([]environment.Environment, error) {
	return listJSON[environment.Environment](filepath.Join(s.Root, "environments"), "filesystem.ListEnvironments")
}
func (s *Store) UpdateEnvironment(ctx context.Context, value environment.Environment, expectedUpdatedAt time.Time) error {
	const op = "filesystem.UpdateEnvironment"
	if err := validateEnvironment(op, value); err != nil {
		return err
	}
	existing, err := s.GetEnvironment(ctx, value.ID)
	if err != nil {
		return err
	}
	if !existing.UpdatedAt.Equal(expectedUpdatedAt) {
		return repositoryError(stratumerrors.KindConflict, op, fmt.Sprintf("expected updated_at %s, got %s", expectedUpdatedAt.Format(time.RFC3339Nano), existing.UpdatedAt.Format(time.RFC3339Nano)), nil)
	}
	return updateJSON(s.entityPath("environments", value.ID), op, value)
}
func (s *Store) DeleteEnvironment(_ context.Context, id string) error {
	const op = "filesystem.DeleteEnvironment"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("environments", id), op)
}

func (s *Store) CreateResourcePolicy(_ context.Context, value resourcepolicy.Policy) error {
	const op = "filesystem.CreateResourcePolicy"
	if err := validateResourcePolicy(op, value); err != nil {
		return err
	}
	return createJSON(s.entityPath("resource-policies", value.ID), op, value)
}
func (s *Store) GetResourcePolicy(_ context.Context, id string) (resourcepolicy.Policy, error) {
	const op = "filesystem.GetResourcePolicy"
	if err := validateID(op, id); err != nil {
		return resourcepolicy.Policy{}, err
	}
	return readJSON[resourcepolicy.Policy](s.entityPath("resource-policies", id), op)
}
func (s *Store) ListResourcePolicies(_ context.Context) ([]resourcepolicy.Policy, error) {
	return listJSON[resourcepolicy.Policy](filepath.Join(s.Root, "resource-policies"), "filesystem.ListResourcePolicies")
}
func (s *Store) UpdateResourcePolicy(_ context.Context, value resourcepolicy.Policy) error {
	const op = "filesystem.UpdateResourcePolicy"
	if err := validateResourcePolicy(op, value); err != nil {
		return err
	}
	return updateJSON(s.entityPath("resource-policies", value.ID), op, value)
}
func (s *Store) DeleteResourcePolicy(_ context.Context, id string) error {
	const op = "filesystem.DeleteResourcePolicy"
	if err := validateID(op, id); err != nil {
		return err
	}
	return deleteJSON(s.entityPath("resource-policies", id), op)
}

func (s *Store) AppendAuditEvent(_ context.Context, value audit.Event) error {
	const operation = "filesystem.AppendAuditEvent"
	if err := validateAuditEvent(operation, value); err != nil {
		return err
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	path := filepath.Join(s.Root, "audit", "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "open audit log", err)
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return repositoryError(stratumerrors.KindConflict, operation, "append audit event", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return repositoryError(stratumerrors.KindConflict, operation, "sync audit log", err)
	}
	if err := file.Close(); err != nil {
		return repositoryError(stratumerrors.KindConflict, operation, "close audit log", err)
	}
	return nil
}

func (s *Store) ListAuditEvents(_ context.Context) ([]audit.Event, error) {
	const operation = "filesystem.ListAuditEvents"
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	file, err := os.Open(filepath.Join(s.Root, "audit", "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return []audit.Event{}, nil
	}
	if err != nil {
		return nil, repositoryError(stratumerrors.KindConflict, operation, "open audit log", err)
	}
	defer file.Close()
	var events []audit.Event
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		var event audit.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, repositoryError(stratumerrors.KindConflict, operation, fmt.Sprintf("decode audit event on line %d", line), err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, repositoryError(stratumerrors.KindConflict, operation, "read audit log", err)
	}
	return events, nil
}

func validateProject(op string, value project.Project) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if strings.TrimSpace(value.Name) == "" || value.CreatedAt.IsZero() {
		return validationError(op, "project requires name and creation time")
	}
	return nil
}
func validateRoom(op string, value room.Room) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.ProjectID == "" || strings.TrimSpace(value.Name) == "" || value.EnvironmentID == "" || value.BaseWorldRef == "" || value.CreatedAt.IsZero() {
		return validationError(op, "room requires project, name, environment, base world, and creation time")
	}
	return nil
}
func validateSession(op string, value session.Session) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.ProjectID == "" || value.OwnerUserID == "" || value.Type == "" || value.State == "" || value.EnvironmentID == "" || value.CreatedAt.IsZero() || value.LastActiveAt.IsZero() {
		return validationError(op, "session requires project, owner, type, state, environment, creation time, and last-active time")
	}
	return nil
}

func validateOperation(op string, value operation.Operation) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.RequestID == "" || value.ActorID == "" || value.Action == "" || value.TargetType == "" || value.TargetID == "" || value.Status == "" || value.CreatedAt.IsZero() {
		return validationError(op, "operation requires request, actor, action, target, status, and creation time")
	}
	return nil
}
func validateRuntimeObservation(op string, value runtimeobservation.Observation) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.SessionID == "" || value.ObservedAt.IsZero() || value.ControllerSessionState == "" || value.MismatchType == "" || value.Severity == "" || value.RecommendedAction == "" {
		return validationError(op, "runtime observation requires session, observed time, controller state, mismatch type, severity, and recommended action")
	}
	return nil
}
func validateCheckpoint(op string, value checkpoint.Checkpoint) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.ProjectID == "" || value.SourceSessionID == "" || value.CreatorID == "" || value.Kind == "" || value.WorldStateRef == "" || value.EnvironmentID == "" || value.CreatedAt.IsZero() {
		return validationError(op, "checkpoint requires project, source session, creator, kind, world state, environment, and creation time")
	}
	return nil
}
func validateArtifact(op string, value artifact.Artifact) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if strings.TrimSpace(value.Name) == "" || value.Type == "" || value.UploaderID == "" || value.Status == "" || value.CreatedAt.IsZero() {
		return validationError(op, "artifact requires name, type, uploader, status, and creation time")
	}
	if err := artifact.ValidateType(value.Type); err != nil {
		return validationError(op, err.Error())
	}
	switch value.PayloadStatus {
	case artifact.PayloadMetadataOnly:
		if value.SHA256 != "" || value.SizeBytes != 0 || value.PayloadAlgorithm != "" || value.PayloadReference != "" || value.PayloadImportedBy != "" || value.PayloadImportedAt != nil {
			return validationError(op, "metadata-only artifact must not define payload metadata")
		}
	case artifact.PayloadAvailable:
		if len(value.SHA256) != 64 {
			return validationError(op, "available artifact requires a SHA-256 hash")
		}
		if value.PayloadAlgorithm != "" && value.PayloadAlgorithm != "sha256" {
			return validationError(op, fmt.Sprintf("unsupported artifact payload algorithm %q", value.PayloadAlgorithm))
		}
	case "":
		if value.SHA256 == "" {
			return validationError(op, "legacy artifact requires a payload hash")
		}
	default:
		return validationError(op, fmt.Sprintf("unsupported artifact payload status %q", value.PayloadStatus))
	}
	return nil
}
func validateArtifactStagingPlan(op string, value artifactstaging.Plan) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.SessionID == "" || value.ProjectID == "" || value.ArtifactID == "" || value.ArtifactName == "" || value.ArtifactType == "" || value.ArtifactStatus == "" || value.TargetStagingName == "" || value.StagingKind == "" || value.ActorID == "" || value.Status == "" || value.CreatedAt.IsZero() {
		return validationError(op, "artifact staging plan requires session, project, artifact metadata, staging target, actor, status, and creation time")
	}
	return nil
}
func validateArtifactApplyPlan(op string, value artifactapply.Plan) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.SessionID == "" || value.ProjectID == "" || value.ActorID == "" || value.SourceStagingPlanID == "" || value.ArtifactID == "" || value.MaterializedArtifactHash == "" || value.MaterializedArtifactName == "" || value.ApplyKind == "" || value.TargetRoot == "" || value.TargetRelativePath == "" || value.Status == "" || value.CreatedAt.IsZero() {
		return validationError(op, "artifact apply plan requires session, project, actor, staging plan, artifact, materialized hash/name, apply kind, target root, target path, status, and creation time")
	}
	return nil
}
func validateEnvironment(op string, value environment.Environment) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.Name == "" || value.MinecraftVersion == "" || value.LoaderType == "" || value.ServerCore == "" {
		return validationError(op, "environment requires name, Minecraft version, loader type, and server core")
	}
	if err := value.Validate(); err != nil {
		return validationError(op, err.Error())
	}
	return nil
}
func validateResourcePolicy(op string, value resourcepolicy.Policy) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.GlobalMaxRunning < 0 || value.PerProjectMax < 0 || value.PerUserMax < 0 || value.ReviewMaxRunning < 0 {
		return validationError(op, "resource policy limits cannot be negative")
	}
	return nil
}
func validateAuditEvent(op string, value audit.Event) error {
	if err := validateID(op, value.ID); err != nil {
		return err
	}
	if value.ActorID == "" || value.Action == "" || value.TargetType == "" || value.TargetID == "" || value.CreatedAt.IsZero() {
		return validationError(op, "audit event requires actor, action, target, and creation time")
	}
	return nil
}
