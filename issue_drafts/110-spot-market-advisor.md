# Spot market advisor, issue 110

**Status:** researching
**Type:** feature
**Priority:** low, strategic

## User story

A platform engineer needs to compare on-demand costs with current Spot rates and assess volatility and risk when choosing Spot instances.

## Technical approach

Integrate the EC2 Spot API with upcoming `finfocus-spec` Spot features.

### Scope

1. **Price comparison:**
   - Use `ec2:DescribeSpotPriceHistory` to fetch current Spot rates for specific instance types and availability zones.
   - Compare rates with on-demand prices from `GetProducts` or standard pricing.
2. **Risk analysis:**
   - Investigate sources for `SpotRisk` factors, such as interruption probability.
   - The AWS SDK doesn't expose interruption rates through a standard API call. AWS publishes them in the [Spot Instance Advisor JSON feed](https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json). Research reliable consumption from a Go binary without a scraper.

## Constraints

1. Report price differences without placing bids or launching instances.
2. Use an authoritative risk source. Don't calculate risk from price-history standard deviation unless the specification explicitly defines this method.
3. Keep cache lifetimes short, such as a few minutes, because Spot prices change frequently.

## Research tasks

- [ ] Prototype `DescribeSpotPriceHistory` with `Linux/UNIX` product description and availability zone filters.
- [ ] Investigate the Spot Advisor JSON feed's stability and schema for interruption frequency.
- [ ] Define the mapping from AWS interruption frequency buckets, such as less than 5% or 5% to 10%, to the `finfocus-spec` `SpotRisk` field or enumeration.
