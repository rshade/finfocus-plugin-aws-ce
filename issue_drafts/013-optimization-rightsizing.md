# Issue: Feature - Rightsizing Recommendations (#13)

**Status:** Researching
**Type:** Feature
**Priority:** Medium

## User Story
As a cloud architect, I want to identify underutilized EC2 instances so I can downsize them and reduce waste.

## Technical Thesis
Implement the `GetRecommendations` RPC (scoped to `RightSizing`) by proxying `costexplorer.GetRightsizingRecommendation`.

### API Mapping
*   **RPC:** `GetRecommendations` (Filter: `RECOMMENDATION_TYPE_RIGHTSIZING`)
*   **AWS API:** `costexplorer.GetRightsizingRecommendation`
*   **Supported Services:** EC2, RDS (if supported by AWS API in region).

## Boundary Guardrails
1.  **No Logic:** We do not calculate utilization percentages. We only report what AWS flags as "Idle" or "Underutilized".
2.  **Filtering:** Support basic filtering (Service, Region) as mapped from the Core request.
3.  **Output:** Map AWS `TerminateRecommendationDetail` and `ModifyRecommendationDetail` to the FOCUS Recommendation structure.

## Acceptance Criteria
- [ ] Returns EC2 rightsizing recommendations.
- [ ] Correctly distinguishes between "Terminate" and "Modify" actions.
- [ ] Populates "Potential Savings" based on AWS estimation.
