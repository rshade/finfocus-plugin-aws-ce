package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCacheKeyStaysInDirectory(t *testing.T) {
	t.Parallel()

	// Parent segment plus colon. A raw file name leaves the cache directory.
	assertCacheKeyInside(t, "../../x:arn:aws:ec2:us-east-1:123456789012:instance:i-0abc")
	// The slash in an ARN resource must not open a directory outside the cache.
	assertCacheKeyInside(t, "../../x:arn:aws:ec2:us-east-1:123456789012:instance/i-0abc")
}

func assertCacheKeyInside(t *testing.T, key string) {
	t.Helper()

	root := t.TempDir()
	cacheDir := filepath.Join(root, "nested", "cache")
	cm, err := NewCacheManager(cacheDir, time.Hour)
	if err != nil {
		t.Fatalf("NewCacheManager: %v", err)
	}

	want := []CostEntry{{Service: "AmazonS3", Amount: 1.25, Currency: "USD"}}
	if err := cm.Set(key, want); err != nil {
		t.Fatalf("Set %q: %v", key, err)
	}

	found := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(cacheDir, path)
		if relErr != nil {
			t.Fatalf("Rel(%s, %s): %v", cacheDir, path, relErr)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("cache file %s leaves %s (rel %s)", path, cacheDir, rel)
		}
		if !sha256FileName(filepath.Base(path)) {
			t.Fatalf("cache file %s is not <sha256>.json", filepath.Base(path))
		}
		found++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if found != 1 {
		t.Fatalf("cache files = %d, want 1", found)
	}

	loaded, err := NewCacheManager(cacheDir, time.Hour)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := loaded.Get(key)
	if !ok || len(got) != 1 || got[0].Amount != want[0].Amount || got[0].Service != want[0].Service {
		t.Fatalf("reloaded %#v ok=%v", got, ok)
	}
}

