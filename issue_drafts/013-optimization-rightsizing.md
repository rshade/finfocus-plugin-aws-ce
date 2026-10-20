# AWS optimization recommendations, issue #13

**Status:** implemented. The task report records offline verification. Live verification is pending.
**Type:** feature
**Priority:** medium

Issue #13 combines EC2 rightsizing with Compute and EC2 Instance Savings Plans
purchase recommendations through `GetRecommendations`. The approved contract is
[specification 005](../specs/005-aws-ce-recommendations-plugin/spec.md).

The implementation uses cost category and the rightsize, `TERMINATE`, or
purchase commitment action filters from the pinned SDK. Rightsizing supports EC2
only. It maps the first modify target with `DefaultTargetInstance=true`, and uses
the first target only when AWS flags no target. Savings Plans use a 30-day
lookback, one-year term, no upfront payment, and payer scope. AWS supplies
monthly savings. Daily and annual views are display conversions. The plugin
never changes resources or commitments, and never requests recommendation
generation.

See the README for permissions, examples, filtering, pagination, overlapping
savings, and the opt-in live test. RDS, Reserved Instances, SageMaker Savings Plans,
Database Savings Plans, custom scoring, and persistent dismissals remain outside scope.
