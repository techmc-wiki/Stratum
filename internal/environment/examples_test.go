package environment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGTMCEnvironmentExamples(t *testing.T) {
	tests := []struct {
		name string
		file string
		id   string
	}{
		{name: "1.12", file: "gtmc-1.12.example.json", id: "gtmc-1-12"},
		{name: "1.17", file: "gtmc-1.17.example.json", id: "gtmc-1-17"},
		{name: "latest", file: "gtmc-latest.example.json", id: "gtmc-latest"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "docs", "environments", test.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example %q: %v", path, err)
			}

			var example Environment
			if err := json.Unmarshal(data, &example); err != nil {
				t.Fatalf("parse example: %v", err)
			}
			if example.ID != test.id {
				t.Fatalf("id = %q, want %q", example.ID, test.id)
			}
			if example.Metadata["exampleOnly"] != "true" {
				t.Fatalf("exampleOnly = %q", example.Metadata["exampleOnly"])
			}
			if err := example.Validate(); err != nil {
				t.Fatalf("validate example: %v", err)
			}
		})
	}
}
