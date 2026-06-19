package java

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`(?m)(?:openjdk|java) version "([^"]+)"`)

type Installation struct {
	Version        string `json:"version"`
	Major          int    `json:"major"`
	ExecutablePath string `json:"executablePath"`
	Home           string `json:"home,omitempty"`
	Source         string `json:"source,omitempty"`
}

type Detector struct {
	LookPath      func(string) (string, error)
	RunVersion    func(context.Context, string) (string, error)
	Getenv        func(string) string
	Candidates    []string
	HomeVariables []string
}

func NewDetector() *Detector {
	return &Detector{
		LookPath:   exec.LookPath,
		RunVersion: defaultRunVersion,
		Getenv:     os.Getenv,
		Candidates: defaultCandidateCommands(),
		HomeVariables: []string{
			"JAVA_HOME",
			"JAVA_25_HOME",
			"JAVA_21_HOME",
			"JAVA_17_HOME",
			"JAVA_16_HOME",
			"JAVA_11_HOME",
			"JAVA_8_HOME",
		},
	}
}

func (d *Detector) DetectInstallations(ctx context.Context) ([]Installation, error) {
	if d == nil {
		d = NewDetector()
	}
	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runVersion := d.RunVersion
	if runVersion == nil {
		runVersion = defaultRunVersion
	}
	getenv := d.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	candidates := append([]candidate(nil), d.homeCandidates(getenv)...)
	for _, name := range d.commandCandidates() {
		path, err := lookPath(name)
		if err == nil && strings.TrimSpace(path) != "" {
			candidates = append(candidates, candidate{path: path, source: "path:" + name})
		}
	}

	seen := map[string]struct{}{}
	installations := make([]Installation, 0, len(candidates))
	var problems []string
	for _, item := range candidates {
		path := filepath.Clean(item.path)
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		output, err := runVersion(ctx, path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		version, major, err := ParseVersionOutput(output)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		installations = append(installations, Installation{
			Version:        version,
			Major:          major,
			ExecutablePath: path,
			Home:           homeFromExecutable(path),
			Source:         item.source,
		})
	}

	sort.SliceStable(installations, func(i, j int) bool {
		if installations[i].Major != installations[j].Major {
			return installations[i].Major < installations[j].Major
		}
		return installations[i].ExecutablePath < installations[j].ExecutablePath
	})
	if len(installations) == 0 {
		if len(problems) > 0 {
			return nil, fmt.Errorf("no usable Java installation found: %s", strings.Join(problems, "; "))
		}
		return nil, errors.New("no Java installation found")
	}
	return installations, nil
}

func (d *Detector) SelectForVersion(ctx context.Context, minimumMajor int) (Installation, error) {
	if minimumMajor <= 0 {
		return Installation{}, fmt.Errorf("minimum Java major version must be positive")
	}
	installations, err := d.DetectInstallations(ctx)
	if err != nil {
		return Installation{}, err
	}
	for _, item := range installations {
		if item.Major >= minimumMajor {
			return item, nil
		}
	}
	return Installation{}, fmt.Errorf("no Java installation satisfies minimum major version %d", minimumMajor)
}

func (d *Detector) SelectForMinecraftVersion(ctx context.Context, minecraftVersion string) (Installation, error) {
	major, err := RequiredMajorForMinecraftVersion(minecraftVersion)
	if err != nil {
		return Installation{}, err
	}
	return d.SelectForVersion(ctx, major)
}

func ParseVersionOutput(output string) (string, int, error) {
	match := versionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", 0, fmt.Errorf("Java version output does not contain a version string")
	}
	version := strings.TrimSpace(match[1])
	major, err := MajorFromVersion(version)
	if err != nil {
		return "", 0, err
	}
	return version, major, nil
}

func MajorFromVersion(version string) (int, error) {
	parts := strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+' || r == ' '
	})
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return 0, fmt.Errorf("invalid Java version %q", version)
	}
	first, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid Java version %q", version)
	}
	if first == 1 {
		if len(parts) < 2 {
			return 0, fmt.Errorf("invalid legacy Java version %q", version)
		}
		legacy, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid legacy Java version %q", version)
		}
		return legacy, nil
	}
	return first, nil
}

func RequiredMajorForMinecraftVersion(version string) (int, error) {
	major, minor, patch, err := parseMinecraftVersion(version)
	if err != nil {
		return 0, err
	}
	if major >= 26 {
		return 25, nil
	}
	if major != 1 {
		return 0, fmt.Errorf("unsupported Minecraft version %q", version)
	}
	switch {
	case minor <= 5:
		return 5, nil
	case minor <= 11:
		return 6, nil
	case minor <= 16:
		return 8, nil
	case minor == 17:
		return 16, nil
	case minor == 18 || minor == 19:
		return 17, nil
	case minor == 20 && patch <= 4:
		return 17, nil
	case minor == 20 && patch >= 5:
		return 21, nil
	case minor == 21:
		return 21, nil
	default:
		return 21, nil
	}
}

type candidate struct {
	path   string
	source string
}

func (d *Detector) homeCandidates(getenv func(string) string) []candidate {
	vars := d.HomeVariables
	if len(vars) == 0 {
		vars = []string{"JAVA_HOME"}
	}
	items := make([]candidate, 0, len(vars))
	for _, name := range vars {
		home := strings.TrimSpace(getenv(name))
		if home == "" {
			continue
		}
		items = append(items, candidate{path: javaExecutableInHome(home), source: "env:" + name})
	}
	return items
}

func (d *Detector) commandCandidates() []string {
	if len(d.Candidates) > 0 {
		return append([]string(nil), d.Candidates...)
	}
	return defaultCandidateCommands()
}

func defaultCandidateCommands() []string {
	return []string{"java", "java25", "java21", "java17", "java16", "java11", "java8"}
}

func javaExecutableInHome(home string) string {
	exe := "java"
	if runtime.GOOS == "windows" {
		exe = "java.exe"
	}
	return filepath.Join(home, "bin", exe)
}

func homeFromExecutable(path string) string {
	bin := filepath.Dir(path)
	if filepath.Base(bin) != "bin" {
		return ""
	}
	return filepath.Dir(bin)
}

func defaultRunVersion(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func parseMinecraftVersion(version string) (int, int, int, error) {
	value := strings.TrimSpace(version)
	if value == "" {
		return 0, 0, 0, fmt.Errorf("Minecraft version is required")
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("unsupported Minecraft version %q", version)
	}
	major, err := strconv.Atoi(numericPrefix(parts[0]))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("unsupported Minecraft version %q", version)
	}
	minor, err := strconv.Atoi(numericPrefix(parts[1]))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("unsupported Minecraft version %q", version)
	}
	patch := 0
	if len(parts) > 2 {
		patchText := numericPrefix(parts[2])
		if patchText != "" {
			patch, err = strconv.Atoi(patchText)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("unsupported Minecraft version %q", version)
			}
		}
	}
	return major, minor, patch, nil
}

func numericPrefix(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
