# Issue: Research - Spot Market Advisor (#110)

**Status:** Researching
**Type:** Feature
**Priority:** Low (Strategic)

## User Story
As a platform engineer, I want to compare On-Demand costs with real-time Spot Market rates and understand the associated volatility/risk, so I can make informed decisions about using Spot instances for my workloads.

## Technical Thesis
Implement the "Spot Market Advisor" capabilities by integrating with EC2 Spot APIs and aligned with upcoming `finfocus-spec` Spot features.

### Scope
1.  **Real-Time Arbitrage:**
    *   Use `ec2:DescribeSpotPriceHistory` to fetch current spot rates for specific Instance Types and AZs.
    *   Compare these against the On-Demand rates (retrieved via `GetProducts` or standard pricing).
2.  **Risk Analysis:**
    *   Investigate methods to populate `SpotRisk` factors (e.g., Interruption Probability).
    *   *Note:* AWS SDK does not directly expose "Interruption Rate" via a standard API call. It is typically published via the [Spot Instance Advisor JSON feed](https://spot-bid-advisor.s3.amazonaws.com/spot-advisor-data.json). Research is needed on how to consume this reliably in a Go binary without embedding a scraper.

## Boundary Guardrails
1.  **No "Trade" Logic:** We report the price difference. We do not automatically "bid" or launch instances.
2.  **Risk Data Source:** We must find an authoritative source for "Risk". We will NOT calculate risk based on our own volatility math (standard deviation of price history) unless explicitly defined by the Spec as the standard method.
3.  **Data Freshness:** Spot prices change frequently. Caching strategies must be short-lived (e.g., minutes).

## Research Tasks
- [ ] Prototype `DescribeSpotPriceHistory` call with filters for Product Description (Linux/UNIX) and AZ.
- [ ] Investigate the stability and schema of the Spot Advisor JSON feed for "Interruption Frequency" data.
- [ ] Define how to map AWS "Frequency of Interruption" buckets (e.g., "<5%", "5-10%") to the `finfocus-spec` `SpotRisk` enum/field.
