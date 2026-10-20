package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// CoreActualCostConfig selects an explicit core binary and one account or service query.
// OfflineEndpoint enables credential isolation and directs every AWS call to that URL.
type CoreActualCostConfig struct {
	CoreBinary, PluginBinary string
	Start, End               time.Time
	ResourceType, ResourceID string
	Env                      map[string]string
	OfflineEndpoint          string
}

// CoreCostResult contains the public actual-cost JSON fields checked by the harness.
type CoreCostResult struct {
	ResourceID   string             `json:"resourceId"`
	ResourceType string             `json:"resourceType"`
	Adapter      string             `json:"adapter"`
	Currency     string             `json:"currency"`
	TotalCost    float64            `json:"totalCost"`
	Breakdown    map[string]float64 `json:"breakdown"`
	StartDate    time.Time          `json:"startDate"`
	EndDate      time.Time          `json:"endDate"`
	Notes        string             `json:"notes"`
	Error        json.RawMessage    `json:"error"`
}

// RunCoreActualCost runs the real core through an isolated plugin registry.
// CoreBinary must be supplied explicitly or through FINFOCUS_CORE_BINARY; tests
// never download core. An empty PluginBinary builds this checkout's cmd/plugin.
// A successful command with missing costs is returned as an error. Live stderr
// is deliberately excluded from errors because it can contain account details.
func RunCoreActualCost(t testing.TB, cfg CoreActualCostConfig) ([]CoreCostResult, error) {
	t.Helper()
	if cfg.CoreBinary == "" {
		cfg.CoreBinary = os.Getenv("FINFOCUS_CORE_BINARY")
	}
	if cfg.CoreBinary == "" {
		return nil, errors.New("set FINFOCUS_CORE_BINARY or -core-binary to an explicitly built core executable")
	}
	coreBinary, err := filepath.Abs(cfg.CoreBinary)
	if err != nil {
		return nil, fmt.Errorf("resolve core executable: %w", err)
	}
	if cfg.ResourceType == "" {
		cfg.ResourceType = "aws:account"
	}
	if cfg.ResourceID == "" {
		if cfg.ResourceType != "aws:account" {
			return nil, errors.New("service queries require a resource ID containing the AWS service name")
		}
		cfg.ResourceID = "aws-account-total"
	}
	if cfg.ResourceType != "aws:account" && cfg.ResourceType != "aws:service" {
		return nil, errors.New("core harness supports explicit aws:account or aws:service queries")
	}
	if !cfg.End.After(cfg.Start) || !cfg.Start.Equal(cfg.Start.UTC().Truncate(24*time.Hour)) ||
		!cfg.End.Equal(cfg.End.UTC().Truncate(24*time.Hour)) {
		return nil, errors.New("core harness dates must be increasing UTC day boundaries")
	}
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "aws-ce", "0.0.0")
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		return nil, fmt.Errorf("create plugin registry: %w", err)
	}
	pluginPath := filepath.Join(pluginDir, "finfocus-plugin-aws-ce")
	if runtime.GOOS == "windows" {
		pluginPath += ".exe"
	}
	if err := installCoreTestPlugin(t, cfg.PluginBinary, pluginPath); err != nil {
		return nil, err
	}
	plan := map[string]any{"steps": []any{map[string]any{
		"op": "same", "urn": coreResourceURN(cfg.ResourceType), "type": cfg.ResourceType,
		"inputs": map[string]any{"pulumi:cloudId": cfg.ResourceID},
	}}}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode core plan: %w", err)
	}
	planPath := filepath.Join(root, "plan.json")
	if err := os.WriteFile(planPath, planJSON, 0o600); err != nil {
		return nil, fmt.Errorf("write core plan: %w", err)
	}
	for _, name := range []string{"aws-config", "aws-credentials"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			return nil, fmt.Errorf("write isolated AWS configuration: %w", err)
		}
	}
	// Finish before the plugin's per-minute request budget can reset.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, coreBinary, "cost", "actual", "--pulumi-json", planPath,
		"--from", cfg.Start.Format(time.DateOnly), "--to", cfg.End.Format(time.DateOnly),
		"--adapter", "aws-ce", "--output", "json")
	cmd.Dir = root
	cmd.Env = coreEnvironment(root, cfg)
	configureCoreProcess(cmd)
	defer func() { _ = stopCoreProcess(cmd) }()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if cfg.OfflineEndpoint != "" {
			return nil, fmt.Errorf("offline core actual-cost command failed: %w\n%s", err, stderr.String())
		}
		return nil, fmt.Errorf("core actual-cost command failed: %w", err)
	}
	return decodeCoreActualCost(stdout.Bytes(), cfg)
}

