package lucybridge

import (
	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/integration/lucy"
)

func EnvironmentToSpec(env environment.Environment) (lucy.EnvironmentSpec, error) {
	spec := lucy.EnvironmentSpec{
		EnvironmentID:    env.ID,
		MinecraftVersion: env.MinecraftVersion,
		JavaVersion:      env.JavaVersion,
		LoaderType:       string(env.LoaderType),
		LoaderVersion:    env.LoaderVersion,
		ServerCore:       string(env.ServerCore),
		CarpetRequired:   env.CarpetRequired,
		MCDRRequired:     env.MCDRRequired,
		RuntimeProfileID: env.RuntimeProfileID,
		Packages:         []lucy.PackageRef{},
		LocalArtifacts:   []lucy.LocalArtifactRef{},
		Metadata:         make(map[string]string),
	}
	if env.Metadata != nil {
		for k, v := range env.Metadata {
			spec.Metadata[k] = v
		}
	}
	if err := spec.Validate(); err != nil {
		return lucy.EnvironmentSpec{}, err
	}
	return spec, nil
}

func ArtifactToLocalRef(art artifact.Artifact, runtimeName string) (lucy.LocalArtifactRef, error) {
	ref := lucy.LocalArtifactRef{
		ArtifactID:       art.ID,
		PayloadAlgorithm: art.PayloadAlgorithm,
		PayloadHash:      art.SHA256,
		PayloadSize:      art.SizeBytes,
		ArtifactType:     string(art.Type),
		RuntimeName:      runtimeName,
		Metadata:         make(map[string]string),
	}
	if err := ref.Validate(); err != nil {
		return lucy.LocalArtifactRef{}, err
	}
	return ref, nil
}
