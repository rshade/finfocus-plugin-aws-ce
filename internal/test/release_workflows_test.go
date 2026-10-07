package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestReleaseWorkflows(t *testing.T) {
	paths, err := filepath.Glob("../../.github/workflows/*.yml")
	require.NoError(t, err)
	yamlPaths, err := filepath.Glob("../../.github/workflows/*.yaml")
	require.NoError(t, err)
	paths = append(paths, yamlPaths...)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var workflow map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &workflow))
		for _, banned := range []string{"docker/build-push-action", "--rm-dist", "google-apis/"} {
			require.NotContains(t, string(raw), banned, path)
		}
		triggers, _ := workflow["on"].(map[string]any)
		push, _ := triggers["push"].(map[string]any)
		require.NotContains(t, push, "tags", path)
	}
	raw, err := os.ReadFile("../../.github/workflows/release-please.yml")
	require.NoError(t, err)
	require.Contains(t, string(raw), "googleapis/release-please-action@v5.0.0")
	require.Contains(t, string(raw), "token: ${{ secrets.RELEASE_PLEASE_TOKEN }}")
	raw, err = os.ReadFile("../../.github/workflows/release.yml")
	require.NoError(t, err)
	var workflow struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			Steps []struct {
				Uses string         `yaml:"uses"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &workflow))
	release, ok := workflow.On["release"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"created"}, release["types"])
	dispatch, ok := workflow.On["workflow_dispatch"].(map[string]any)
	require.True(t, ok)
	inputs, ok := dispatch["inputs"].(map[string]any)
	require.True(t, ok)
	tag, ok := inputs["tag"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, tag["required"])
	steps := workflow.Jobs["goreleaser"].Steps
	require.Len(t, steps, 3)
	require.Equal(t, "actions/checkout@v7", steps[0].Uses)
	require.Equal(t, "${{ inputs.tag || github.event.release.tag_name }}", steps[0].With["ref"])
	require.Equal(t, 0, steps[0].With["fetch-depth"])
	require.Equal(t, "actions/setup-go@v7", steps[1].Uses)
	require.Equal(t, "go.mod", steps[1].With["go-version-file"])
	require.Equal(t, "goreleaser/goreleaser-action@v7", steps[2].Uses)
	require.Equal(t, "release --clean", strings.TrimSpace(steps[2].With["args"].(string)))
}
