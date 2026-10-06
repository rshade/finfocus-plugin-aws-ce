package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeDevelopmentTargets(t *testing.T) {
	root := filepath.Join("..", "..")
	temp := t.TempDir()
	fakeGo := filepath.Join(temp, "go")
	marker := filepath.Join(temp, "calls")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MAKE_TEST_LOG\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		target   string
		commands []string
	}{
		{"test", []string{"test -count=1 -v ./..."}},
		{"test-integration", []string{"test -count=1 -v ./test/integration/... ./test/conformance/..."}},
		{"develop", []string{"mod tidy", "mod download"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			if err := os.WriteFile(marker, nil, 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("make", "-C", root, tc.target, "GO="+fakeGo)
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if key != "PATH" && key != "MAKE_TEST_LOG" {
					command.Env = append(command.Env, entry)
				}
			}
			command.Env = append(command.Env, "PATH="+temp+string(os.PathListSeparator)+os.Getenv("PATH"), "MAKE_TEST_LOG="+marker)
			if out, err := command.CombinedOutput(); err != nil {
				t.Fatalf("make %s: %v\n%s", tc.target, err, out)
			}
			calls, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.commands {
				if !strings.Contains(string(calls), want+"\n") {
					t.Fatalf("recipe did not execute %q, calls=%q", want, calls)
				}
			}
		})
	}
}

func TestMakeInstallLocal(t *testing.T) {
	temp := t.TempDir()
	buildDir := filepath.Join(temp, "bin")
	home := filepath.Join(temp, "finfocus")
	command := exec.Command("make", "-C", filepath.Join("..", ".."), "install-local", "BUILD_DIR="+buildDir, "FINFOCUS_HOME="+home)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("make install-local: %v\n%s", err, out)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Metadata.Version == "" {
		t.Fatal("manifest version is empty")
	}
	binary := filepath.Join(home, "plugins", "aws-ce", manifest.Metadata.Version, "finfocus-plugin-aws-ce")
	command = exec.Command(binary, "--help")
	if out, err := command.CombinedOutput(); err != nil || !strings.Contains(string(out), "port") {
		t.Fatalf("installed binary: %v\n%s", err, out)
	}
}

func TestMakeHasNoDockerTarget(t *testing.T) {
	root := filepath.Join("..", "..")
	help := exec.Command("make", "-C", root, "help")
	out, err := help.CombinedOutput()
	if err != nil || strings.Contains(strings.ToLower(string(out)), "docker") {
		t.Fatalf("help must describe archive-only support: %v\n%s", err, out)
	}
	makefile, err := filepath.Abs(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("make", "-f", makefile, "-n", "docker")
	command.Dir = t.TempDir()
	out, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "No rule to make target") {
		t.Fatalf("docker must have no recipe: %v\n%s", err, out)
	}
}