func installCoreTestPlugin(t testing.TB, binary, destination string) error {
	t.Helper()
	if binary != "" {
		data, err := os.ReadFile(binary)
		if err != nil {
			return fmt.Errorf("read local plugin executable: %w", err)
		}
		return os.WriteFile(destination, data, 0o700)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("cannot locate plugin source checkout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", destination, "./cmd/plugin")
	cmd.Dir = filepath.Join(filepath.Dir(source), "..", "..")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build local plugin: %w\n%s", err, output)
	}
	return nil
}

func coreResourceURN(resourceType string) string {
	return "urn:pulumi:e2e::aws-ce::" + resourceType + "::actual-cost"
}

func coreEnvironment(root string, cfg CoreActualCostConfig) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "FINFOCUS_") || strings.HasPrefix(key, "PULUMI_") ||
			(cfg.OfflineEndpoint != "" && strings.HasPrefix(key, "AWS_")) {
			continue
		}
		values[key] = value
	}
	for key, value := range cfg.Env {
		values[key] = value
	}
	values["FINFOCUS_HOME"] = root
	values["FINFOCUS_CACHE_ENABLED"] = "false"
	values["FINFOCUS_CACHE_DIR"] = filepath.Join(root, "cache")
	values["FINFOCUS_SKIP_MIGRATION_CHECK"] = "true"
	values["FINFOCUS_HIDE_ALIAS_HINT"] = "true"
	values["FINFOCUS_PLUGIN_MODE"] = "false"
	values["FINFOCUS_LOG_LEVEL"] = "error"
	values["FINFOCUS_LOG_FILE"] = ""
	values["AWS_REGION"] = "us-east-1"
	values["AWS_DEFAULT_REGION"] = "us-east-1"
	values["AWS_MAX_ATTEMPTS"] = "1"
	for key := range values {
		if strings.HasPrefix(key, "AWS_ENDPOINT_URL") {
			delete(values, key)
		}
	}
	values["AWS_IGNORE_CONFIGURED_ENDPOINT_URLS"] = "true"
	if cfg.OfflineEndpoint != "" {
		// Apply isolation after caller overrides, so accidental credentials cannot
		// turn an offline invocation into a request signed by a real principal.
		for key := range values {
			if strings.HasPrefix(key, "AWS_") {
				delete(values, key)
			}
		}
		values["AWS_REGION"], values["AWS_DEFAULT_REGION"] = "us-east-1", "us-east-1"
		values["AWS_MAX_ATTEMPTS"] = "1"
		values["AWS_ACCESS_KEY_ID"], values["AWS_SECRET_ACCESS_KEY"] = "test", "test"
		values["AWS_CONFIG_FILE"] = filepath.Join(root, "aws-config")
		values["AWS_SHARED_CREDENTIALS_FILE"] = filepath.Join(root, "aws-credentials")
		values["AWS_EC2_METADATA_DISABLED"] = "true"
		values["AWS_IGNORE_CONFIGURED_ENDPOINT_URLS"] = "false"
		values["AWS_ENDPOINT_URL"], values["AWS_ENDPOINT_URL_COST_EXPLORER"] = cfg.OfflineEndpoint, cfg.OfflineEndpoint
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func decodeCoreActualCost(output []byte, cfg CoreActualCostConfig) ([]CoreCostResult, error) {
	var rows []CoreCostResult
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&rows); err != nil {
		return nil, errors.New("core output is not an actual-cost JSON array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("core output contains trailing data")
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("core returned %d cost rows; expected exactly one", len(rows))
	}
	row := rows[0]
	if row.ResourceID != coreResourceURN(cfg.ResourceType) || row.ResourceType != cfg.ResourceType || row.Adapter != "aws-ce" {
		return nil, errors.New("core result identity or adapter does not match the local aws-ce query")
	}
	if row.Currency == "" || !row.StartDate.Equal(cfg.Start) || !row.EndDate.Equal(cfg.End) {
		return nil, errors.New("core result currency or period is missing or differs")
	}
	if len(row.Error) > 0 && string(row.Error) != "null" {
		return nil, errors.New("core result contains a structured error")
	}
	for _, marker := range []string{"error", "fallback", "no cost", "no data", "estimate"} {
		if strings.Contains(strings.ToLower(row.Notes), marker) {
			return nil, errors.New("core result contains an error or fallback note")
		}
	}
	sourceCost, ok := row.Breakdown["aws-ce"]
	if !ok || len(row.Breakdown) != 1 || math.IsNaN(row.TotalCost) || math.IsInf(row.TotalCost, 0) ||
		math.Abs(sourceCost-row.TotalCost) > 1e-9*math.Max(1, math.Abs(row.TotalCost)) {
		return nil, errors.New("core result is missing consistent aws-ce source costs")
	}
	return rows, nil
}
