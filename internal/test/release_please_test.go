package test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleasePleaseConfiguration(t *testing.T) {
	raw, err := os.ReadFile("../../release-please-config.json")
	require.NoError(t, err)
	var cfg struct {
		Packages map[string]map[string]any `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	pkg := cfg.Packages["."]
	require.NotNil(t, pkg)
	for key, want := range map[string]any{
		"release-type": "go", "bump-minor-pre-major": true,
		"bump-patch-for-minor-pre-major": true, "changelog-path": "CHANGELOG.md",
		"draft": false, "prerelease": false, "package-name": "finfocus-plugin-aws-ce",
		"include-component-in-tag": false, "initial-version": "0.1.0",
	} {
		require.Equal(t, want, pkg[key], key)
	}
	require.Equal(t, []any{
		map[string]any{"type": "feat", "section": "Features", "hidden": false},
		map[string]any{"type": "fix", "section": "Bug Fixes", "hidden": false},
		map[string]any{"type": "docs", "section": "Documentation", "hidden": false},
		map[string]any{"type": "chore", "section": "Miscellaneous", "hidden": true},
		map[string]any{"type": "test", "section": "Tests", "hidden": true},
	}, pkg["changelog-sections"])
	raw, err = os.ReadFile("../../.release-please-manifest.json")
	require.NoError(t, err)
	var manifest map[string]string
	require.NoError(t, json.Unmarshal(raw, &manifest))
	version := manifest["."]
	require.True(t, validReleaseVersion(version), "invalid release manifest version: %q", version)
}

func validReleaseVersion(version string) bool {
	return regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) &&
		(version == "0.0.0" || !strings.HasPrefix(version, "0.0."))
}

func TestReleaseManifestAllowsFutureVersions(t *testing.T) {
	for _, version := range []string{"0.0.0", "0.1.0", "0.1.1", "0.2.0", "1.0.0", "10.4.3"} {
		require.True(t, validReleaseVersion(version), version)
	}
	for _, version := range []string{"", "0.0.1", "0.1", "v0.1.0", "01.1.0", "0.1.0-rc.1", "broken"} {
		require.False(t, validReleaseVersion(version), version)
	}
}
