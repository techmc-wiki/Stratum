package main

import (
	"errors"
	"strings"
	"testing"
)

func TestServeRejectsUnsupportedRuntimeMode(t *testing.T) {
	err := serve("127.0.0.1:0", "", "shell", t.TempDir(), "")
	var usage usageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), `unsupported runtime mode "shell"`) {
		t.Fatalf("err=%v", err)
	}
}
