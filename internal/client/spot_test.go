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
