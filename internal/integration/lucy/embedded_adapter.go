package lucy

import (
	"context"
	"errors"
)

type EmbeddedBackend interface {
	Capabilities(ctx context.Context) (Capabilities, error)
	Plan(ctx context.Context, spec EnvironmentSpec) (EnvironmentPlan, error)
	Lock(ctx context.Context, spec EnvironmentSpec) (EnvironmentLock, error)
	Status(ctx context.Context, spec EnvironmentSpec, lock *EnvironmentLock) (EnvironmentStatus, error)
	Install(ctx context.Context, req InstallPackagesRequest) (InstallPackagesResult, error)
	VerifyIntegrity(ctx context.Context, req IntegrityRequest) (IntegrityResult, error)
}

type EmbeddedAdapter struct {
	backend EmbeddedBackend
}

var _ Adapter = (*EmbeddedAdapter)(nil)

func NewEmbeddedAdapter(backend EmbeddedBackend) (*EmbeddedAdapter, error) {
	if backend == nil {
		return nil, NewAdapterError(ErrorCodeInvalidRequest, "backend required", nil, false)
	}
	return &EmbeddedAdapter{backend: backend}, nil
}

func (a *EmbeddedAdapter) Capabilities(ctx context.Context) (Capabilities, error) {
	caps, err := a.backend.Capabilities(ctx)
	if err != nil {
		return Capabilities{}, a.classifyError(err)
	}
	if err := caps.Validate(); err != nil {
		return Capabilities{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend capabilities", err, false)
	}
	return caps, nil
}

func (a *EmbeddedAdapter) PlanEnvironment(ctx context.Context, req PlanEnvironmentRequest) (EnvironmentPlan, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentPlan{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid environment spec", err, false)
	}
	plan, err := a.backend.Plan(ctx, req.Spec)
	if err != nil {
		return EnvironmentPlan{}, a.classifyError(err)
	}
	if err := plan.Validate(); err != nil {
		return EnvironmentPlan{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend plan", err, false)
	}
	return plan, nil
}

func (a *EmbeddedAdapter) LockEnvironment(ctx context.Context, req LockEnvironmentRequest) (EnvironmentLock, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentLock{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid environment spec", err, false)
	}
	lock, err := a.backend.Lock(ctx, req.Spec)
	if err != nil {
		return EnvironmentLock{}, a.classifyError(err)
	}
	if err := lock.Validate(); err != nil {
		return EnvironmentLock{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend lock", err, false)
	}
	return lock, nil
}

func (a *EmbeddedAdapter) CheckStatus(ctx context.Context, req StatusRequest) (EnvironmentStatus, error) {
	if err := req.Spec.Validate(); err != nil {
		return EnvironmentStatus{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid environment spec", err, false)
	}
	if req.Lock != nil {
		if err := req.Lock.Validate(); err != nil {
			return EnvironmentStatus{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid lock", err, false)
		}
	}
	status, err := a.backend.Status(ctx, req.Spec, req.Lock)
	if err != nil {
		return EnvironmentStatus{}, a.classifyError(err)
	}
	if err := status.Validate(); err != nil {
		return EnvironmentStatus{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend status", err, false)
	}
	return status, nil
}

func (a *EmbeddedAdapter) InstallPackages(ctx context.Context, req InstallPackagesRequest) (InstallPackagesResult, error) {
	if err := req.Validate(); err != nil {
		return InstallPackagesResult{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid install request", err, false)
	}
	result, err := a.backend.Install(ctx, req)
	if err != nil {
		return InstallPackagesResult{}, a.classifyError(err)
	}
	fillInstallPaths(req, &result)
	result = validateInstalledHashes(req, result)
	if err := result.Validate(); err != nil {
		return InstallPackagesResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend install result", err, false)
	}
	return result, nil
}

func (a *EmbeddedAdapter) VerifyIntegrity(ctx context.Context, req IntegrityRequest) (IntegrityResult, error) {
	if err := req.Validate(); err != nil {
		return IntegrityResult{}, NewAdapterError(ErrorCodeInvalidRequest, "invalid integrity request", err, false)
	}
	result, err := a.backend.VerifyIntegrity(ctx, req)
	if err != nil {
		return IntegrityResult{}, a.classifyError(err)
	}
	if err := result.Validate(); err != nil {
		return IntegrityResult{}, NewAdapterError(ErrorCodeValidationFailed, "invalid backend integrity result", err, false)
	}
	return result, nil
}

func (a *EmbeddedAdapter) classifyError(err error) error {
	var aerr AdapterError
	if errors.As(err, &aerr) {
		return aerr
	}
	return NewAdapterError(ErrorCodeInternalError, "backend error", err, false)
}
