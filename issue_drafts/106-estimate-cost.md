# Issue: Research - EstimateCost (What-If) (#106)

**Status:** Researching
**Type:** Feature
**Priority:** Low (Strategic)

## User Story
As a developer, I want to estimate the cost of a resource *before* I deploy it (Shift-Left), using official AWS list prices.

## Technical Thesis
Implement the `EstimateCost` RPC by utilizing the AWS Pricing API (Price List Service) via `pricing:GetProducts`.

### API Mapping
*   **RPC:** `EstimateCost`
*   **AWS API:** `pricing.GetProducts`
*   **Logic:**
    1.  Extract attributes from `ResourceDescriptor` (e.g., `instanceType`, `region`, `operatingSystem`).
    2.  Query `GetProducts` with these filters.
    3.  Parse the returned JSON (Price List) to find the On-Demand price.

## Boundary Guardrails (Hard Constraints)
1.  **No Local Database:** We do not download the huge `index.json` price file. We must query the API live or cache specific SKUs.
2.  **Accuracy:** Disclaimer required: "This is a List Price estimate. It does not include your EDP discounts, Savings Plans, or Spot fluctuations."
3.  **Complexity:** Limit initial support to EC2 and RDS. Complex pricing (e.g., Lambda request tiers, S3 storage classes) is out of scope for v1.

## Research Tasks
- [ ] Verify latency of `GetProducts` for a simple EC2 query.
- [ ] Confirm if we can query by `sku` if the Core provides it.
- [ ] Assess the complexity of parsing the "Terms" JSON blob from the Pricing API.
