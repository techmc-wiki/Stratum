package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandShowsHelp(t *testing.T) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "serve") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
