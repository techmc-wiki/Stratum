package fileops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicWritesPayloadAndPermissions(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", "file.txt")
	if err := WriteFileAtomic(target, []byte("payload"), 0o640, 0o750, ".test-*.tmp"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload" {
		t.Fatalf("payload=%q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("permissions=%#o, want 0640", got)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".test-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestWriteJSONAtomicWritesIndentedJSON(t *testing.T) {
	target := filepath.Join(t.TempDir(), "manifest.json")
	value := struct {
		Name string `json:"name"`
	}{Name: "test"}
	if err := WriteJSONAtomic(target, value, 0o640, 0o750, ".json-*.tmp"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"name\": \"test\"\n}\n" {
		t.Fatalf("JSON payload=%q", data)
	}
	var decoded struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != value.Name {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestSHA256Hex(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(target, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := SHA256Hex(target)
	if err != nil {
		t.Fatal(err)
	}
	digestWithSize, size, err := SHA256HexWithSize(target)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5" {
		t.Fatalf("digest=%s", digest)
	}
	if digestWithSize != digest || size != int64(len("payload")) {
		t.Fatalf("digestWithSize=%s size=%d", digestWithSize, size)
	}
}
