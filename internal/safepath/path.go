package safepath

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func Within(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func Resolve(root, relativePath string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(relativePath), `\`, "/")
	if normalized == "" || path.IsAbs(normalized) || filepath.IsAbs(filepath.FromSlash(normalized)) || HasWindowsVolumePrefix(normalized) {
		return "", errors.New("path must be relative")
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes root")
	}
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.FromSlash(clean))
	if !Within(root, candidate) {
		return "", errors.New("path escapes root")
	}
	return candidate, nil
}

func RejectSymlinkPath(root, candidate, message string) error {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if !Within(root, candidate) {
		return errors.New(message + " escapes root")
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", message, err)
	}
	current := root
	if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New(message + " contains a symbolic link")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", message, err)
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", message, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New(message + " contains a symbolic link")
		}
	}
	return nil
}

func HasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}
