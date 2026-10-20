package testutil

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecodeCoreActualCost(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	cfg := CoreActualCostConfig{Start: start, End: start.AddDate(0, 0, 2), ResourceType: "aws:account"}
	valid := `[{"resourceId":"urn:pulumi:e2e::aws-ce::aws:account::actual-cost","resourceType":"aws:account","adapter":"aws-ce","currency":"USD","totalCost":3.75,"breakdown":{"aws-ce":3.75},"startDate":"2026-09-20T00:00:00Z","endDate":"2026-09-22T00:00:00Z"}]`
	for _, tc := range []struct {
		name, output string
		wantError    bool
	}{
		{name: "valid", output: valid},
		{name: "empty success", output: `[]`, wantError: true},
		{name: "null success", output: `null`, wantError: true},
		{name: "trailing document", output: valid + `[]`, wantError: true},
		{name: "missing costs", output: strings.ReplaceAll(valid, `"breakdown":{"aws-ce":3.75}`, `"breakdown":{}`), wantError: true},
		{name: "wrong adapter", output: strings.Replace(valid, `"adapter":"aws-ce"`, `"adapter":"none"`, 1), wantError: true},
		{name: "missing resource", output: strings.Replace(valid, `::actual-cost`, `::other`, 1), wantError: true},
		{name: "wrong period", output: strings.Replace(valid, `2026-09-22`, `2026-09-23`, 1), wantError: true},
		{name: "structured error", output: strings.Replace(valid, `"currency":"USD"`, `"currency":"USD","error":{"code":"PLUGIN_ERROR"}`, 1), wantError: true},
		{name: "wrong sum", output: strings.Replace(valid, `"totalCost":3.75`, `"totalCost":9`, 1), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, err := decodeCoreActualCost([]byte(tc.output), cfg)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, 3.75, rows[0].TotalCost)
		})
	}
}

func TestCoreEnvironmentIsolation(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "real-key-must-not-propagate")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "real-secret-must-not-propagate")
	t.Setenv("AWS_SESSION_TOKEN", "real-token-must-not-propagate")
	t.Setenv("AWS_ENDPOINT_URL_STS", "https://inherited.invalid")
	t.Setenv("FINFOCUS_HOME", "/inherited/home")
	t.Setenv("FINFOCUS_PLUGIN_MODE", "true")
	root := t.TempDir()
	entries := coreEnvironment(root, CoreActualCostConfig{
		OfflineEndpoint: "http://127.0.0.1:12345",
		Env: map[string]string{
			"AWS_ACCESS_KEY_ID":      "override-key-must-not-propagate",
			"AWS_SESSION_TOKEN":      "override-token-must-not-propagate",
			"AWS_ENDPOINT_URL":       "https://override.invalid",
			"FINFOCUS_HOME":          "/override/home",
			"FINFOCUS_CACHE_ENABLED": "true",
		},
	})
	values := make(map[string]string)
	for _, entry := range entries {
		key, value, _ := strings.Cut(entry, "=")
		require.NotContains(t, values, key, "environment must not contain duplicate keys")
		values[key] = value
	}
	require.Equal(t, "test", values["AWS_ACCESS_KEY_ID"])
	require.Equal(t, "test", values["AWS_SECRET_ACCESS_KEY"])
	require.NotContains(t, values, "AWS_SESSION_TOKEN")
	require.NotContains(t, values, "AWS_ENDPOINT_URL_STS")
	require.Equal(t, "http://127.0.0.1:12345", values["AWS_ENDPOINT_URL_COST_EXPLORER"])
	require.Equal(t, "http://127.0.0.1:12345", values["AWS_ENDPOINT_URL"])
	require.Equal(t, root, values["FINFOCUS_HOME"])
	require.Equal(t, "false", values["FINFOCUS_CACHE_ENABLED"])
	require.Equal(t, "false", values["FINFOCUS_PLUGIN_MODE"])
	require.Equal(t, "true", values["AWS_EC2_METADATA_DISABLED"])
}
