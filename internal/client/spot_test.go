package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeSpotAPI struct {
	inputs []*ec2.DescribeSpotPriceHistoryInput
	output *ec2.DescribeSpotPriceHistoryOutput
	err    error
	calls  int
}

func (f *fakeSpotAPI) DescribeSpotPriceHistory(_ context.Context, params *ec2.DescribeSpotPriceHistoryInput, _ ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	f.calls++
	cloned := *params
	f.inputs = append(f.inputs, &cloned)
	if f.err != nil {
		return nil, f.err
	}
	if f.output == nil {
		return &ec2.DescribeSpotPriceHistoryOutput{}, nil
	}
	return f.output, nil
}

func TestLatestSpotPriceFiltersZoneAndKeepsNewest(t *testing.T) {
	older := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: []ec2types.SpotPrice{
		{AvailabilityZone: aws.String("us-east-1a"), SpotPrice: aws.String("0.0400"), Timestamp: &older},
		{AvailabilityZone: aws.String("us-east-1b"), SpotPrice: aws.String("0.0010"), Timestamp: &newer},
		{AvailabilityZone: aws.String("us-east-1a"), SpotPrice: aws.String("0.0314"), Timestamp: &newer},
	}}}
	hourly, err := (&Client{spotAPI: api}).LatestSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX", "us-east-1a")
	if err != nil {
		t.Fatal(err)
	}
	if hourly != 0.0314 {
		t.Fatalf("hourly=%v", hourly)
	}
	in := api.inputs[0]
	if aws.ToString(in.AvailabilityZone) != "us-east-1a" || len(in.ProductDescriptions) != 1 || in.ProductDescriptions[0] != "Linux/UNIX" {
		t.Fatalf("input zone=%q products=%v", aws.ToString(in.AvailabilityZone), in.ProductDescriptions)
	}
	if len(in.InstanceTypes) != 1 || in.InstanceTypes[0] != "m5.large" {
		t.Fatalf("instance types=%v", in.InstanceTypes)
	}
}

func TestLatestSpotPriceCachesEmptyAndHit(t *testing.T) {
	api := &fakeSpotAPI{}
	c := &Client{spotAPI: api}
	_, err := c.LatestSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX", "us-east-1a")
	if !errors.Is(err, ErrNoSpotPrice) {
		t.Fatalf("err=%v", err)
	}
	_, err = c.LatestSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX", "us-east-1a")
	if !errors.Is(err, ErrNoSpotPrice) {
		t.Fatalf("second err=%v", err)
	}
	if api.calls != 1 {
		t.Fatalf("calls=%d", api.calls)
	}
}

func TestLatestSpotPriceUnconfigured(t *testing.T) {
	_, err := (&Client{}).LatestSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX", "us-east-1a")
	if !errors.Is(err, ErrSpotClientUnconfigured) {
		t.Fatalf("err=%v", err)
	}
}

type pagedSpotAPI struct {
	pages []*ec2.DescribeSpotPriceHistoryOutput
	calls int
}

func (f *pagedSpotAPI) DescribeSpotPriceHistory(_ context.Context, _ *ec2.DescribeSpotPriceHistoryInput, _ ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	f.calls++
	if f.calls > len(f.pages) {
		return &ec2.DescribeSpotPriceHistoryOutput{}, nil
	}
	return f.pages[f.calls-1], nil
}

func regionalRow(zone, price string, at time.Time) ec2types.SpotPrice {
	return ec2types.SpotPrice{AvailabilityZone: aws.String(zone), SpotPrice: aws.String(price), Timestamp: &at}
}

