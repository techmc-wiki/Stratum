package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactHashMetadata(t *testing.T) {
	data := []byte("stratum artifact")
	path := filepath.Join(t.TempDir(), "artifact.jar")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	hash, size, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash != HashBytes(data) {
		t.Fatalf("file hash %q differs from byte hash %q", hash, HashBytes(data))
	}
	if size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", size, len(data))
	}
}
