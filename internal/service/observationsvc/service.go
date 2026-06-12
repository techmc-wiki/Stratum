package observationsvc

import (
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
)

type Options struct {
	ObservedAt               time.Time
	ExpectedRuntimeProfileID string
	StaleAfter               time.Duration
	LogsAvailable            *bool
	ResourceSnapshot         *runtimeobservation.ResourceSnapshot
	Metadata                 map[string]string
}

func Observe(controller session.Session, status *agent.SessionStatus, options Options) runtimeobservation.Observation {
	observedAt := options.ObservedAt.UTC()
	if observedAt.IsZero() && status != nil && !status.ObservedAt.IsZero() {
		observedAt = status.ObservedAt.UTC()
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	result := runtimeobservation.Observation{
		ID:                     fmt.Sprintf("runtime-observation-%s-%d", controller.ID, observedAt.UnixNano()),
		SessionID:              controller.ID,
		ProjectID:              controller.ProjectID,
		RoomID:                 controller.RoomID,
		ObservedAt:             observedAt,
		ControllerSessionState: string(controller.State),
		ResourceSnapshot:       options.ResourceSnapshot,
		MismatchType:           runtimeobservation.MismatchNone,
		Severity:               runtimeobservation.SeverityInfo,
		RecommendedAction:      runtimeobservation.ActionNone,
		Metadata:               cloneMetadata(options.Metadata),
	}
	if status != nil {
		result.ObserverAgentID = status.AgentID
		result.AgentRuntimeStatus = status.Status
		result.RuntimeProfileID = status.RuntimeProfileID
		result.ProcessID = status.ProcessID
		result.PID = status.PID
		result.ExitCode = status.ExitCode
		result.Crashed = status.Crashed
		result.LastError = status.LastError
		result.LogsAvailable = status.ProcessID != ""
	}
	if options.LogsAvailable != nil {
		result.LogsAvailable = *options.LogsAvailable
	}

	result.MismatchType, result.Severity, result.RecommendedAction = classify(controller, status, options, observedAt)
	result.MismatchDetected = result.MismatchType != runtimeobservation.MismatchNone
	return result
}

func classify(controller session.Session, status *agent.SessionStatus, options Options, observedAt time.Time) (runtimeobservation.MismatchType, runtimeobservation.Severity, runtimeobservation.RecommendedAction) {
	controllerRunning := controller.State == session.StateRunning || controller.State == session.StateFrozen
	if status == nil || status.Status == "" || status.Status == "unknown" || status.Status == "not_started" {
		if controllerRunning {
			return runtimeobservation.MismatchAgentUnknownControllerRunning, runtimeobservation.SeverityCritical, runtimeobservation.ActionInspect
		}
		return runtimeobservation.MismatchNone, runtimeobservation.SeverityInfo, runtimeobservation.ActionNone
	}
	if controller.AssignedAgentID != "" && status.AgentID != "" && controller.AssignedAgentID != status.AgentID {
		return runtimeobservation.MismatchAssignedAgent, runtimeobservation.SeverityCritical, runtimeobservation.ActionManualReview
	}
	if options.ExpectedRuntimeProfileID != "" && status.RuntimeProfileID != "" && options.ExpectedRuntimeProfileID != status.RuntimeProfileID {
		return runtimeobservation.MismatchRuntimeProfile, runtimeobservation.SeverityWarning, runtimeobservation.ActionManualReview
	}
	if options.StaleAfter > 0 && !status.ObservedAt.IsZero() && observedAt.Sub(status.ObservedAt) > options.StaleAfter {
		return runtimeobservation.MismatchStaleObservation, runtimeobservation.SeverityWarning, runtimeobservation.ActionInspect
	}
	if controllerRunning && (status.Crashed || status.Status == "crashed") {
		return runtimeobservation.MismatchControllerRunningAgentCrashed, runtimeobservation.SeverityCritical, runtimeobservation.ActionMarkCrashed
	}
	if controllerRunning && (status.Status == "stopped" || status.Status == "exited" || !status.Running) {
		return runtimeobservation.MismatchControllerRunningAgentStopped, runtimeobservation.SeverityWarning, runtimeobservation.ActionMarkStopped
	}
	if controller.State == session.StateStopped && status.Running {
		return runtimeobservation.MismatchControllerStoppedAgentRunning, runtimeobservation.SeverityCritical, runtimeobservation.ActionStopRuntime
	}
	if controller.AssignedAgentID == "" && status.Running {
		return runtimeobservation.MismatchControllerUnknownAgentKnown, runtimeobservation.SeverityWarning, runtimeobservation.ActionManualReview
	}
	return runtimeobservation.MismatchNone, runtimeobservation.SeverityInfo, runtimeobservation.ActionNone
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
