# Greenops discovery, issue 105

**Status:** researching
**Type:** feature
**Priority:** low

## User story

A sustainability lead needs the carbon footprint of AWS usage to report on environmental, social, and governance goals.

## Technical approach

Investigate whether the plugin can retrieve carbon footprint data through an API.

### Findings as of 2025-12

- **Public API:** AWS didn't provide a public API for the Customer Carbon Footprint Tool as of late 2025.
- **Workarounds:** experimental scripts use screen scraping or browser automation, which makes them fragile.
- **Data exports:** comma-separated values and Parquet exports require S3 bucket access and asynchronous processing.

## Recommendation

Mark this feature as blocked or unsupported for the synchronous gRPC plugin.

- The plugin can't scrape web consoles or wait for S3 exports during a `GetActualCost` call.
- Reassess support if AWS releases a `carbon:GetCarbonFootprint` API.

## Research tasks

- [ ] Confirm whether `finfocus-spec` permits null or zero carbon emissions while remaining compliant. These fields are optional.
- [ ] Document this limitation in `CONTEXT.md`.
