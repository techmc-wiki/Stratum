package worldcheckpoint

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const AgentLocalScheme = "agent-local://"

func BuildAgentLocalSnapshotRef(agentID, sessionID, snapshotPath, runtimeRoot string) (string, error) {
	if agentID == "" {
		return "", errors.New("agent ID must not be empty")
	}
	if strings.ContainsAny(agentID, "/\\\x00") {
		return "", errors.New("agent ID must not contain path separators or NUL")
	}
	if sessionID == "" {
		return "", errors.New("session ID must not be empty")
	}
	if strings.ContainsAny(sessionID, "/\\\x00") {
		return "", errors.New("session ID must not contain path separators or NUL")
	}
	if strings.TrimSpace(snapshotPath) == "" {
		return "", errors.New("snapshot path must not be empty")
	}
	absRuntime, err := filepath.Abs(filepath.Clean(strings.TrimSpace(runtimeRoot)))
	if err != nil {
		return "", fmt.Errorf("resolve runtime root: %w", err)
	}
	absSnapshot, err := filepath.Abs(filepath.Clean(snapshotPath))
	if err != nil {
		return "", fmt.Errorf("resolve snapshot path: %w", err)
	}
	if !pathWithin(absRuntime, absSnapshot) {
		return "", fmt.Errorf("snapshot path %q escapes runtime root", snapshotPath)
	}
	rel, err := filepath.Rel(absRuntime, absSnapshot)
	if err != nil {
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "..") {
		return "", errors.New("relative path must not contain ..")
	}
	if rel == "." {
		return "", errors.New("snapshot path must not be the runtime root")
	}
	return AgentLocalScheme + agentID + "/" + rel, nil
}

func ParseAgentLocalSnapshotRef(ref string) (agentID, sessionID, relativePath string, err error) {
	if !strings.HasPrefix(ref, AgentLocalScheme) {
		return "", "", "", fmt.Errorf("not an agent-local snapshot ref: %q", ref)
	}
	rest := ref[len(AgentLocalScheme):]
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return "", "", "", fmt.Errorf("invalid agent-local ref format: %q", ref)
	}
	agentID = rest[:idx]
	relativePath = rest[idx+1:]
	parts := strings.SplitN(relativePath, "/", 3)
	if len(parts) >= 2 && parts[0] == "sessions" {
		sessionID = parts[1]
	}
	if agentID == "" || relativePath == "" {
		return "", "", "", fmt.Errorf("invalid agent-local ref format: %q", ref)
	}
	return agentID, sessionID, relativePath, nil
}
