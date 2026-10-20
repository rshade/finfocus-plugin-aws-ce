package client

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// SpotAPI is the EC2 surface this plugin calls for a Spot price.
type SpotAPI interface {
	DescribeSpotPriceHistory(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// ErrNoSpotPrice means EC2 returned no price for the instance type, product, and zone.
var ErrNoSpotPrice = errors.New("no spot price")

// ErrSpotClientUnconfigured means the client has neither an EC2 or Pricing API nor credentials.
var ErrSpotClientUnconfigured = errors.New("spot or price list client is not configured")

// ErrSpotHistoryTruncated means DescribeSpotPriceHistory still had a page token after five pages.
var ErrSpotHistoryTruncated = errors.New("spot price history exceeded 5 pages")

const pricePageCap = 5

// PriceCacheTTL is how long Spot and on-demand hourly rates stay cached.
const PriceCacheTTL = 5 * time.Minute

type cachedHourly struct {
	expires time.Time
	hourly  float64
	ok      bool
}

// LatestSpotPrice returns the newest Spot price in one availability zone.
// DescribeSpotPriceHistory is filtered by instance type, product description, and zone.
// A successful price and a confirmed empty result are cached for five minutes.
func (c *Client) LatestSpotPrice(ctx context.Context, region, instanceType, product, availabilityZone string) (float64, error) {
	key := region + "\x00" + instanceType + "\x00" + product + "\x00" + availabilityZone
	if hourly, ok, hit := c.cached(&c.spotCache, key); hit {
		if !ok {
			return 0, ErrNoSpotPrice
		}
		return hourly, nil
	}
	api, err := c.spotAPIFor(region)
	if err != nil {
		return 0, err
	}
	hourly, found, err := readSpotPrice(ctx, api, instanceType, product, availabilityZone)
	if err != nil {
		return 0, err
	}
	c.store(&c.spotCache, key, hourly, found)
	if !found {
		return 0, ErrNoSpotPrice
	}
	return hourly, nil
}

func (c *Client) spotAPIFor(region string) (SpotAPI, error) {
	if c.spotAPI != nil {
		return c.spotAPI, nil
	}
	if c.awsConfig.Credentials == nil {
		return nil, ErrSpotClientUnconfigured
	}
	cfg := c.awsConfig.Copy()
	cfg.Region = region
	return ec2.NewFromConfig(cfg, func(options *ec2.Options) {
		options.Retryer = aws.NopRetryer{}
	}), nil
}

func readSpotPrice(ctx context.Context, api SpotAPI, instanceType, product, availabilityZone string) (float64, bool, error) {
	start := time.Now().UTC().Add(-time.Hour)
	var token *string
	var newest time.Time
	var hourly float64
	found := false
	for page := 0; page < pricePageCap; page++ {
		out, err := api.DescribeSpotPriceHistory(ctx, &ec2.DescribeSpotPriceHistoryInput{
			InstanceTypes:       []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
			ProductDescriptions: []string{product},
			AvailabilityZone:    aws.String(availabilityZone),
			StartTime:           &start,
			MaxResults:          aws.Int32(100),
			NextToken:           token,
		})
		if err != nil {
			return 0, false, err
		}
		if out == nil {
			return hourly, found, nil
		}
		for _, row := range out.SpotPriceHistory {
			price, at, ok := spotRow(row, availabilityZone)
			if !ok || (found && !at.After(newest)) {
				continue
			}
			hourly = price
			newest = at
			found = true
		}
		if out.NextToken == nil || *out.NextToken == "" {
			return hourly, found, nil
		}
		token = out.NextToken
	}
	return 0, false, ErrSpotHistoryTruncated
}

func spotRow(row ec2types.SpotPrice, availabilityZone string) (float64, time.Time, bool) {
	if aws.ToString(row.AvailabilityZone) != availabilityZone || row.Timestamp == nil {
		return 0, time.Time{}, false
	}
	hourly, err := strconv.ParseFloat(aws.ToString(row.SpotPrice), 64)
	if err != nil || hourly < 0 {
		return 0, time.Time{}, false
	}
	return hourly, row.Timestamp.UTC(), true
}

func (c *Client) cached(bucket *map[string]cachedHourly, key string) (float64, bool, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := (*bucket)[key]
	if !ok || time.Now().After(entry.expires) {
		return 0, false, false
	}
	return entry.hourly, entry.ok, true
}

func (c *Client) store(bucket *map[string]cachedHourly, key string, hourly float64, ok bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if *bucket == nil {
		*bucket = map[string]cachedHourly{}
	}
	(*bucket)[key] = cachedHourly{expires: time.Now().Add(PriceCacheTTL), hourly: hourly, ok: ok}
}
