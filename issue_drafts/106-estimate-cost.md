# Cost estimates before deployment, issue 106

**Status:** researching
**Type:** feature
**Priority:** low, strategic

## User story

A developer needs to estimate resource costs before deployment using official AWS list prices.

## Technical approach

Implement the `EstimateCost` remote procedure call through the Price List Service in the AWS Pricing API and `pricing:GetProducts`.

### API mapping

- **Remote procedure call:** `EstimateCost`.
- **AWS API:** `pricing.GetProducts`.
- **Processing steps:**
  1. Extract attributes from `ResourceDescriptor`, such as `instanceType`, `region`, and `operatingSystem`.
  2. Query `GetProducts` with these filters.
  3. Parse the returned price list JSON to find the on-demand price.

## Constraints

1. Query the API live or cache specific stock keeping units. Don't download the full `index.json` price file.
2. Include this qualification: this list price estimate excludes enterprise discount program discounts, Savings Plans, and Spot price fluctuations.
3. Limit initial support to EC2 and RDS. Version 1 excludes complex pricing. Examples include Lambda request tiers and S3 storage classes.

## Research tasks

- [ ] Measure `GetProducts` latency for a simple EC2 query.
- [ ] Check whether the plugin can query by `sku` when the core provides it.
- [ ] Assess the complexity of parsing the `Terms` JSON object from the Pricing API.
