## Why

Issue #33 requires AWS-generated reservation purchase opportunities for RDS,
Redshift, ElastiCache, and OpenSearch. GetRecommendations currently supports
EC2 rightsizing and Savings Plans but does not retrieve non-EC2 reservations.

## What Changes

- Retrieve all pages of GetReservationPurchaseRecommendation for four services.
- Map AWS savings, currency, quantity, instance details, and purchase terms into
  SDK purchase commitment recommendations; preserve upfront costs in metadata.
- Include reservation sources in unfiltered and purchase commitment requests,
  selecting individual services through resource type filters and targets.
- Preserve call-scoped credentials, shared request budgets, 100-page limits,
  SDK filtering and summaries, and atomic failure behavior.
- Document the additional IAM permission, account-level opportunity identities,
  defaults, and EC2 RI exclusion.

## Capabilities

### New Capabilities

- `reservation-recommendations`: Non-EC2 reservation purchase recommendations.

### Modified Capabilities

None.

## Impact

Unfiltered GetRecommendations requests additionally require
ce:GetReservationPurchaseRecommendation. No SDK or dependency changes are needed.
No resources or reservations are purchased, modified, or generated.
