package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestGoreleaserAssetNamesMatchInstallerExpectations validates that the actual goreleaser
// archive names in .goreleaser.yaml will be found by the finfocus plugin installer.
//
// This test reads the real .goreleaser.yaml, renders the name_template and format_overrides,
// and verifies each rendered name would be matched by buildAssetPatterns in
// finfocus/internal/registry/github.go (lines 502-531).
//
// If someone changes the template to produce "Macos" instead of "Darwin", or introduces
// dashes in the wrong place, this test will fail.
func TestGoreleaserAssetNamesMatchInstallerExpectations(t *testing.T) {
	// Find .goreleaser.yaml relative to this test file
	testDir := filepath.Dir(".")
	projRoot := filepath.Join(testDir, "..", "..")
	goreleaserPath := filepath.Join(projRoot, ".goreleaser.yaml")

	// Read the actual goreleaser config
	goreleaserYAML, err := os.ReadFile(goreleaserPath)
	require.NoError(t, err, "failed to read .goreleaser.yaml")

	// Parse YAML
	var config struct {
		ProjectName string `yaml:"project_name"`
		Archives    []struct {
			NameTemplate    string `yaml:"name_template"`
			FormatOverrides []struct {
				Goos   string `yaml:"goos"`
				Format string `yaml:"format"`
			} `yaml:"format_overrides"`
		} `yaml:"archives"`
	}
	require.NoError(t, yaml.Unmarshal(goreleaserYAML, &config), "failed to parse .goreleaser.yaml")
	require.NotEmpty(t, config.ProjectName, "project_name not found in .goreleaser.yaml")
	require.NotEmpty(t, config.Archives, "archives not found in .goreleaser.yaml")

	// Get the name_template and format_overrides
	archive := config.Archives[0] // Assume first archive entry
	nameTemplate := archive.NameTemplate
	require.NotEmpty(t, nameTemplate, "name_template is empty")

	// Build format override map
	formatOverrides := make(map[string]string)
	for _, override := range archive.FormatOverrides {
		formatOverrides[override.Goos] = override.Format
	}

	// Define text/template functions (matching goreleaser's template functions)
	funcMap := template.FuncMap{
		"title": func(s string) string {
			if len(s) == 0 {
				return ""
			}
			return string(s[0]-'a'+'A') + s[1:]
		},
	}

	// Parse and render the template
	tmpl, err := template.New("archive").Funcs(funcMap).Parse(nameTemplate)
	require.NoError(t, err, "failed to parse name_template: %s", nameTemplate)

	// Test matrix: all combinations of OS/arch/version
	testCases := []struct {
		version string
		goos    string
		goarch  string
	}{
		{"v0.1.0", "linux", "amd64"},
		{"v0.1.0", "linux", "arm64"},
		{"v0.1.0", "darwin", "amd64"},
		{"v0.1.0", "darwin", "arm64"},
		{"v0.1.0", "windows", "amd64"},
		{"v0.1.0", "windows", "arm64"},
		// Also test without leading v
		{"0.1.0", "linux", "amd64"},
		{"0.1.0", "darwin", "arm64"},
		{"0.1.0", "windows", "amd64"},
	}

	for _, tc := range testCases {
		t.Run(tc.version+"/"+tc.goos+"/"+tc.goarch, func(t *testing.T) {
			// Render the template
			var buf strings.Builder
			err := tmpl.Execute(&buf, map[string]string{
				"ProjectName": config.ProjectName,
				"Version":     tc.version,
				"Os":          tc.goos,
				"Arch":        tc.goarch,
			})
			require.NoError(t, err, "failed to render template")
			archiveName := buf.String()

			// Determine extension based on format_overrides
			ext := ".tar.gz"
			if format, ok := formatOverrides[tc.goos]; ok && format == "zip" {
				ext = ".zip"
			}

			// Construct full archive name with extension
			fullName := archiveName + ext

			// Verify using the installer's buildAssetPatterns logic
			// Source: finfocus/internal/registry/github.go lines 502-531
			patterns := buildAssetPatterns(config.ProjectName, tc.version, tc.goos, tc.goarch)

			// Check that the rendered name matches one of the expected patterns
			found := false
			for _, pattern := range patterns {
				if fullName == pattern {
					found = true
					break
				}
			}
			require.True(t, found, "archive name %q not in installer patterns for %s/%s/%s: %v",
				fullName, tc.version, tc.goos, tc.goarch, patterns)
		})
	}
}

// buildAssetPatterns generates the set of asset names that the finfocus installer will try
// to find for a given plugin release. This is a local copy of the logic from
// finfocus/internal/registry/github.go lines 502-531, kept in sync by comment reference.
//
// The installer tries multiple OS and arch name variations to be flexible with different
// naming conventions across plugin releases.
func buildAssetPatterns(projectName, version, goos, goarch string) []string {
	var osNames []string
	switch goos {
	case "darwin":
		osNames = []string{"darwin", "Darwin", "macos", "macOS", "MacOS"}
	case "linux":
		osNames = []string{"linux", "Linux"}
	case "windows":
		osNames = []string{"windows", "Windows"}
	}

	var archNames []string
	switch goarch {
	case "amd64":
		archNames = []string{"amd64", "x86_64", "X86_64", "AMD64"}
	case "arm64":
		archNames = []string{"arm64", "ARM64", "aarch64", "AARCH64"}
	}

	// Build all combinations of patterns the installer tries
	// Format: <name>_<version>_<os>_<arch>.<ext>
	var patterns []string
	for _, osName := range osNames {
		for _, archName := range archNames {
			// Determine extension
			ext := ".tar.gz"
			if goos == "windows" {
				ext = ".zip"
			}
			pattern := projectName + "_" + version + "_" + osName + "_" + archName + ext
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}
