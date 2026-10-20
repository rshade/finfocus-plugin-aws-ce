# Commitment mapping

## Purpose

Describe tested FOCUS mapping of AWS commitment costs.

## Requirements

### Requirement: Map identified commitments to FOCUS

The plugin SHALL map a sole reservation or Savings Plan identifier to FOCUS
commitment ID and type, use the selected Cost Explorer metric for cost, and
leave unsupported commitment fields unset. Covered by
`TestCommitmentLineMapsToFocusRecord` in `internal/pricing/commitment_test.go`.

#### Scenario: Reservation line

- **WHEN** a cost row has a reservation ARN and amortized amount
- **THEN** its FOCUS record identifies a Reserved Instance and uses that amount

#### Scenario: Savings Plan line

- **WHEN** a cost row has a Savings Plan ARN and amortized amount
- **THEN** its FOCUS record identifies a Savings Plan and uses that amount

### Requirement: Do not invent commitment metadata

The plugin SHALL leave commitment fields empty for an account-total cost with
no identified commitment. Covered by `TestGetActualCost_CommitmentFieldsUnset`
in `internal/pricing/commitment_test.go`.

#### Scenario: Account total

- **WHEN** an account-total result has no commitment identifier
- **THEN** its FOCUS record has no commitment ID, type, or unit
