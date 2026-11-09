# Proposal

## Why

With only aws-ce installed, `finfocus cost projected` shows a plugin error for
every resource `GetProjectedCost` cannot price, such as an S3 bucket or any EC2
instance when Spot comparison is off (issue #108). `Supports` must answer `true`
for those types because the same type-only call gates actual cost. finfocus-spec
v0.7.12 (rshade/finfocus-spec#675) added `SupportsRequest.capability`, so the
plugin can now give a different answer when the host asks about projected cost.

## What Changes

- Bump `github.com/rshade/finfocus-spec` from v0.7.11 to v0.7.12.
- When the request names `PLUGIN_CAPABILITY_PROJECTED_COSTS`, `Supports` applies
  the checks `GetProjectedCost` makes before any AWS call: Spot eligibility when
  the Spot opt-in is on, the `aws:account` / `aws:service` scope, and
  `pluginsdk.ValidateProjectedCostRequest`. A resource that fails them gets
  `supported: false` and a reason.
- `ACTUAL_COSTS`, any other capability, an unspecified capability (older hosts),
  and an unknown value answer exactly as before.

## Capabilities

### Modified Capabilities

- `supports`: answers per requested capability for projected cost.

## Impact

- Code: `go.mod`/`go.sum`, `internal/pricing/calculator.go`, spec version in
  `manifest.json`, `manifest.yaml`, and docs.
- No new AWS calls or IAM actions. Hosts that do not send the field see no change.
