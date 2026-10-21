## Context

The existing GetRecommendations RPC collects rightsizing and Savings Plans using
a single request-scoped Cost Explorer client and budget. It maps records before
SDK filtering, sorting, pagination, and summary calculation. Issue #33 extends
that flow with non-EC2 reservation purchase opportunities.

## Goals / Non-Goals

Goals: proxy RDS, Redshift, ElastiCache, and OpenSearch recommendations; retain
AWS savings and identity details; preserve the SDK response contract and existing
error, credential, budget, and pagination semantics.

Non-goals: EC2 RIs, MemoryDB, DynamoDB capacity, local utilization or ROI
calculations, recommendation generation, purchases, and configurable purchase
parameters.

## Decisions

Use a separate optional ReservationRecommendationsAPI interface so existing
clients implementing only rightsizing and Savings Plans remain compatible.
Reuse recommendationPages for retries, cancellation, page hooks, token guards,
and the 100-page limit. Requests use billing service names, PAYER scope,
THIRTY_DAYS, ONE_YEAR, and NO_UPFRONT. Account filters use the API AccountId
field; other filters apply after mapping rather than using unsupported AWS
reservation filter expressions.

Expose PURCHASE_COMMITMENT actions with reserved_instance commitment type and
AWS's recommended instance quantity. Account-level resource types are
rds:Account, redshift:Account, elasticache:Account, and es:Account. Short service
names and AWS service codes canonicalize to those types. IDs identify purchase
opportunities, not existing database, cluster, or reservation resources.

Use AWS EstimatedMonthlySavingsAmount and currency. Keep upfront cost, recurring
cost, break-even months, original quantity, service code, billing service name,
scope, terms, and instance details in metadata. Currency falls back to the AWS
group summary. Missing account identity is permitted only for payer-scoped
recommendations and is labeled payer without inventing an account ID.

Hash service, account, scope, terms, lookback, and all AWS instance detail fields
for stable recommendation IDs that distinguish engines, licenses, deployment
options, regions, and node types. Generation timestamps do not affect IDs.

Reservation sources join unfiltered and purchase commitment requests. Resource
type and target intersections select services before credentials or AWS access.
Optimization-specific resource types suppress unrelated anomaly access. A
failure in any selected source fails the complete response.

## Risks / Trade-offs

Unfiltered requests make four additional source calls and require a new IAM
action. Service-specific filters avoid unrelated permissions and datasets.
Savings opportunities can overlap and are not guaranteed additive savings.
AWS-provided region values are preserved; region filters use exact matching.
Offline fixtures verify the protocol but cannot establish live dataset access.

## Validation

Run client pagination, cancellation, retry, budget, page-cap, and error tests;
service mapping and stable identity tests; source and target selection tests;
real AWS JSON serialization/deserialization through SDK gRPC for all four
services; SDK empty-response tests; and the full offline suite and linters.
