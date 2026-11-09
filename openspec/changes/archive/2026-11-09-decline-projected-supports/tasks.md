## 1. SDK bump

- [x] 1.1 Bump `finfocus-spec` to v0.7.12 and `specVersion` in `calculator.go`, `manifest.json`, and `manifest.yaml`.
  Verify: `go build ./... && go test ./internal/pricing/ -run Info` passes.

## 2. Projected-cost Supports answer (TDD)

- [x] 2.1 Write failing `TestSupportsProjectedCostCapability` cases: S3, EC2 with Spot off, eligible EC2 with Spot on,
  EC2 without an instance type, account and service, service without id, account without sku, other provider,
  actual, unspecified, and an unknown value.
  Verify: the projected declines fail before 2.2.
- [x] 2.2 Add `projectedSupport` in `calculator.go` and call it from `Supports` when `RequestedCapability` is
  `PROJECTED_COSTS` and the type-only answer is supported.
  Verify: `go test ./internal/pricing/` passes.
  Break check: `TestSupports` and `TestSupportsEC2InstanceStaysSupportedForActuals` pass unchanged.

## 3. Documentation

- [x] 3.1 Update the spec version and the `Supports` description in `README.md`, `docs/api.md`, `CLAUDE.md`,
  `CONTEXT.md`, and `ROADMAP.md`.
  Verify: markdownlint reports no new findings on changed files.

## 4. Verification

- [x] 4.1 `make test`, `make lint`, and `openspec validate --all --strict` pass.
