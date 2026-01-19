# Issue: Research - Greenops Discovery (#105)

**Status:** Researching
**Type:** Feature
**Priority:** Low

## User Story
As a sustainability lead, I want to see the Carbon Footprint of my AWS usage to report on ESG goals.

## Technical Thesis
Investigate the feasibility of retrieving Carbon Footprint data programmatically.

### Findings (Dec 2025)
*   **No Public API:** AWS does **not** provide a public API for the Customer Carbon Footprint Tool as of late 2025.
*   **Workarounds:** "Experimental scripts" exist but are brittle (screen scraping/browser automation).
*   **Data Exports:** CSV/Parquet exports are available but require S3 bucket access and async processing.

## Recommendation
**Mark as "Blocked / Not Supported"** for the synchronous gRPC plugin.
*   *Why:* We cannot scrape web consoles. We cannot wait for S3 exports in a real-time `GetActualCost` call.
*   *Future:* Re-evaluate only if AWS releases a `carbon:GetCarbonFootprint` API.

## Research Tasks
- [ ] Confirm if `finfocus-spec` allows "Carbon Emissions" to be `null/zero` while still being compliant. (Yes, optional).
- [ ] Document this limitation in `CONTEXT.md` explicitly.
