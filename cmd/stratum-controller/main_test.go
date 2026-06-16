package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandPrintsMVPStub(t *testing.T) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "stratum-controller MVP stub") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
