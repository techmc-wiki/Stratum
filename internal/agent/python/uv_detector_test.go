package python

import (
	"context"
	"strings"
	"testing"
)

func TestUVDetector_Detect(t *testing.T) {
	ctx := context.Background()
	detector := NewUVDetector()

	version, err := detector.Detect(ctx)
	if err != nil {
		t.Skipf("uv not available: %v", err)
	}

	if version == "" {
		t.Error("version should not be empty")
	}

	if !strings.Contains(strings.ToLower(version), "uv") {
		t.Errorf("unexpected version format: %q", version)
	}

	t.Logf("detected uv: %s", version)
}

func TestUVDetector_IsAvailable(t *testing.T) {
	ctx := context.Background()
	detector := NewUVDetector()

	available := detector.IsAvailable(ctx)
	t.Logf("uv available: %v", available)
}
