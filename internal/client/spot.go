package client

import (
	"context"
	"errors"
	"sort"
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

// RegionalSpotPrice is the region-level Spot price: the median of the latest price in
// each zone that reported one in the window. Zones is how many zones the median used.
type RegionalSpotPrice struct {
	Hourly float64
	Zones  int
}

type cachedHourly struct {
	expires time.Time
	hourly  float64
	zones   int
	ok      bool
}

type zonePrice struct {
	hourly float64
	at     time.Time
}

// LatestSpotPrice returns the newest Spot price in one availability zone.
// DescribeSpotPriceHistory is filtered by instance type, product description, and zone.
// A successful price and a confirmed empty result are cached for five minutes.
func (c *Client) LatestSpotPrice(ctx context.Context, region, instanceType, product, availabilityZone string) (float64, error) {
	key := region + "\x00" + instanceType + "\x00" + product + "\x00" + availabilityZone
	if entry, hit := c.cached(&c.spotCache, key); hit {
		if !entry.ok {
			return 0, ErrNoSpotPrice
		}
		return entry.hourly, nil
	}
	api, err := c.spotAPIFor(region)
	if err != nil {
		return 0, err
	}
	hourly, found, err := readSpotPrice(ctx, api, instanceType, product, availabilityZone)
	if err != nil {
		return 0, err
	}
	c.store(&c.spotCache, key, cachedHourly{hourly: hourly, ok: found})
	if !found {
		return 0, ErrNoSpotPrice
	}
	return hourly, nil
}

// LatestRegionalSpotPrice returns the median of the newest Spot price in each zone of the
// region, for a target that has no availability zone. Zones with no row in the last hour
// are not counted. The median is used instead of the cheapest zone because the zone a
// new instance lands in is not known in advance.
// A successful median and a confirmed empty result are cached for five minutes.
func (c *Client) LatestRegionalSpotPrice(ctx context.Context, region, instanceType, product string) (RegionalSpotPrice, error) {
	key := region + "\x00" + instanceType + "\x00" + product
	if entry, hit := c.cached(&c.regionalSpotCache, key); hit {
		if !entry.ok {
			return RegionalSpotPrice{}, ErrNoSpotPrice
		}
		return RegionalSpotPrice{Hourly: entry.hourly, Zones: entry.zones}, nil
	}
	api, err := c.spotAPIFor(region)
	if err != nil {
		return RegionalSpotPrice{}, err
	}
	quote, found, err := readRegionalSpotPrice(ctx, api, instanceType, product)
	if err != nil {
		return RegionalSpotPrice{}, err
	}
	c.store(&c.regionalSpotCache, key, cachedHourly{hourly: quote.Hourly, zones: quote.Zones, ok: found})
	if !found {
		return RegionalSpotPrice{}, ErrNoSpotPrice
	}
	return quote, nil
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
			zone, price, at, ok := parseSpotRow(row)
			if !ok || zone != availabilityZone || (found && !at.After(newest)) {
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

// readRegionalSpotPrice pages through every zone's history for the window. The page cap is
// the same as the zone query, but the rows span all zones, so reaching it fails the lookup.
func readRegionalSpotPrice(ctx context.Context, api SpotAPI, instanceType, product string) (RegionalSpotPrice, bool, error) {
	start := time.Now().UTC().Add(-time.Hour)
	var token *string
	newest := map[string]zonePrice{}
	for page := 0; page < pricePageCap; page++ {
		out, err := api.DescribeSpotPriceHistory(ctx, &ec2.DescribeSpotPriceHistoryInput{
			InstanceTypes:       []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
			ProductDescriptions: []string{product},
			StartTime:           &start,
			MaxResults:          aws.Int32(100),
			NextToken:           token,
		})
		if err != nil {
			return RegionalSpotPrice{}, false, err
		}
		if out == nil {
			break
		}
		for _, row := range out.SpotPriceHistory {
			zone, price, at, ok := parseSpotRow(row)
			if !ok || !newerZonePrice(newest, zone, price, at) {
				continue
			}
			newest[zone] = zonePrice{hourly: price, at: at}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			quote, found := medianSpotPrice(newest)
			return quote, found, nil
		}
		token = out.NextToken
	}
	return RegionalSpotPrice{}, false, ErrSpotHistoryTruncated
}

// newerZonePrice reports whether a row should replace the zone's current entry. A later
// timestamp wins. On an equal timestamp the higher price wins, so response order cannot
// change the result.
func newerZonePrice(newest map[string]zonePrice, zone string, price float64, at time.Time) bool {
	current, ok := newest[zone]
	if !ok {
		return true
	}
	if at.After(current.at) {
		return true
	}
	return at.Equal(current.at) && price > current.hourly
}

// medianSpotPrice takes the median of the per-zone prices. An odd count uses the middle
// price. An even count uses the mean of the two middle prices. Prices are sorted before
// selection, so map iteration order never affects the result.
func medianSpotPrice(newest map[string]zonePrice) (RegionalSpotPrice, bool) {
	if len(newest) == 0 {
		return RegionalSpotPrice{}, false
	}
	prices := make([]float64, 0, len(newest))
	for _, entry := range newest {
		prices = append(prices, entry.hourly)
	}
	sort.Float64s(prices)
	mid := len(prices) / 2
	hourly := prices[mid]
	if len(prices)%2 == 0 {
		hourly = (prices[mid-1] + prices[mid]) / 2
	}
	return RegionalSpotPrice{Hourly: hourly, Zones: len(prices)}, true
}

// parseSpotRow returns the zone, price, and timestamp of a usable row. A row without a zone,
// a timestamp, or a parseable non-negative price is not usable.
func parseSpotRow(row ec2types.SpotPrice) (string, float64, time.Time, bool) {
	zone := aws.ToString(row.AvailabilityZone)
	if zone == "" || row.Timestamp == nil {
		return "", 0, time.Time{}, false
	}
	hourly, err := strconv.ParseFloat(aws.ToString(row.SpotPrice), 64)
	if err != nil || hourly < 0 {
		return "", 0, time.Time{}, false
	}
	return zone, hourly, row.Timestamp.UTC(), true
}

func (c *Client) cached(bucket *map[string]cachedHourly, key string) (cachedHourly, bool) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := (*bucket)[key]
	if !ok || time.Now().After(entry.expires) {
		return cachedHourly{}, false
	}
	return entry, true
}

func (c *Client) store(bucket *map[string]cachedHourly, key string, entry cachedHourly) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if *bucket == nil {
		*bucket = map[string]cachedHourly{}
	}
	entry.expires = time.Now().Add(PriceCacheTTL)
	(*bucket)[key] = entry
}
