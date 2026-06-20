package python

import (
	"context"
	"os"
	"testing"
)

func TestUVDetector_DetectWhenUnavailable(t *testing.T) {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	ctx := context.Background()
	detector := NewUVDetector()

	_, err := detector.Detect(ctx)
	if err == nil {
		t.Error("expected error when uv is not in PATH")
	}

	if err != nil && !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	if err != nil && !contains(err.Error(), "https://") {
		t.Errorf("error should include installation URL: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
