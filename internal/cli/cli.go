package cli

import (
	"io"
)

const (
	defaultDataDirectory    = ".stratum/data"
	defaultArtifactBlobRoot = ".stratum/artifacts"
)

func Run(args []string, stdout, stderr io.Writer) int {
	return runCobra(args, stdout, stderr)
}
