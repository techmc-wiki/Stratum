package java

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestMajorFromVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{version: "1.8.0_402", want: 8},
		{version: "16.0.2", want: 16},
		{version: "17.0.10+7", want: 17},
		{version: "21.0.2", want: 21},
	}
	for _, test := range tests {
		got, err := MajorFromVersion(test.version)
		if err != nil {
			t.Fatalf("MajorFromVersion(%q): %v", test.version, err)
		}
		if got != test.want {
			t.Fatalf("MajorFromVersion(%q)=%d want %d", test.version, got, test.want)
		}
	}
}

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		output      string
		wantVersion string
		wantMajor   int
	}{
		{output: `openjdk version "17.0.10" 2024-01-16`, wantVersion: "17.0.10", wantMajor: 17},
		{output: `java version "1.8.0_402"`, wantVersion: "1.8.0_402", wantMajor: 8},
	}
	for _, test := range tests {
		version, major, err := ParseVersionOutput(test.output)
		if err != nil {
			t.Fatalf("ParseVersionOutput(%q): %v", test.output, err)
		}
		if version != test.wantVersion || major != test.wantMajor {
			t.Fatalf("ParseVersionOutput(%q)=(%q,%d) want (%q,%d)", test.output, version, major, test.wantVersion, test.wantMajor)
		}
	}
}

func TestRequiredMajorForMinecraftVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{version: "1.5.2", want: 5},
		{version: "1.11.2", want: 6},
		{version: "1.12", want: 8},
		{version: "1.16.5", want: 8},
		{version: "1.17.1", want: 16},
		{version: "1.18", want: 17},
		{version: "1.20.4", want: 17},
		{version: "1.20.5", want: 21},
		{version: "1.21.11", want: 21},
		{version: "26.1", want: 25},
	}
	for _, test := range tests {
		got, err := RequiredMajorForMinecraftVersion(test.version)
		if err != nil {
			t.Fatalf("RequiredMajorForMinecraftVersion(%q): %v", test.version, err)
		}
		if got != test.want {
			t.Fatalf("RequiredMajorForMinecraftVersion(%q)=%d want %d", test.version, got, test.want)
		}
	}
}

func TestDetectInstallationsUsesEnvAndPathCandidates(t *testing.T) {
	java8Home := filepath.Join("javas", "java8")
	java8 := javaExecutableInHome(java8Home)
	java17 := filepath.Join("javas", "java17", "bin", "java")
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if name == "java" {
				return java17, nil
			}
			return "", errors.New("not found")
		},
		RunVersion: func(_ context.Context, path string) (string, error) {
			switch path {
			case java8:
				return `openjdk version "1.8.0_402"`, nil
			case java17:
				return `openjdk version "17.0.10"`, nil
			default:
				return "", fmt.Errorf("unexpected path %s", path)
			}
		},
		Getenv: func(name string) string {
			if name == "JAVA_8_HOME" {
				return java8Home
			}
			return ""
		},
		Candidates:    []string{"java"},
		HomeVariables: []string{"JAVA_8_HOME"},
	}

	installations, err := detector.DetectInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 2 {
		t.Fatalf("installations=%+v", installations)
	}
	if installations[0].Major != 8 || installations[1].Major != 17 {
		t.Fatalf("installations not sorted by major version: %+v", installations)
	}
}

func TestSelectForVersionChoosesLowestSatisfyingJava(t *testing.T) {
	versions := map[string]string{
		"java8":  `openjdk version "1.8.0_402"`,
		"java17": `openjdk version "17.0.10"`,
		"java21": `openjdk version "21.0.2"`,
	}
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if _, ok := versions[name]; ok {
				return name, nil
			}
			return "", errors.New("not found")
		},
		RunVersion: func(_ context.Context, path string) (string, error) {
			return versions[path], nil
		},
		Getenv:        func(string) string { return "" },
		Candidates:    []string{"java21", "java8", "java17"},
		HomeVariables: []string{},
	}

	selected, err := detector.SelectForVersion(context.Background(), 16)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Major != 17 || selected.ExecutablePath != "java17" {
		t.Fatalf("selected=%+v", selected)
	}
}

func TestSelectForMinecraftVersion(t *testing.T) {
	detector := &Detector{
		LookPath: func(name string) (string, error) {
			if name == "java16" {
				return name, nil
			}
			return "", errors.New("not found")
		},
		RunVersion:    func(context.Context, string) (string, error) { return `openjdk version "16.0.2"`, nil },
		Getenv:        func(string) string { return "" },
		Candidates:    []string{"java16"},
		HomeVariables: []string{},
	}

	selected, err := detector.SelectForMinecraftVersion(context.Background(), "1.17.1")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Major != 16 {
		t.Fatalf("selected=%+v", selected)
	}
}
