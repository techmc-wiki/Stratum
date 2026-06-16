package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCobraRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "StratumMC collaborative testing control plane CLI") || !strings.Contains(stdout.String(), "sessions") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
