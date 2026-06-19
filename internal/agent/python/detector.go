package python

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	MinimumMCDRMajor = 3
	MinimumMCDRMinor = 9
)

var pythonVersionPattern = regexp.MustCompile(`(?m)Python\s+(\d+)\.(\d+)\.(\d+)`)

type Installation struct {
	Version        string   `json:"version"`
	Major          int      `json:"major"`
	Minor          int      `json:"minor"`
	Patch          int      `json:"patch"`
	ExecutablePath string   `json:"executablePath"`
	PrefixArgs     []string `json:"prefixArgs,omitempty"`
	Source         string   `json:"source,omitempty"`
	HasVenv        bool     `json:"hasVenv"`
	HasPip         bool     `json:"hasPip"`
}

func (i Installation) CommandArgv(extra ...string) []string {
	argv := make([]string, 0, 1+len(i.PrefixArgs)+len(extra))
	argv = append(argv, i.ExecutablePath)
	argv = append(argv, i.PrefixArgs...)
	argv = append(argv, extra...)
	return argv
}

type Detector struct {
	LookPath       func(string) (string, error)
	Run            func(context.Context, string, ...string) (string, error)
	Getenv         func(string) string
	Candidates     []Candidate
	ExecutableEnvs []string
	MinimumMajor   int
	MinimumMinor   int
}

type Candidate struct {
	Name       string
	PrefixArgs []string
	Source     string
}

func NewDetector() *Detector {
	return &Detector{
		LookPath:       exec.LookPath,
		Run:            defaultRun,
		Getenv:         os.Getenv,
		Candidates:     defaultCandidates(),
		ExecutableEnvs: []string{"STRATUM_PYTHON", "MCDR_PYTHON", "PYTHON"},
		MinimumMajor:   MinimumMCDRMajor,
		MinimumMinor:   MinimumMCDRMinor,
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
	run := d.Run
	if run == nil {
		run = defaultRun
	}
	getenv := d.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	candidates := append([]resolvedCandidate(nil), d.envCandidates(getenv)...)
	for _, item := range d.commandCandidates() {
		path, err := lookPath(item.Name)
		if err == nil && strings.TrimSpace(path) != "" {
			source := item.Source
			if source == "" {
				source = "path:" + item.Name
			}
			candidates = append(candidates, resolvedCandidate{path: path, prefixArgs: item.PrefixArgs, source: source})
		}
	}

	seen := map[string]struct{}{}
	installations := make([]Installation, 0, len(candidates))
	var problems []string
	for _, item := range candidates {
		path := strings.TrimSpace(item.path)
		if path == "" {
			continue
		}
		key := strings.ToLower(path + "\x00" + strings.Join(item.prefixArgs, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		versionOutput, err := run(ctx, path, append(item.prefixArgs, "--version")...)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", item.label(), err))
			continue
		}
		version, major, minor, patch, err := ParseVersionOutput(versionOutput)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", item.label(), err))
			continue
		}

		installation := Installation{
			Version:        version,
			Major:          major,
			Minor:          minor,
			Patch:          patch,
			ExecutablePath: path,
			PrefixArgs:     append([]string(nil), item.prefixArgs...),
			Source:         item.source,
		}
		installation.HasVenv = moduleAvailable(ctx, run, installation, "venv")
		installation.HasPip = moduleAvailable(ctx, run, installation, "pip")
		installations = append(installations, installation)
	}

	sort.SliceStable(installations, func(i, j int) bool {
		left, right := installations[i], installations[j]
		if left.Major != right.Major {
			return left.Major < right.Major
		}
		if left.Minor != right.Minor {
			return left.Minor < right.Minor
		}
		if left.Patch != right.Patch {
			return left.Patch < right.Patch
		}
		return strings.Join(left.CommandArgv(), " ") < strings.Join(right.CommandArgv(), " ")
	})
	if len(installations) == 0 {
		if len(problems) > 0 {
			return nil, fmt.Errorf("no usable Python installation found: %s", strings.Join(problems, "; "))
		}
		return nil, errors.New("no Python installation found")
	}
	return installations, nil
}

func (d *Detector) SelectForMCDR(ctx context.Context) (Installation, error) {
	minimumMajor, minimumMinor := d.minimumVersion()
	installations, err := d.DetectInstallations(ctx)
	if err != nil {
		return Installation{}, err
	}
	for _, item := range installations {
		if !SatisfiesMinimum(item.Major, item.Minor, minimumMajor, minimumMinor) {
			continue
		}
		if !item.HasVenv {
			continue
		}
		if !item.HasPip {
			continue
		}
		return item, nil
	}
	return Installation{}, fmt.Errorf("no Python installation satisfies MCDR requirements: >=%d.%d with venv and pip", minimumMajor, minimumMinor)
}

func ParseVersionOutput(output string) (string, int, int, int, error) {
	match := pythonVersionPattern.FindStringSubmatch(output)
	if len(match) != 4 {
		return "", 0, 0, 0, fmt.Errorf("Python version output does not contain a version string")
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	return version, major, minor, patch, nil
}

func SatisfiesMinimum(major, minor, minimumMajor, minimumMinor int) bool {
	if major != minimumMajor {
		return major > minimumMajor
	}
	return minor >= minimumMinor
}

type resolvedCandidate struct {
	path       string
	prefixArgs []string
	source     string
}

func (c resolvedCandidate) label() string {
	if len(c.prefixArgs) == 0 {
		return c.path
	}
	return c.path + " " + strings.Join(c.prefixArgs, " ")
}

func (d *Detector) envCandidates(getenv func(string) string) []resolvedCandidate {
	vars := d.ExecutableEnvs
	if len(vars) == 0 {
		vars = []string{"STRATUM_PYTHON", "MCDR_PYTHON", "PYTHON"}
	}
	items := make([]resolvedCandidate, 0, len(vars))
	for _, name := range vars {
		path := strings.TrimSpace(getenv(name))
		if path == "" {
			continue
		}
		items = append(items, resolvedCandidate{path: path, source: "env:" + name})
	}
	return items
}

func (d *Detector) commandCandidates() []Candidate {
	if len(d.Candidates) > 0 {
		return append([]Candidate(nil), d.Candidates...)
	}
	return defaultCandidates()
}

func (d *Detector) minimumVersion() (int, int) {
	major, minor := d.MinimumMajor, d.MinimumMinor
	if major == 0 {
		major = MinimumMCDRMajor
	}
	if minor == 0 && major == MinimumMCDRMajor {
		minor = MinimumMCDRMinor
	}
	return major, minor
}

func defaultCandidates() []Candidate {
	return []Candidate{
		{Name: "python3"},
		{Name: "python"},
		{Name: "python3.13"},
		{Name: "python3.12"},
		{Name: "python3.11"},
		{Name: "python3.10"},
		{Name: "python3.9"},
		{Name: "py", PrefixArgs: []string{"-3"}, Source: "path:py -3"},
	}
}

func moduleAvailable(ctx context.Context, run func(context.Context, string, ...string) (string, error), installation Installation, module string) bool {
	args := append([]string{}, installation.PrefixArgs...)
	args = append(args, "-m", module, "--version")
	_, err := run(ctx, installation.ExecutablePath, args...)
	return err == nil
}

func defaultRun(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}
