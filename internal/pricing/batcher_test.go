package pricing

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// recordingAPI answers resource-level queries with one group per key that
// emit returns, and records the RESOURCE_ID values of every query.
type recordingAPI struct {
	mockCostExplorerAPI
	mu    sync.Mutex
	calls [][]string
	emit  func(call int, ids []string) []string
}

func (r *recordingAPI) GetCostAndUsageWithResources(_ context.Context, in *costexplorer.GetCostAndUsageWithResourcesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageWithResourcesOutput, error) {
	ids := resourceIDsOf(in.Filter)
	r.mu.Lock()
	r.calls = append(r.calls, ids)
	call := len(r.calls)
	r.mu.Unlock()
	keys := ids
	if r.emit != nil {
		keys = r.emit(call, ids)
	}
	var groups []types.Group
	for _, key := range keys {
		groups = append(groups, types.Group{
			Keys:    []string{key},
			Metrics: map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String("2"), Unit: aws.String("USD")}},
		})
	}
	return &costexplorer.GetCostAndUsageWithResourcesOutput{ResultsByTime: []types.ResultByTime{{
		TimePeriod: &types.DateInterval{Start: aws.String("2026-10-01"), End: aws.String("2026-10-02")},
		Groups:     groups,
	}}}, nil
}

func (r *recordingAPI) GetDimensionValues(context.Context, *costexplorer.GetDimensionValuesInput, ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	return &costexplorer.GetDimensionValuesOutput{}, nil
}

func (r *recordingAPI) queryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func resourceIDsOf(e *types.Expression) []string {
	if e == nil {
		return nil
	}
	var ids []string
	if e.Dimensions != nil && e.Dimensions.Key == types.DimensionResourceId {
		ids = append(ids, e.Dimensions.Values...)
	}
	for _, part := range e.And {
		ids = append(ids, resourceIDsOf(&part)...)
	}
	return ids
}

func batchInstanceID(n int) string { return fmt.Sprintf("i-%017x", n) }

func batchCalc(api *recordingAPI, window time.Duration) *Calculator {
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	calc.cache = nil
	calc.batcher = newResourceBatcher(window)
	return calc
}

func actualCostsTogether(calc *Calculator, ids []string) []error {
	start, end := recentCostWindow()
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = calc.GetActualCost(context.Background(), costRequest(id, "", start, end, nil))
		}()
	}
	wg.Wait()
	return errs
}

func TestResourceLookupsArrivingTogetherShareOneQuery(t *testing.T) {
	t.Parallel()
	api := &recordingAPI{}
	calc := batchCalc(api, 300*time.Millisecond)
	var ids []string
	for i := range 12 {
		ids = append(ids, batchInstanceID(i))
	}
	for i, err := range actualCostsTogether(calc, ids) {
		if err != nil {
			t.Fatalf("resource %d: %v", i, err)
		}
	}
	if got := api.queryCount(); got != 1 {
		t.Fatalf("Cost Explorer resource queries = %d, want 1 for 12 resources", got)
	}
	if len(api.calls[0]) != 12 {
		t.Fatalf("query carried %d resource ids, want 12", len(api.calls[0]))
	}
}

func TestResourceBatchesAreCappedAtFiftyIDs(t *testing.T) {
	t.Parallel()
	api := &recordingAPI{}
	calc := batchCalc(api, 300*time.Millisecond)
	var ids []string
	for i := range 120 {
		ids = append(ids, batchInstanceID(i))
	}
	for i, err := range actualCostsTogether(calc, ids) {
		if err != nil {
			t.Fatalf("resource %d: %v", i, err)
		}
	}
	total := 0
	for _, call := range api.calls {
		if len(call) > maxBatchIDs {
			t.Fatalf("query carried %d ids, cap is %d", len(call), maxBatchIDs)
		}
		total += len(call)
	}
	if total != 120 || len(api.calls) > 4 {
		t.Fatalf("queries=%d ids=%d, want at most 4 queries covering all 120", len(api.calls), total)
	}
}

func TestResourcesWithoutCostAreNotFoundForEveryCaller(t *testing.T) {
	t.Parallel()
	api := &recordingAPI{emit: func(int, []string) []string { return nil }}
	calc := batchCalc(api, 200*time.Millisecond)
	errs := actualCostsTogether(calc, []string{batchInstanceID(1), batchInstanceID(2), batchInstanceID(3)})
	for i, err := range errs {
		if status.Code(err) != codes.NotFound {
			t.Fatalf("resource %d: %v, want NotFound", i, err)
		}
	}
	if got := api.queryCount(); got != 1 {
		t.Fatalf("queries = %d, want 1: nothing was unclaimed, so nobody needs a retry", got)
	}
}

func TestResourceKeyedDifferentlyIsQueriedAlone(t *testing.T) {
	t.Parallel()
	a, b := batchInstanceID(1), batchInstanceID(2)
	api := &recordingAPI{emit: func(call int, ids []string) []string {
		if call == 1 {
			return []string{a, "keyed-another-way"}
		}
		return ids
	}}
	calc := batchCalc(api, 200*time.Millisecond)
	for i, err := range actualCostsTogether(calc, []string{a, b}) {
		if err != nil {
			t.Fatalf("resource %d: %v", i, err)
		}
	}
	if got := api.queryCount(); got != 2 {
		t.Fatalf("queries = %d, want the shared query plus one for the unmatched resource", got)
	}
	if len(api.calls[1]) != 1 || api.calls[1][0] != b {
		t.Fatalf("follow-up query ids = %v, want only %s", api.calls[1], b)
	}
}

func TestZeroWindowQueriesEveryResourceAlone(t *testing.T) {
	t.Parallel()
	api := &recordingAPI{}
	calc := batchCalc(api, 0)
	var ids []string
	for i := range 5 {
		ids = append(ids, batchInstanceID(i))
	}
	for i, err := range actualCostsTogether(calc, ids) {
		if err != nil {
			t.Fatalf("resource %d: %v", i, err)
		}
	}
	if got := api.queryCount(); got != 5 {
		t.Fatalf("queries = %d, want 5 with batching off", got)
	}
}

func TestCanceledCallerLeavesTheSharedQueryRunning(t *testing.T) {
	t.Parallel()
	api := &recordingAPI{}
	calc := batchCalc(api, 150*time.Millisecond)
	start, end := recentCostWindow()
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := calc.GetActualCost(ctx, costRequest(batchInstanceID(1), "", start, end, nil))
		first <- err
	}()
	second := make(chan error, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, err := calc.GetActualCost(context.Background(), costRequest(batchInstanceID(2), "", start, end, nil))
		second <- err
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	if err := <-first; err == nil {
		t.Fatal("the canceled caller should get an error")
	}
	if err := <-second; err != nil {
		t.Fatalf("the second caller should still get its cost, got %v", err)
	}
}
