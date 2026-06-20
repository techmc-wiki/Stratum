package python

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		output string
		want   string
		major  int
		minor  int
		patch  int
	}{
		{output: "Python 3.9.18", want: "3.9.18", major: 3, minor: 9, patch: 18},
		{output: "Python 3.12.2\n", want: "3.12.2", major: 3, minor: 12, patch: 2},
	}
	for _, test := range tests {
		version, major, minor, patch, err := ParseVersionOutput(test.output)
		if err != nil {
			t.Fatalf("ParseVersionOutput(%q): %v", test.output, err)
		}
		if version != test.want || major != test.major || minor != test.minor || patch != test.patch {
			t.Fatalf("ParseVersionOutput(%q)=(%q,%d,%d,%d)", test.output, version, major, minor, patch)
		}
	}
}

func TestSatisfiesMinimum(t *testing.T) {
	tests := []struct {
		major int
		minor int
		want  bool
	}{
		{major: 3, minor: 8, want: false},
		{major: 3, minor: 9, want: true},
		{major: 3, minor: 12, want: true},
		{major: 4, minor: 0, want: true},
	}
	for _, test := range tests {
		got := SatisfiesMinimum(test.major, test.minor, 3, 9)
		if got != test.want {
			t.Fatalf("SatisfiesMinimum(%d,%d,3,9)=%t want %t", test.major, test.minor, got, test.want)
		}
	}
}

func TestDetectInstallationsChecksVenvAndPip(t *testing.T) {
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if name == "python3" {
				return "/usr/bin/python3", nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, path string, args ...string) (string, error) {
			joined := strings.Join(append([]string{path}, args...), " ")
			switch joined {
			case "/usr/bin/python3 --version":
				return "Python 3.11.5", nil
			case "/usr/bin/python3 -m venv --help":
				return "", nil
			case "/usr/bin/python3 -m pip --help":
				return "pip 24.0", nil
			default:
				return "", errors.New("unexpected command: " + joined)
			}
		},
		Getenv:         func(string) string { return "" },
		Candidates:     []Candidate{{Name: "python3"}},
		ExecutableEnvs: []string{},
	}

	installations, err := detector.DetectInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 {
		t.Fatalf("installations=%+v", installations)
	}
	got := installations[0]
	if got.Version != "3.11.5" || got.Major != 3 || got.Minor != 11 || !got.HasVenv || !got.HasPip {
		t.Fatalf("installation=%+v", got)
	}
}

func TestSelectForMCDRChoosesLowestSatisfyingPython(t *testing.T) {
	versions := map[string]string{
		"python38": "Python 3.8.18",
		"python39": "Python 3.9.18",
		"python12": "Python 3.12.2",
	}
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if _, ok := versions[name]; ok {
				return name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, path string, args ...string) (string, error) {
			if reflect.DeepEqual(args, []string{"--version"}) {
				return versions[path], nil
			}
			if reflect.DeepEqual(args, []string{"-m", "venv", "--help"}) || reflect.DeepEqual(args, []string{"-m", "pip", "--help"}) {
				return "ok", nil
			}
			return "", errors.New("unexpected args")
		},
		Getenv:         func(string) string { return "" },
		Candidates:     []Candidate{{Name: "python12"}, {Name: "python38"}, {Name: "python39"}},
		ExecutableEnvs: []string{},
	}

	selected, err := detector.SelectForMCDR(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selected.ExecutablePath != "python39" || selected.Major != 3 || selected.Minor != 9 {
		t.Fatalf("selected=%+v", selected)
	}
}

func TestSelectForMCDRRequiresVenvAndPip(t *testing.T) {
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if name == "python3" {
				return name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, _ string, args ...string) (string, error) {
			switch strings.Join(args, " ") {
			case "--version":
				return "Python 3.11.5", nil
			case "-m venv --help":
				return "", nil
			case "-m pip --help":
				return "", errors.New("pip missing")
			default:
				return "", errors.New("unexpected command")
			}
		},
		Getenv:         func(string) string { return "" },
		Candidates:     []Candidate{{Name: "python3"}},
		ExecutableEnvs: []string{},
	}

	_, err := detector.SelectForMCDR(context.Background())
	if err == nil || !strings.Contains(err.Error(), "venv and pip") {
		t.Fatalf("expected venv/pip requirement error, got %v", err)
	}
}

func TestPyLauncherCandidateUsesPrefixArgs(t *testing.T) {
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if name == "py" {
				return "py", nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, _ string, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			switch joined {
			case "-3 --version":
				return "Python 3.11.5", nil
			case "-3 -m venv --help", "-3 -m pip --help":
				return "ok", nil
			default:
				return "", errors.New("unexpected args: " + joined)
			}
		},
		Getenv:         func(string) string { return "" },
		Candidates:     []Candidate{{Name: "py", PrefixArgs: []string{"-3"}}},
		ExecutableEnvs: []string{},
	}

	selected, err := detector.SelectForMCDR(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.CommandArgv("-m", "venv", "venv"), []string{"py", "-3", "-m", "venv", "venv"}) {
		t.Fatalf("unexpected argv: %+v", selected.CommandArgv("-m", "venv", "venv"))
	}
}