func sha256FileName(name string) bool {
	hexPart, ok := strings.CutSuffix(name, ".json")
	if !ok || len(hexPart) != 64 {
		return false
	}
	for _, r := range hexPart {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func TestActualCostTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	closed := now.Add(-72 * time.Hour)
	recent := now.Add(-47 * time.Hour)
	edge := now.Add(-recentCostHorizon)

	if got := actualCostTTL(closed, []CostEntry{{}}, now); got != closedHistoryTTL {
		t.Fatalf("closed history = %s, want %s", got, closedHistoryTTL)
	}
	if got := actualCostTTL(closed, []CostEntry{{}, {Estimated: true}}, now); got != recentCostTTL {
		t.Fatalf("estimated = %s, want %s", got, recentCostTTL)
	}
	if got := actualCostTTL(recent, []CostEntry{{}}, now); got != recentCostTTL {
		t.Fatalf("recent end = %s, want %s", got, recentCostTTL)
	}
	if got := actualCostTTL(edge, []CostEntry{{}}, now); got != closedHistoryTTL {
		t.Fatalf("end exactly 48h ago = %s, want %s", got, closedHistoryTTL)
	}
}

func TestActualCostCacheKey(t *testing.T) {
	t.Parallel()

	start := time.Unix(1_700_000_000, 0).UTC()
	end := start.Add(24 * time.Hour)
	plain := costRequest("AmazonS3", "", start, end, nil)
	tagged := costRequest("AmazonS3", "", start, end, map[string]string{"Team": "finops"})
	plan := costQueryPlan{cacheID: "AmazonS3"}
	if actualCostCacheKey(plan, plain) != actualCostCacheKey(plan, tagged) {
		t.Fatal("tags changed the cache key")
	}
	key := actualCostCacheKey(plan, plain)
	if !strings.Contains(key, ":DAILY:SERVICE:UnblendedCost") {
		t.Fatalf("key %q missing granularity, group, or metric", key)
	}
	if strings.Contains(key, "finops") {
		t.Fatalf("key %q contains a tag", key)
	}
	resource := costQueryPlan{cacheID: bareInstanceID, resourceLevel: true}
	if !strings.Contains(actualCostCacheKey(resource, plain), ":DAILY:RESOURCE_ID:UnblendedCost") {
		t.Fatalf("resource key = %s", actualCostCacheKey(resource, plain))
	}
}

func TestParseMaxRequestsPerMinute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: " 0 ", want: 0},
		{raw: "1", want: 1},
		{raw: "15", want: 15},
		{raw: "-1", want: 0},
		{raw: "nope", want: 0},
	}
	for _, tc := range cases {
		if got := parseMaxRequestsPerMinute(tc.raw); got != tc.want {
			t.Fatalf("parseMaxRequestsPerMinute(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestGetActualCost_ExpiresAt(t *testing.T) {
	t.Parallel()

	closedStart, closedEnd := closedHistoryWindow()
	recentStart, recentEnd := recentCostWindow()

	t.Run("estimated", func(t *testing.T) {
		t.Parallel()
		resp := actualCost(t, pagingCostAPI(1, true), "AmazonS3", closedStart, closedEnd)
		assertExpiresAbout(t, resp, recentCostTTL)
	})
	t.Run("closed_history", func(t *testing.T) {
		t.Parallel()
		resp := actualCost(t, pagingCostAPI(1, false), "AmazonS3", closedStart, closedEnd)
		assertExpiresAbout(t, resp, closedHistoryTTL)
	})
	t.Run("recent", func(t *testing.T) {
		t.Parallel()
		resp := actualCost(t, pagingCostAPI(1, false), "AmazonS3", recentStart, recentEnd)
		assertExpiresAbout(t, resp, recentCostTTL)
	})
}

func TestGetActualCost_CacheHitSkipsCostExplorer(t *testing.T) {
	t.Parallel()

	const pages = 2
	api := pagingCostAPI(pages, false)
	calc, buf := cachedCalc(t, api)
	start, end := closedHistoryWindow()
	req := costRequest("AmazonS3", "", start, end, nil)

	first, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Fatalf("first GetActualCost: %v", err)
	}
	second, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Fatalf("second GetActualCost: %v", err)
	}
	if len(api.costCalls) != pages {
		t.Fatalf("endpoint calls = %d, want %d", len(api.costCalls), pages)
	}
	counts := ceRequestCounts(t, buf)
	if len(counts) != 2 || counts[0] != pages+2 || counts[1] != 0 {
		t.Fatalf("ce_requests = %v, want [%d 0]", counts, pages+2)
	}
	if len(first.GetResults()) != 1 || len(second.GetResults()) != 1 {
		t.Fatalf("results first=%d second=%d", len(first.GetResults()), len(second.GetResults()))
	}
	if !second.GetResults()[0].GetExpiresAt().AsTime().Equal(first.GetResults()[0].GetExpiresAt().AsTime()) {
		t.Fatal("cache hit changed ExpiresAt")
	}
}

func TestGetActualCost_TagsDoNotChangeCacheKey(t *testing.T) {
	t.Parallel()

	api := pagingCostAPI(1, false)
	calc, _ := cachedCalc(t, api)
	start, end := closedHistoryWindow()
	plain := costRequest("AmazonS3", "", start, end, nil)
	tagged := costRequest("AmazonS3", "", start, end, map[string]string{"Team": "finops"})
	if _, err := calc.GetActualCost(context.Background(), plain); err != nil {
		t.Fatalf("plain: %v", err)
	}
	if _, err := calc.GetActualCost(context.Background(), tagged); err != nil {
		t.Fatalf("tagged: %v", err)
	}
	if len(api.costCalls) != 1 {
		t.Fatalf("endpoint calls = %d, want 1", len(api.costCalls))
	}
}

