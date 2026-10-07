# Rightsizing recommendations, issue 13

**Status:** researching
**Type:** feature
**Priority:** medium

## User story

A cloud architect needs to identify EC2 instances with low utilization to downsize them and reduce waste.

## Technical approach

Implement the `GetRecommendations` remote procedure call for `RightSizing` as a proxy for `costexplorer.GetRightsizingRecommendation`.

### API mapping

- **Remote procedure call:** `GetRecommendations`, with the `RECOMMENDATION_TYPE_RIGHTSIZING` filter.
- **AWS API:** `costexplorer.GetRightsizingRecommendation`.
- **Services:** EC2 and RDS, where the AWS API supports them in the region.

## Constraints

1. Report instances that AWS flags as idle or underutilized. Don't calculate utilization percentages.
2. Map service and region filters from the core request.
3. Map `TerminateRecommendationDetail` and `ModifyRecommendationDetail` to the FOCUS recommendation structure.

## Acceptance criteria

- [ ] Return EC2 rightsizing recommendations.
- [ ] Distinguish `Terminate` actions from `Modify` actions.
- [ ] Populate potential savings from AWS estimates.
