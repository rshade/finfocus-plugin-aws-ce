# Reserved Instance recommendations, issue 109

**Status:** researching
**Type:** feature
**Priority:** low, legacy

## User story

A cloud administrator needs Reserved Instance recommendations for services without Savings Plans support, such as RDS, ElastiCache, Redshift, and OpenSearch.

## Technical approach

Implement the `GetRecommendations` remote procedure call for `ReservedInstances` as a proxy for `costexplorer.GetReservationPurchaseRecommendation`.

### API mapping

- **Remote procedure call:** `GetRecommendations`, with the `RECOMMENDATION_TYPE_RESERVATION` filter.
- **AWS API:** `costexplorer.GetReservationPurchaseRecommendation`.
- **Scope:** RDS, Redshift, ElastiCache, and OpenSearch, identified as `ES`.

## Constraints

1. Forward AWS recommendations without additional calculations.
2. Exclude EC2 Reserved Instance recommendations when the user prefers Savings Plans. Research whether this requires user configuration or whether the plugin should return all AWS recommendations.

## Acceptance criteria

- [ ] Return Reserved Instance recommendations for RDS and other non-compute services.
- [ ] Map each service, such as `AmazonRDS`, to the recommendation record.