func TestGetActualCost_RateLimit(t *testing.T) {
	t.Parallel()

	t.Run("second uncached call", func(t *testing.T) {
		t.Parallel()
		api := pagingCostAPI(1, false)
		calc, buf := cachedCalc(t, api)
		calc.maxRequestsPerMinute = 3
		start, end := closedHistoryWindow()
		req := costRequest("AmazonS3", "", start, end, nil)
		if _, err := calc.GetActualCost(context.Background(), req); err != nil {
			t.Fatalf("first call: %v", err)
		}
		if _, err := calc.GetActualCost(context.Background(), req); err != nil {
			t.Fatalf("cache hit under the limit: %v", err)
		}
		req2 := costRequest("service-total", "", start, end, nil)
		req2.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:service", Id: "Amazon Elastic Compute Cloud - Compute"}
		_, err := calc.GetActualCost(context.Background(), req2)
		assertRateLimited(t, err)
		if len(api.costCalls) != 1 {
			t.Fatalf("endpoint calls = %d, want 1", len(api.costCalls))
		}
		if strings.Contains(buf.String(), "secret") || strings.Contains(buf.String(), "AKIA") {
			t.Fatalf("log contains credential-like text: %s", buf.String())
		}
	})

	t.Run("next page", func(t *testing.T) {
		t.Parallel()
		api := pagingCostAPI(5, false)
		calc, _ := cachedCalc(t, api)
		calc.maxRequestsPerMinute = 3
		start, end := closedHistoryWindow()
		_, err := calc.GetActualCost(context.Background(), costRequest("AmazonS3", "", start, end, nil))
		assertRateLimited(t, err)
		if len(api.costCalls) != 1 {
			t.Fatalf("endpoint calls = %d, want 1", len(api.costCalls))
		}
	})
}

func assertRateLimited(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("status = %s, want ResourceExhausted (%v)", status.Code(err), err)
	}
	if got := errorDetailCode(err); got != pbc.ErrorCode_ERROR_CODE_RATE_LIMITED {
		t.Fatalf("ErrorDetail = %s, want ERROR_CODE_RATE_LIMITED", got)
	}
}

func actualCost(t *testing.T, api *mockCostExplorerAPI, resourceID string, start, end time.Time) *pbc.GetActualCostResponse {
	t.Helper()
	calc, _ := cachedCalc(t, api)
	resp, err := calc.GetActualCost(context.Background(), costRequest(resourceID, "", start, end, nil))
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	return resp
}

func assertExpiresAbout(t *testing.T, resp *pbc.GetActualCostResponse, want time.Duration) {
	t.Helper()
	if resp == nil || len(resp.GetResults()) == 0 {
		t.Fatalf("no results: %#v", resp)
	}
	for i, res := range resp.GetResults() {
		got := res.GetExpiresAt()
		if got == nil {
			t.Fatalf("result %d ExpiresAt is nil", i)
		}
		delta := time.Until(got.AsTime())
		if delta < want-30*time.Second || delta > want+5*time.Second {
			t.Fatalf("result %d expires in %s, want about %s", i, delta, want)
		}
	}
}

func cachedCalc(t *testing.T, api *mockCostExplorerAPI) (*Calculator, *bytes.Buffer) {
	t.Helper()
	calc := calculatorWithCache(t, api)
	var buf bytes.Buffer
	calc.logger = zerolog.New(&buf)
	return calc, &buf
}

func pagingCostAPI(pages int, estimated bool) *mockCostExplorerAPI {
	var n int
	return &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			n++
			out := pricedCostOutput()
			out.ResultsByTime[0].Estimated = estimated
			if n < pages {
				out.NextPageToken = aws.String(fmt.Sprintf("page-%d", n+1))
			}
			return out, nil
		},
	}
}

func closedHistoryWindow() (time.Time, time.Time) {
	end := time.Now().UTC().Add(-72 * time.Hour)
	return end.Add(-24 * time.Hour), end
}

func ceRequestCounts(t *testing.T, buf *bytes.Buffer) []int {
	t.Helper()
	raw := strings.TrimSpace(buf.String())
	if raw == "" {
		t.Fatal("no log output")
	}
	var counts []int
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(line, "ce_requests") {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("log is not JSON: %v\n%s", err, line)
		}
		n, ok := decoded["ce_requests"].(float64)
		if !ok {
			t.Fatalf("ce_requests has type %T in %s", decoded["ce_requests"], line)
		}
		counts = append(counts, int(n))
	}
	return counts
}
