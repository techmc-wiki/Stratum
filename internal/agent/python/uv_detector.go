package python

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type UVDetector struct{}

func NewUVDetector() *UVDetector {
	return &UVDetector{}
}

func (d *UVDetector) IsAvailable(ctx context.Context) bool {
	_, err := d.Detect(ctx)
	return err == nil
}

func (d *UVDetector) Detect(ctx context.Context) (string, error) {
	path, err := exec.LookPath("uv")
	if err != nil {
		return "", fmt.Errorf("uv not found in PATH: install from https://docs.astral.sh/uv/getting-started/installation/")
	}
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("uv found but unable to execute: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("uv version detection failed")
	}
	return version, nil
}
