package safepath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithin(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		candidate string
		want      bool
	}{
		{name: "same path", root: "/a", candidate: "/a", want: true},
		{name: "child path", root: "/a", candidate: "/a/b", want: true},
		{name: "sibling prefix", root: "/a", candidate: "/aa", want: false},
		{name: "parent path", root: "/a/b", candidate: "/a", want: false},
		{name: "cleaned escape", root: "/a", candidate: "/a/b/../../b", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Within(tt.root, tt.candidate); got != tt.want {
				t.Fatalf("Within(%q, %q) = %t, want %t", tt.root, tt.candidate, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "file", input: "mods/carpet.jar"},
		{name: "backslash normalized", input: `mods\carpet.jar`},
		{name: "empty", input: "", wantErr: "relative"},
		{name: "absolute", input: filepath.Join(root, "mods"), wantErr: "relative"},
		{name: "parent", input: "../escape", wantErr: "escapes"},
		{name: "windows volume", input: `C:\escape`, wantErr: "relative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := Resolve(root, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Resolve(%q) err=%v, want %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tt.input, err)
			}
			if !Within(root, path) {
				t.Fatalf("resolved path escapes root: %q", path)
			}
		})
	}
}

func TestRejectSymlinkPath(t *testing.T) {
	root := t.TempDir()
	safeDir := filepath.Join(root, "safe")
	if err := os.MkdirAll(safeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RejectSymlinkPath(root, filepath.Join(safeDir, "missing.txt"), "test path"); err != nil {
		t.Fatalf("safe missing leaf rejected: %v", err)
	}
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := RejectSymlinkPath(root, filepath.Join(link, "file.txt"), "test path"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symlink rejection: %v", err)
	}
}
