package client

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// PricingAPI is the Price List surface this plugin calls for an on-demand rate.
type PricingAPI interface {
	GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

const pricingAPIRegion = "us-east-1"

// ErrPriceListTruncated means GetProducts still had a page token after five pages.
var ErrPriceListTruncated = errors.New("price list exceeded 5 pages")

// OnDemandHourly returns the shared-tenancy on-demand hourly price for an instance type.
// The bool is false when the price list has no matching compute price. AWS errors are returned.
// A price and a confirmed miss are cached for five minutes.
func (c *Client) OnDemandHourly(ctx context.Context, region, instanceType, operatingSystem string) (float64, bool, error) {
	key := region + "\x00" + instanceType + "\x00" + operatingSystem
	if hourly, ok, hit := c.cached(&c.priceCache, key); hit {
		return hourly, ok, nil
	}
	api, err := c.pricingAPIFor()
	if err != nil {
		return 0, false, err
	}
	items, err := readPriceList(ctx, api, region, instanceType, operatingSystem)
	if err != nil {
		return 0, false, err
	}
	hourly, ok := onDemandHourlyFromPriceList(items, instanceType)
	c.store(&c.priceCache, key, hourly, ok)
	return hourly, ok, nil
}

func (c *Client) pricingAPIFor() (PricingAPI, error) {
	if c.pricingAPI != nil {
		return c.pricingAPI, nil
	}
	if c.awsConfig.Credentials == nil {
		return nil, ErrSpotClientUnconfigured
	}
	cfg := c.awsConfig.Copy()
	cfg.Region = pricingAPIRegion
	return pricing.NewFromConfig(cfg, func(options *pricing.Options) {
		options.Region = pricingAPIRegion
		options.Retryer = aws.NopRetryer{}
	}), nil
}

func readPriceList(ctx context.Context, api PricingAPI, region, instanceType, operatingSystem string) ([]string, error) {
	input := &pricing.GetProductsInput{
		ServiceCode:   aws.String("AmazonEC2"),
		FormatVersion: aws.String("aws_v1"),
		MaxResults:    aws.Int32(100),
		Filters: []pricingtypes.Filter{
			termFilter("instanceType", instanceType),
			termFilter("regionCode", region),
			termFilter("operatingSystem", operatingSystem),
			termFilter("tenancy", "Shared"),
			termFilter("preInstalledSw", "NA"),
			termFilter("capacitystatus", "Used"),
			termFilter("licenseModel", "No License required"),
		},
	}
	var items []string
	for page := 0; page < pricePageCap; page++ {
		out, err := api.GetProducts(ctx, input)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return items, nil
		}
		items = append(items, out.PriceList...)
		if out.NextToken == nil || *out.NextToken == "" {
			return items, nil
		}
		input.NextToken = out.NextToken
	}
	return nil, ErrPriceListTruncated
}

func termFilter(field, value string) pricingtypes.Filter {
	return pricingtypes.Filter{
		Type:  pricingtypes.FilterTypeTermMatch,
		Field: aws.String(field),
		Value: aws.String(value),
	}
}

type priceListItem struct {
	Product struct {
		ProductFamily string            `json:"productFamily"`
		Attributes    map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				Unit         string            `json:"unit"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// onDemandHourlyFromPriceList selects the BoxUsage on-demand hourly USD rate.
func onDemandHourlyFromPriceList(items []string, instanceType string) (float64, bool) {
	needle := "BoxUsage:" + instanceType
	for _, raw := range items {
		var item priceListItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			continue
		}
		usage := item.Product.Attributes["usagetype"]
		if item.Product.ProductFamily != "Compute Instance" || strings.Contains(usage, "Host") || !strings.HasSuffix(usage, needle) {
			continue
		}
		for _, term := range item.Terms.OnDemand {
			for _, dimension := range term.PriceDimensions {
				if dimension.Unit != "Hrs" {
					continue
				}
				hourly, err := strconv.ParseFloat(dimension.PricePerUnit["USD"], 64)
				if err != nil || hourly < 0 {
					continue
				}
				return hourly, true
			}
		}
	}
	return 0, false
}
