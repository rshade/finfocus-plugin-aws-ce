# AWS optimization recommendations, issue #13

**Status:** implemented; offline verification recorded in task report; live verification pending
**Type:** feature
**Priority:** medium

Issue #13 combines EC2 rightsizing with Compute and EC2 Instance Savings Plans
purchase recommendations through `GetRecommendations`. The approved contract is
[specification 005](../specs/005-aws-ce-recommendations-plugin/spec.md).

The implementation uses cost category and rightsize, terminate, or purchase
commitment action filters from the pinned SDK. Rightsizing supports EC2 only
and maps the first AWS-ranked modify target. Savings Plans use a 30-day lookback,
one-year term, no upfront payment, and payer scope. AWS supplies monthly savings;
daily and annual views are display conversions. No resources or commitments
are changed, and recommendation generation is never requested.

See the README for permissions, examples, filtering, pagination, overlapping
savings, and the opt-in live test. RDS, Reserved Instances, SageMaker and Database
Savings Plans, custom scoring, and persistent dismissals remain outside scope.
