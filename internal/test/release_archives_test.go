package test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestReleaseArchivesOnly(t *testing.T) {
	raw, err := os.ReadFile("../../.goreleaser.yaml")
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	for _, banned := range []string{"dockers", "dockers_v2", "homebrew_casks", "brews", "nfpms"} {
		require.NotContains(t, cfg, banned)
	}
	require.NotContains(t, string(raw), "pulumicost")
	require.NotContains(t, string(raw), ".Dirty")
	require.Equal(t, "finfocus-plugin-aws-ce", cfg["project_name"])
	archives, ok := cfg["archives"].([]any)
	require.True(t, ok)
	require.Len(t, archives, 1)
	archive, ok := archives[0].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, archive, "format")
	require.Equal(t, []any{"tar.gz"}, archive["formats"])
	overrides, ok := archive["format_overrides"].([]any)
	require.True(t, ok)
	require.Len(t, overrides, 1)
	override, ok := overrides[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "windows", override["goos"])
	require.NotContains(t, override, "format")
	require.Equal(t, []any{"zip"}, override["formats"])
	require.NotContains(t, archive["name_template"], ".tar.gz")
	require.NotContains(t, archive["name_template"], ".zip")
	checksum, ok := cfg["checksum"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "checksums.txt", checksum["name_template"])
}
