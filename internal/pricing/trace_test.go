package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const traceCE14 = "trace-ce-1.4"

func TestSupports_LogTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := calculatorWithLogBuffer()
	ctx := pluginsdk.ContextWithTraceID(context.Background(), traceCE14)
	resp, err := calc.Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{
		Provider: "aws",
		Id:       "i-1234567890abcdef0",
	}})
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if resp == nil || !resp.GetSupported() || resp.GetReason() != "" {
		t.Fatalf("Supports decision changed: %#v", resp)
	}
	assertLogTraceID(t, buf, traceCE14)
}

func TestGetPluginInfo_LogTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := calculatorWithLogBuffer()
	ctx := pluginsdk.ContextWithTraceID(context.Background(), traceCE14)
	resp, err := calc.GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo: %v", err)
	}
	if resp.GetName() != pluginInfoName || resp.GetVersion() != pluginInfoVersion || resp.GetSpecVersion() != pluginInfoSpecVersion {
		t.Fatalf("GetPluginInfo fields changed: name=%q version=%q spec=%q", resp.GetName(), resp.GetVersion(), resp.GetSpecVersion())
	}
	if len(resp.GetProviders()) != 1 || resp.GetProviders()[0] != "aws" {
		t.Fatalf("providers changed: %v", resp.GetProviders())
	}
	assertLogTraceID(t, buf, traceCE14)
}

func TestGetPluginInfo_LogOmitsTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := calculatorWithLogBuffer()
	resp, err := calc.GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo: %v", err)
	}
	if resp.GetName() != pluginInfoName || resp.GetVersion() != pluginInfoVersion || resp.GetSpecVersion() != pluginInfoSpecVersion {
		t.Fatalf("GetPluginInfo fields changed: name=%q version=%q spec=%q", resp.GetName(), resp.GetVersion(), resp.GetSpecVersion())
	}
	assertLogOmitsTraceID(t, buf)
}

func TestGetActualCost_LogTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := actualCostCalculatorWithLogBuffer()
	ctx := pluginsdk.ContextWithTraceID(context.Background(), traceCE14)
	resp, err := calc.GetActualCost(ctx, serviceCostRequest())
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if resp == nil || len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1 {
		t.Fatalf("cost result changed: %#v", resp)
	}
	if !strings.Contains(buf.String(), "Retrieved costs from AWS") {
		t.Fatalf("missing cost log: %s", buf.String())
	}
	assertLogTraceID(t, buf, traceCE14)
}

func TestGetActualCost_MalformedARNLogTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := actualCostCalculatorWithLogBuffer()
	ctx := pluginsdk.ContextWithTraceID(context.Background(), traceCE14)
	req := serviceCostRequest()
	req.Arn = "invalid-arn-format"
	req.ResourceId = "i-fallback"
	if _, err := calc.GetActualCost(ctx, req); err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if !strings.Contains(buf.String(), "querying service totals") {
		t.Fatalf("malformed ARN log missing: %s", buf.String())
	}
	assertLogTraceID(t, buf, traceCE14)
}

func TestGetActualCost_LogOmitsTraceID(t *testing.T) {
	t.Parallel()

	calc, buf := actualCostCalculatorWithLogBuffer()
	resp, err := calc.GetActualCost(context.Background(), serviceCostRequest())
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if resp == nil || len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1 {
		t.Fatalf("cost result changed: %#v", resp)
	}
	assertLogOmitsTraceID(t, buf)
}

func calculatorWithLogBuffer() (*Calculator, *bytes.Buffer) {
	var buf bytes.Buffer
	calc := NewCalculator()
	calc.logger = zerolog.New(&buf)
	return calc, &buf
}

func actualCostCalculatorWithLogBuffer() (*Calculator, *bytes.Buffer) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	var buf bytes.Buffer
	calc.logger = zerolog.New(&buf)
	return calc, &buf
}

func serviceCostRequest() *pbc.GetActualCostRequest {
	return &pbc.GetActualCostRequest{
		ResourceId: "AmazonS3",
		Start:      timestamppb.New(time.Now().Add(-24 * time.Hour)),
		End:        timestamppb.New(time.Now()),
	}
}

func assertLogTraceID(t *testing.T, buf *bytes.Buffer, want string) {
	t.Helper()
	lines := jsonLogLines(t, buf)
	for _, raw := range lines {
		if strings.Count(raw, `"trace_id"`) != 1 {
			t.Fatalf("trace_id count = %d, want 1: %s", strings.Count(raw, `"trace_id"`), raw)
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log is not JSON: %v\n%s", err, raw)
		}
		if line["trace_id"] != want {
			t.Fatalf("trace_id = %v, want %q; log %s", line["trace_id"], want, raw)
		}
	}
}

func assertLogOmitsTraceID(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	lines := jsonLogLines(t, buf)
	for _, raw := range lines {
		if strings.Contains(raw, `"trace_id"`) {
			t.Fatalf("bare context log has trace_id: %s", raw)
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("log is not JSON: %v\n%s", err, raw)
		}
		if _, ok := line["trace_id"]; ok {
			t.Fatalf("bare context log has trace_id %v", line["trace_id"])
		}
	}
}

func jsonLogLines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	raw := strings.TrimSpace(buf.String())
	if raw == "" {
		t.Fatal("no log output")
	}
	return strings.Split(raw, "\n")
}
