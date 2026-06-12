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

func TestValidateType(t *testing.T) {
	for _, value := range []Type{TypeJar, TypeDatapack, TypeMCDRPlugin, TypeConfigPreset, TypeCarpetRules, TypeSchematic, TypeWorldArchive} {
		if err := ValidateType(value); err != nil {
			t.Fatalf("type %q: %v", value, err)
		}
	}
	if err := ValidateType("binary"); err == nil {
		t.Fatal("invalid artifact type should fail")
	}
}
