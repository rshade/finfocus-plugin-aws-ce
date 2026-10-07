package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConcurrentBatchClientInitialization(t *testing.T) {
	f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
	calc := NewCalculator()
	calc.cache = nil
	calc.logger = zerolog.New(io.Discard)
	var opens atomic.Int32
	calc.openClient = func(ctx context.Context, cfg client.Config) (*client.Client, error) {
		opens.Add(1)
		time.Sleep(5 * time.Millisecond)
		cfg.Region = "us-east-1"
		cfg.BaseEndpoint = f.srv.URL
		cfg.AccessKeyID = "test"
		cfg.SecretAccessKey = "test"
		return client.NewClient(ctx, cfg)
	}
	srv := pluginsdk.NewTestServer(t, calc)
	defer srv.Close()
	start, end := recentCostWindow()
	req := &pbc.BatchCostRequest{QueryType: pbc.CostQueryType_COST_QUERY_TYPE_ACTUAL, Start: timestamppb.New(start), End: timestamppb.New(end)}
	for range 40 {
		req.Resources = append(req.Resources, &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: bareInstanceID})
	}
	resp, err := srv.Client().BatchCost(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetResults()) != 40 {
		t.Fatalf("results=%d", len(resp.GetResults()))
	}
	for _, row := range resp.GetResults() {
		if row.GetError() != nil {
			t.Fatalf("batch row error: %v", row.GetError())
		}
		actual := row.GetCostData().GetActualCost()
		if len(actual.GetResults()) != 1 || actual.GetResults()[0].GetCost() != 1 {
			t.Fatalf("batch cost: %v", actual)
		}
	}
	if opens.Load() != 1 {
		t.Fatalf("client initializations=%d want 1", opens.Load())
	}
}

func TestClientInitializationFailureCooldown(t *testing.T) {
	calc := NewCalculator()
	calc.logger = zerolog.New(io.Discard)
	var opens int
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		opens++
		return nil, errors.New("controlled initialization failure")
	}
	for range 3 {
		if err := calc.initClient(context.Background(), calc.logger); err == nil {
			t.Fatal("initialization failure hidden")
		}
	}
	if opens != 1 {
		t.Fatalf("opens=%d want 1 during cooldown", opens)
	}
	calc.clientMu.Lock()
	calc.clientRetryAt = time.Now().Add(-time.Second)
	calc.clientMu.Unlock()
	if err := calc.initClient(context.Background(), calc.logger); err == nil {
		t.Fatal("retry failure hidden")
	}
	if opens != 2 {
		t.Fatalf("opens=%d want 2 after cooldown", opens)
	}
}
