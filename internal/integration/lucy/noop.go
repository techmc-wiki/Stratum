package lucy

import "context"

// NoopAdapter implements the Lucy boundary without performing I/O, dependency
// resolution, command execution, or manifest processing.
type NoopAdapter struct{}

var _ Adapter = NoopAdapter{}

func (NoopAdapter) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		SupportedSources: []string{},
		SupportedLoaders: []string{},
		Metadata:         map[string]string{},
	}, nil
}

func (NoopAdapter) PlanEnvironment(context.Context, PlanEnvironmentRequest) (EnvironmentPlan, error) {
	return EnvironmentPlan{
		Actions:  []PlanAction{},
		Warnings: []string{},
		Errors:   []string{},
		Metadata: map[string]string{},
	}, nil
}

func (NoopAdapter) LockEnvironment(context.Context, LockEnvironmentRequest) (EnvironmentLock, error) {
	return EnvironmentLock{
		Packages:         []LockedPackage{},
		Artifacts:        []LockedArtifact{},
		ProviderMetadata: map[string]string{},
	}, nil
}

func (NoopAdapter) CheckStatus(context.Context, StatusRequest) (EnvironmentStatus, error) {
	return EnvironmentStatus{
		Missing:  []string{},
		Drifted:  []string{},
		Warnings: []string{},
		Errors:   []string{},
		Metadata: map[string]string{},
	}, nil
}