var regionalWindowEnd = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestLatestRegionalSpotPriceMedianOfOddZoneCount(t *testing.T) {
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: []ec2types.SpotPrice{
		regionalRow("us-east-1a", "0.0300", regionalWindowEnd),
		regionalRow("us-east-1b", "0.0500", regionalWindowEnd),
		regionalRow("us-east-1c", "0.0400", regionalWindowEnd),
		regionalRow("us-east-1a", "0.0900", regionalWindowEnd.Add(-time.Minute)),
	}}}
	quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Hourly != 0.04 || quote.Zones != 3 {
		t.Fatalf("quote=%+v, want median 0.04 over 3 zones", quote)
	}
	in := api.inputs[0]
	if in.AvailabilityZone != nil {
		t.Fatalf("regional query must not filter by zone, got %q", aws.ToString(in.AvailabilityZone))
	}
	if len(in.InstanceTypes) != 1 || in.InstanceTypes[0] != "m5.large" || len(in.ProductDescriptions) != 1 || in.ProductDescriptions[0] != "Linux/UNIX" {
		t.Fatalf("instance types=%v products=%v", in.InstanceTypes, in.ProductDescriptions)
	}
}

func TestLatestRegionalSpotPriceMeanOfEvenZoneCount(t *testing.T) {
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: []ec2types.SpotPrice{
		regionalRow("us-east-1d", "0.0600", regionalWindowEnd),
		regionalRow("us-east-1a", "0.0200", regionalWindowEnd),
		regionalRow("us-east-1c", "0.0500", regionalWindowEnd),
		regionalRow("us-east-1b", "0.0300", regionalWindowEnd),
	}}}
	quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Hourly != 0.04 || quote.Zones != 4 {
		t.Fatalf("quote=%+v, want mean of the middle pair 0.04 over 4 zones", quote)
	}
}

func TestMedianSpotPriceTiesAndEdges(t *testing.T) {
	tests := []struct {
		name   string
		prices map[string]float64
		want   float64
		zones  int
	}{
		{"single zone", map[string]float64{"a": 0.07}, 0.07, 1},
		{"odd with a tie", map[string]float64{"a": 0.03, "b": 0.03, "c": 0.05}, 0.03, 3},
		{"even with a tie at the middle", map[string]float64{"a": 0.03, "b": 0.03, "c": 0.03, "d": 0.05}, 0.03, 4},
		{"even mean of distinct middles", map[string]float64{"a": 0.01, "b": 0.02, "c": 0.04, "d": 0.09}, 0.03, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newest := map[string]zonePrice{}
			for zone, price := range tt.prices {
				newest[zone] = zonePrice{hourly: price, at: regionalWindowEnd}
			}
			quote, ok := medianSpotPrice(newest)
			if !ok {
				t.Fatal("expected a median")
			}
			if quote.Zones != tt.zones || quote.Hourly < tt.want-1e-12 || quote.Hourly > tt.want+1e-12 {
				t.Fatalf("quote=%+v, want %v over %d zones", quote, tt.want, tt.zones)
			}
		})
	}
	if _, ok := medianSpotPrice(map[string]zonePrice{}); ok {
		t.Fatal("empty input must not produce a median")
	}
}

func TestLatestRegionalSpotPriceNewestRowPerZoneAndOrderIndependent(t *testing.T) {
	older := regionalWindowEnd.Add(-time.Hour)
	forward := []ec2types.SpotPrice{
		regionalRow("us-east-1a", "0.0200", older),
		regionalRow("us-east-1a", "0.0400", regionalWindowEnd),
	}
	reversed := []ec2types.SpotPrice{forward[1], forward[0]}
	for name, rows := range map[string][]ec2types.SpotPrice{"forward": forward, "reversed": reversed} {
		t.Run(name, func(t *testing.T) {
			api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: rows}}
			quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
			if err != nil {
				t.Fatal(err)
			}
			if quote.Hourly != 0.04 || quote.Zones != 1 {
				t.Fatalf("quote=%+v, want the newest 0.04 in one zone", quote)
			}
		})
	}
}

