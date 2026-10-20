package e2e

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	pluginBinary = flag.String("plugin-binary", "", "Plugin binary; empty builds cmd/plugin")
	coreBinary   = flag.String("core-binary", "", "Pinned core executable; defaults to FINFOCUS_CORE_BINARY")
)

func e2eEnabled() bool {
	if value, set := os.LookupEnv("FINFOCUS_E2E"); set {
		return value == "true"
	}
	value, _ := os.LookupEnv("finfocus_E2E")
	return value == "true"
}

func TestE2EGate(t *testing.T) {
	for _, tc := range []struct {
		name, value, legacy string
		unset, want         bool
	}{
		{name: "uppercase", value: "true", want: true},
		{name: "legacy fallback", legacy: "true", unset: true, want: true},
		{name: "explicit false overrides legacy", value: "false", legacy: "true"},
		{name: "disabled", unset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FINFOCUS_E2E", tc.value)
			t.Setenv("finfocus_E2E", tc.legacy)
			if tc.unset {
				require.NoError(t, os.Unsetenv("FINFOCUS_E2E"))
			}
			assert.Equal(t, tc.want, e2eEnabled())
		})
	}
}

func TestE2E(t *testing.T) {
	if !e2eEnabled() {
		t.Skip("Set FINFOCUS_E2E=true and FINFOCUS_CORE_BINARY for offline real-core E2E")
	}
	core := *coreBinary
	if core == "" {
		core = os.Getenv("FINFOCUS_CORE_BINARY")
	}
	require.NotEmpty(t, core, "provide an explicitly built core via -core-binary or FINFOCUS_CORE_BINARY")
	binary := *pluginBinary
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "finfocus-plugin-aws-ce")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/plugin")
		cmd.Dir = filepath.Join("..", "..")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "build local plugin: %s", output)
	}
	end := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -2)
	start := end.AddDate(0, 0, -2)
	for _, tc := range []struct {
		name, resourceType, resourceID, mode string
		want                                 float64
	}{
		{name: "account", resourceType: "aws:account", resourceID: "aws-account-total", want: 4.25},
		{name: "service", resourceType: "aws:service", resourceID: "Amazon Simple Storage Service", want: 3.75},
		{name: "empty data is not success", resourceType: "aws:account", resourceID: "aws-account-total", mode: "empty"},
		{name: "AWS denial is not success", resourceType: "aws:account", resourceID: "aws-account-total", mode: "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCE{t: t, mode: tc.mode, service: tc.resourceType == "aws:service", calls: make(map[string]int), start: start, end: end}
			server := httptest.NewServer(fake)
			defer server.Close()
			rows, err := testutil.RunCoreActualCost(t, testutil.CoreActualCostConfig{
				CoreBinary: core, PluginBinary: binary, Start: start, End: end,
				ResourceType: tc.resourceType, ResourceID: tc.resourceID, OfflineEndpoint: server.URL,
			})
			if tc.mode != "" {
				require.Error(t, err, "core exit zero or empty JSON must not pass the harness")
				require.Empty(t, rows)
			} else {
				require.NoError(t, err)
				require.Len(t, rows, 1)
				assert.Equal(t, tc.want, rows[0].TotalCost)
				assert.Equal(t, tc.want, rows[0].Breakdown["aws-ce"])
				assert.Equal(t, "USD", rows[0].Currency)
				assert.Equal(t, "aws-ce", rows[0].Adapter)
				assert.Equal(t, "urn:pulumi:e2e::aws-ce::"+tc.resourceType+"::actual-cost", rows[0].ResourceID)
				assert.Equal(t, start, rows[0].StartDate)
				assert.Equal(t, end, rows[0].EndDate)
			}
			fake.mu.Lock()
			defer fake.mu.Unlock()
			assert.Equal(t, map[string]int{"RESERVATION_ID": 1, "SAVINGS_PLAN_ARN": 1, "GetCostAndUsage": 1}, fake.calls)
		})
	}
}

type fakeCE struct {
	t       *testing.T
	mu      sync.Mutex
	calls   map[string]int
	mode    string
	service bool
	start   time.Time
	end     time.Time
}

func (f *fakeCE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var request struct {
		TimePeriod  struct{ Start, End string }
		Dimension   string
		Context     string
		Granularity string
		Metrics     []string
		GroupBy     []struct{ Type, Key string }
		Filter      *struct {
			Dimensions struct {
				Key    string
				Values []string
			}
		}
		NextPageToken string
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		f.t.Errorf("decode AWS request: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	assert.Equal(f.t, http.MethodPost, r.Method)
	assert.Contains(f.t, r.Header.Get("Authorization"), "Credential=test/", "offline requests must use dummy credentials")
	assert.Empty(f.t, r.Header.Get("X-Amz-Security-Token"))
	assert.Equal(f.t, f.start.Format(time.DateOnly), request.TimePeriod.Start)
	assert.Equal(f.t, f.end.Format(time.DateOnly), request.TimePeriod.End)
	assert.Empty(f.t, request.NextPageToken)
	if f.service {
		if assert.NotNil(f.t, request.Filter) {
			assert.Equal(f.t, "SERVICE", request.Filter.Dimensions.Key)
			assert.Equal(f.t, []string{"Amazon Simple Storage Service"}, request.Filter.Dimensions.Values)
		}
	} else {
		assert.Nil(f.t, request.Filter)
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	switch operation := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "AWSInsightsIndexService."); operation {
	case "GetDimensionValues":
		assert.Contains(f.t, []string{"RESERVATION_ID", "SAVINGS_PLAN_ARN"}, request.Dimension)
		assert.Equal(f.t, "COST_AND_USAGE", request.Context)
		f.calls[request.Dimension]++
		_, _ = w.Write([]byte(`{"DimensionValues":[]}`))
	case "GetCostAndUsage":
		f.calls[operation]++
		assert.Equal(f.t, "DAILY", request.Granularity)
		assert.Equal(f.t, []string{"UnblendedCost"}, request.Metrics)
		if assert.Len(f.t, request.GroupBy, 1) {
			assert.Equal(f.t, "DIMENSION", request.GroupBy[0].Type)
			assert.Equal(f.t, "SERVICE", request.GroupBy[0].Key)
		}
		if f.mode == "denied" {
			w.Header().Set("X-Amzn-Errortype", "AccessDeniedException")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"AccessDeniedException","message":"offline denial"}`))
			return
		}
		if f.mode == "empty" {
			_, _ = w.Write([]byte(`{"ResultsByTime":[]}`))
			return
		}
		extra := ""
		if !f.service {
			extra = `,{"Keys":["Amazon Elastic Compute Cloud - Compute"],"Metrics":{"UnblendedCost":{"Amount":"0.50","Unit":"USD"}}}`
		}
		middle := f.start.AddDate(0, 0, 1).Format(time.DateOnly)
		_, _ = fmt.Fprintf(w, `{"ResultsByTime":[{"Estimated":false,"TimePeriod":{"Start":%q,"End":%q},"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"1.25","Unit":"USD"}}}%s]},{"Estimated":false,"TimePeriod":{"Start":%q,"End":%q},"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"2.50","Unit":"USD"}}}]}]}`, f.start.Format(time.DateOnly), middle, extra, middle, f.end.Format(time.DateOnly))
	default:
		f.t.Errorf("unexpected AWS operation %q", operation)
		http.Error(w, "unexpected AWS operation", http.StatusBadRequest)
	}
}
