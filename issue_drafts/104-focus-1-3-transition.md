# Issue: Research - FOCUS 1.3 Transition (#104)

**Status:** Researching
**Type:** Chore / Compliance
**Priority:** Medium

## User Story
As a FinOps practitioner, I want my data to include "Commitment" details (FOCUS 1.3) so I can analyze my Reserved Instance and Savings Plan effective rates.

## Technical Thesis
Audit the `finfocus-spec` FOCUS 1.3 columns against AWS Cost Explorer data availability.

### Scope
*   **New Columns:** `CommitmentDiscountCategory`, `CommitmentDiscountId`, `CommitmentDiscountName`, `CommitmentDiscountType`.
*   **AWS Mapping:**
    *   `ReservationARN` -> `CommitmentDiscountId` (for RI)
    *   `SavingsPlanARN` -> `CommitmentDiscountId` (for SP)
    *   `"Savings Plan"` / `"Reserved Instance"` -> `CommitmentDiscountType`

## Boundary Guardrails
1.  **Strict Mapping:** Only populate these fields if `GetCostAndUsage` returns them (e.g., in `GroupByKey` or `ResultsByTime`).
2.  **No Inference:** Do not assume a discount is an RI based on price. Must rely on the `ReservationARN` field presence.

## Research Tasks
- [ ] Verify if `GetCostAndUsage` returns `ReservationARN` and `SavingsPlanARN` when grouping by `Service`. (Likely requires grouping by `RESERVATION_ID` or similar).
- [ ] Check if enabling these groupings explodes the cardinality of the response (row count).
