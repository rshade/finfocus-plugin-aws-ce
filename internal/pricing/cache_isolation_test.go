package pricing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
)

func TestActualCostCacheKeyDistinguishesMidnightNanoseconds(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	plan := costQueryPlan{cacheID: "AmazonS3"}
	midnight := actualCostCacheKey(plan, costRequest("AmazonS3", "", start, end, nil))
	afterMidnight := actualCostCacheKey(plan, costRequest("AmazonS3", "", start, end.Add(time.Nanosecond), nil))
	if midnight == afterMidnight {
		t.Fatal("exclusive midnight and one nanosecond later cover different CE dates but share a cache key")
	}
}

func TestActualCostCacheKeySharesNormalizedUTCPeriod(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	plan := costQueryPlan{cacheID: "AmazonS3"}
	first := actualCostCacheKey(plan, costRequest("AmazonS3", "", start, end.Add(time.Nanosecond), nil))
	zone := time.FixedZone("west", -7*60*60)
	sameDays := actualCostCacheKey(plan, costRequest("AmazonS3", "", start.Add(time.Hour).In(zone), end.Add(12*time.Hour).In(zone), nil))
	if first != sameDays {
		t.Fatalf("same normalized API dates have different cache keys: %q and %q", first, sameDays)
	}
}

func TestNewCalculatorUsesMemoryCacheWithoutDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	calc := NewCalculator()
	if calc.cache == nil {
		t.Fatal("default calculator has no cache")
	}
	if err := calc.cache.Set("test", []CostEntry{{Amount: 1}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := calc.cache.Get("test"); !ok || len(got) != 1 || got[0].Amount != 1 {
		t.Fatalf("memory cache results: %#v, %v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(home, ".finfocus", "cache", "aws-ce")); !os.IsNotExist(err) {
		t.Fatalf("default calculator created disk cache: %v", err)
	}
}

func TestActualCostCacheDoesNotReuseOtherCalculatorDiskResults(t *testing.T) {
	dir := t.TempDir()
	firstAPI := pricedCostAPI()
	first := NewCalculatorWithClient(client.NewClientWithAPI(firstAPI, "us-east-1"))
	var err error
	first.cache, err = NewCacheManager(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	req := costRequest("aws-account-total", "", start, start.Add(24*time.Hour), nil)
	if _, err := first.GetActualCost(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	secondAPI := &mockCostExplorerAPI{GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
		out := pricedCostOutput()
		metric := out.ResultsByTime[0].Groups[0].Metrics["UnblendedCost"]
		metric.Amount = aws.String("2.00")
		out.ResultsByTime[0].Groups[0].Metrics["UnblendedCost"] = metric
		return out, nil
	}}
	second := NewCalculatorWithClient(client.NewClientWithAPI(secondAPI, "us-east-1"))
	second.cache, err = NewCacheManager(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	response, err := second.GetActualCost(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.GetResults()) != 1 || response.GetResults()[0].GetCost() != 2 {
		t.Fatalf("second calculator returned another principal's costs: %v", response)
	}
	if len(secondAPI.costCalls) != 1 {
		t.Fatalf("second calculator made %d API calls; another calculator's cached billing data was reused", len(secondAPI.costCalls))
	}
	if _, err := second.GetActualCost(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(secondAPI.costCalls) != 1 {
		t.Fatalf("same calculator should reuse its cache; API calls = %d", len(secondAPI.costCalls))
	}
}
