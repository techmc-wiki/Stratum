package runtimeprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const maxConfigBytes = 1 << 20

type configDocument struct {
	RuntimeProfiles []profileConfig `json:"runtime_profiles"`
}

type profileConfig struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	RuntimeType         Type              `json:"runtime_type"`
	CommandArgv         []string          `json:"command_argv"`
	WorkingDir          string            `json:"working_dir"`
	Env                 map[string]string `json:"env"`
	StopStrategy        StopStrategy      `json:"stop_strategy"`
	StopStdinCommand    string            `json:"stop_stdin_command"`
	GracefulStopTimeout string            `json:"graceful_stop_timeout"`
	ForceKillTimeout    string            `json:"force_kill_timeout"`
	LogMode             LogMode           `json:"log_mode"`
	Enabled             bool              `json:"enabled"`
	Notes               string            `json:"notes"`
}

func LoadTrustedFile(path string) ([]Profile, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("runtime profile config path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open runtime profile config %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat runtime profile config %q: %w", path, err)
	}
	if info.Size() > maxConfigBytes {
		return nil, fmt.Errorf("runtime profile config %q exceeds %d bytes", path, maxConfigBytes)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read runtime profile config %q: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document configDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode runtime profile config %q: %w", path, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode runtime profile config %q: %w", path, err)
	}

	profiles := make([]Profile, 0, len(document.RuntimeProfiles))
	seen := make(map[string]struct{}, len(document.RuntimeProfiles))
	for index, configured := range document.RuntimeProfiles {
		profile, err := configured.profile()
		if err != nil {
			return nil, fmt.Errorf("runtime_profiles[%d] %q: %w", index, configured.ID, err)
		}
		if err := Validate(profile); err != nil {
			return nil, fmt.Errorf("runtime_profiles[%d] %q: %w", index, configured.ID, err)
		}
		if _, exists := seen[profile.ID]; exists {
			return nil, fmt.Errorf("runtime_profiles[%d] %q duplicates an earlier profile", index, profile.ID)
		}
		seen[profile.ID] = struct{}{}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("config must contain exactly one JSON document")
}

func (value profileConfig) profile() (Profile, error) {
	graceful, err := parseConfigDuration("graceful_stop_timeout", value.GracefulStopTimeout)
	if err != nil {
		return Profile{}, err
	}
	force, err := parseConfigDuration("force_kill_timeout", value.ForceKillTimeout)
	if err != nil {
		return Profile{}, err
	}
	return Profile{
		ID:                  value.ID,
		Name:                value.Name,
		RuntimeType:         value.RuntimeType,
		CommandArgv:         value.CommandArgv,
		WorkingDir:          value.WorkingDir,
		Env:                 value.Env,
		StopStrategy:        value.StopStrategy,
		StopStdinCommand:    value.StopStdinCommand,
		GracefulStopTimeout: graceful,
		ForceKillTimeout:    force,
		LogMode:             value.LogMode,
		Enabled:             value.Enabled,
		Notes:               value.Notes,
	}, nil
}

func parseConfigDuration(field, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	return duration, nil
}