func TestLatestRegionalSpotPriceEqualTimestampKeepsHigherPrice(t *testing.T) {
	for name, rows := range map[string][]ec2types.SpotPrice{
		"low first":  {regionalRow("us-east-1a", "0.0200", regionalWindowEnd), regionalRow("us-east-1a", "0.0400", regionalWindowEnd)},
		"high first": {regionalRow("us-east-1a", "0.0400", regionalWindowEnd), regionalRow("us-east-1a", "0.0200", regionalWindowEnd)},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: rows}}
			quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
			if err != nil {
				t.Fatal(err)
			}
			if quote.Hourly != 0.04 {
				t.Fatalf("hourly=%v, want the higher price 0.04", quote.Hourly)
			}
		})
	}
}

func TestLatestRegionalSpotPriceSkipsUnusableRows(t *testing.T) {
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: []ec2types.SpotPrice{
		{SpotPrice: aws.String("0.0100"), Timestamp: &regionalWindowEnd},
		{AvailabilityZone: aws.String("us-east-1a"), SpotPrice: aws.String("0.0100")},
		regionalRow("us-east-1b", "-0.0100", regionalWindowEnd),
		regionalRow("us-east-1c", "not-a-price", regionalWindowEnd),
		regionalRow("us-east-1d", "0.0500", regionalWindowEnd),
	}}}
	quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Hourly != 0.05 || quote.Zones != 1 {
		t.Fatalf("quote=%+v, want only the usable zone", quote)
	}
}

func TestLatestRegionalSpotPriceFollowsPages(t *testing.T) {
	next := "more"
	api := &pagedSpotAPI{pages: []*ec2.DescribeSpotPriceHistoryOutput{
		{SpotPriceHistory: []ec2types.SpotPrice{regionalRow("us-east-1a", "0.0200", regionalWindowEnd)}, NextToken: &next},
		{SpotPriceHistory: []ec2types.SpotPrice{regionalRow("us-east-1b", "0.0400", regionalWindowEnd)}},
	}}
	quote, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Hourly != 0.03 || quote.Zones != 2 || api.calls != 2 {
		t.Fatalf("quote=%+v calls=%d, want 0.03 over 2 zones in 2 calls", quote, api.calls)
	}
}

func TestLatestRegionalSpotPriceTruncatesAtPageCap(t *testing.T) {
	next := "more"
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{
		SpotPriceHistory: []ec2types.SpotPrice{regionalRow("us-east-1a", "0.0200", regionalWindowEnd)},
		NextToken:        &next,
	}}
	_, err := (&Client{spotAPI: api}).LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
	if !errors.Is(err, ErrSpotHistoryTruncated) {
		t.Fatalf("err=%v, want ErrSpotHistoryTruncated", err)
	}
	if api.calls != pricePageCap {
		t.Fatalf("calls=%d, want %d", api.calls, pricePageCap)
	}
}

func TestLatestRegionalSpotPriceCachesHitAndEmpty(t *testing.T) {
	api := &fakeSpotAPI{output: &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: []ec2types.SpotPrice{
		regionalRow("us-east-1a", "0.0300", regionalWindowEnd),
	}}}
	c := &Client{spotAPI: api}
	for i := 0; i < 2; i++ {
		quote, err := c.LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
		if err != nil || quote.Hourly != 0.03 {
			t.Fatalf("call %d: quote=%+v err=%v", i, quote, err)
		}
	}
	if api.calls != 1 {
		t.Fatalf("calls=%d, want 1 cached lookup", api.calls)
	}

	empty := &fakeSpotAPI{}
	ce := &Client{spotAPI: empty}
	for i := 0; i < 2; i++ {
		_, err := ce.LatestRegionalSpotPrice(context.Background(), "us-east-1", "m5.large", "Linux/UNIX")
		if !errors.Is(err, ErrNoSpotPrice) {
			t.Fatalf("call %d: err=%v, want ErrNoSpotPrice", i, err)
		}
	}
	if empty.calls != 1 {
		t.Fatalf("empty calls=%d, want 1 cached miss", empty.calls)
	}
}
