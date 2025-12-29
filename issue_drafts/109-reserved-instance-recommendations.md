# Issue: Feature - Reserved Instance Recommendations (#109)

**Status:** Researching
**Type:** Feature
**Priority:** Low (Legacy)

## User Story
As a cloud admin, I want to see recommendations for Reserved Instances (RIs) for services that do not yet support Savings Plans (e.g., RDS, ElastiCache, Redshift, OpenSearch).

## Technical Thesis
Implement the `GetRecommendations` RPC (scoped to `ReservedInstances`) by proxying `costexplorer.GetReservationPurchaseRecommendation`.

### API Mapping
*   **RPC:** `GetRecommendations` (Filter: `RECOMMENDATION_TYPE_RESERVATION`)
*   **AWS API:** `costexplorer.GetReservationPurchaseRecommendation`
*   **Scope:** RDS, Redshift, ElastiCache, OpenSearch (ES).

## Boundary Guardrails
1.  **No Logic:** We simply pipe the AWS recommendation.
2.  **Exclusion:** Do NOT return EC2 RI recommendations if Savings Plans are preferred (User config might be needed, or just return everything AWS gives).

## Acceptance Criteria
- [ ] Returns RI recommendations for RDS and other non-compute services.
- [ ] correctly maps the specific service (e.g., "AmazonRDS") to the recommendation record.
