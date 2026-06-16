package mcdrbridge

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func validateSessionID(id string) error {
	if id == "" || id == "." || id == ".." {
		return fmt.Errorf("session id is required")
	}
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("session id %q contains unsupported characters", id)
	}
	return nil
}

func validateRuntimeRelativePath(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("%s must use forward-slash runtime-relative paths", field)
	}
	if filepath.IsAbs(value) || path.IsAbs(value) || hasWindowsVolumePrefix(value) {
		return fmt.Errorf("%s must be runtime-relative", field)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s contains an unsafe or non-canonical path", field)
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func stringsContains(s, substr string) bool {
	return strings.Index(s, substr) >= 0
}
