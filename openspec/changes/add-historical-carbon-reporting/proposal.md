## Why

Issue #29 asks for the carbon footprint of historical AWS usage. Its December
2025 public-API blocker was removed by the AWS Sustainability launch on March
31, 2026. FinFocus's existing offline carbon plugin answers projected EC2
questions and cannot supply this historical inventory.

## What Changes

- Add optional AWS Sustainability reporting, disabled by default, controlled by
  `plugins.aws-ce.carbon_enabled` or `FINFOCUS_AWS_CARBON_ENABLED`.
- Retrieve monthly account/region/service emissions through the official AWS Go
  SDK, preserving accounting method, emissions scope, model version and period.
- Introduce an optional historical-carbon SDK contract and a dedicated FinFocus
  report command; keep aggregate emissions outside resource cost rows.
- Preserve request-scoped credentials, bounded retries and pagination, sanitized
  errors, cancellation, and structured operation logging.
- Verify with offline fixtures and process-level integration. Live AWS checks
  require a separate explicit request.

## Capabilities

### New Capabilities

- `historical-carbon`: Opt-in monthly AWS emissions reports with provenance.

### Modified Capabilities

None. Existing resource costs and projected carbon retain their current meaning.

## Impact

Implementation spans `finfocus-spec` (optional protocol/SDK), this plugin (AWS
reader and setting), and `finfocus` (report client/command). Those companion
changes must be coordinated rather than publishing an unreachable plugin method.
Enabled reports require `sustainability:GetEstimatedCarbonEmissions`.
The separate projected carbon plugin remains offline.
