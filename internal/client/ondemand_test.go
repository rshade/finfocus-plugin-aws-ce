package client

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

func TestOnDemandHourlyFromPriceListSelectsBoxUsage(t *testing.T) {
	items := []string{
		`{"product":{"productFamily":"Compute Instance","attributes":{"usagetype":"HostBoxUsage:m5.large"}},"terms":{"OnDemand":{"t":{"priceDimensions":{"d":{"unit":"Hrs","pricePerUnit":{"USD":"1.00"}}}}}}}`,
		`{"product":{"productFamily":"Compute Instance","attributes":{"usagetype":"BoxUsage:m5.large"}},"terms":{"OnDemand":{"t":{"priceDimensions":{"d":{"unit":"Hrs","pricePerUnit":{"USD":"0.096"}}}}}}}`,
	}
	hourly, ok := onDemandHourlyFromPriceList(items, "m5.large")
	if !ok || hourly != 0.096 {
		t.Fatalf("hourly=%v ok=%v", hourly, ok)
	}
}

type fakePricingAPI struct {
	input *pricing.GetProductsInput
}

func (f *fakePricingAPI) GetProducts(_ context.Context, params *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	f.input = params
	return &pricing.GetProductsOutput{PriceList: []string{
		`{"product":{"productFamily":"Compute Instance","attributes":{"usagetype":"USE1-BoxUsage:m5.large"}},"terms":{"OnDemand":{"t":{"priceDimensions":{"d":{"unit":"Hrs","pricePerUnit":{"USD":"0.096"}}}}}}}`,
	}}, nil
}

func TestOnDemandHourlyFiltersRegionAndOperatingSystem(t *testing.T) {
	api := &fakePricingAPI{}
	hourly, ok, err := (&Client{pricingAPI: api}).OnDemandHourly(context.Background(), "us-east-1", "m5.large", "Linux")
	if err != nil || !ok || hourly != 0.096 {
		t.Fatalf("hourly=%v ok=%v err=%v", hourly, ok, err)
	}
	got := map[string]string{}
	for _, filter := range api.input.Filters {
		if filter.Type != pricingtypes.FilterTypeTermMatch {
			t.Fatalf("filter type %s", filter.Type)
		}
		got[aws.ToString(filter.Field)] = aws.ToString(filter.Value)
	}
	if got["instanceType"] != "m5.large" || got["regionCode"] != "us-east-1" || got["operatingSystem"] != "Linux" || got["tenancy"] != "Shared" {
		t.Fatalf("filters=%v", got)
	}
	if aws.ToString(api.input.ServiceCode) != "AmazonEC2" {
		t.Fatalf("service=%s", aws.ToString(api.input.ServiceCode))
	}
}
