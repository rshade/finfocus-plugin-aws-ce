# Tasks

## 1. Eligibility and logging

- [x] 1.1 Add a test that a region-only EC2 target is eligible, a target with an instance type and neither zone nor region is skipped with the `no region or availability zone` reason, and a non-EC2 target logs nothing. Verify: `go test ./internal/pricing/ -run SkipReasons` passes.
- [x] 1.2 Add `spotIneligibleReason`, derive `wantsSpotComparison` from it, and log skipped EC2 targets at debug in `GetProjectedCost` and `spotRecommendations`. Verify: `go test ./internal/pricing/` passes, and the old zone-less test is renamed to `TestEC2WithoutZoneDeclinedWhenComparisonOff` with the comparison set explicitly.

## 2. Client regional price

- [x] 2.1 Add client tests for the odd and even median, ties, the newest row per zone, equal timestamps, unusable rows, paging, the page cap, and the cache. Verify: `go test ./internal/client/ -run 'Regional|MedianSpot'` passes.
- [x] 2.2 Implement `LatestRegionalSpotPrice` with its own cache bucket and the median rule. Verify: `go test ./internal/client/` passes.

## 3. Quote and recommendation

- [x] 3.1 Add pricing tests for the region-level projection labels, the explicit-zone metadata, the regional NotFound message, the recommendation labels, the regional id, and the pinned explicit-zone id. Verify: `go test ./internal/pricing/ -run 'Region|Regional|Explicit'` passes.
- [x] 3.2 Route region-level queries through `spotQuoteFor`, set the location in the detail, description, reasoning, and metadata, and use the `region` key in `spotRecommendationID`. Verify: `go test ./internal/pricing/` passes.

## 4. Documentation

- [x] 4.1 Update `README.md`, `docs/recommendations.md`, `docs/api.md`, `docs/configuration.md`, and `CLAUDE.md`. Verify: `make lint-md` passes.

## 5. Verification

- [x] 5.1 Run `make test`, `make lint`, `make test-integration`, and `go test -race` on the spot code. Verify: all pass with no AWS credentials.
