## MODIFIED Requirements

### Requirement: Preserve request isolation and complete-source semantics

Reservation requests SHALL share the RPC's client, credentials, and request
budget. The plugin SHALL use existing retry, cancellation, token repetition, and
100-page guards and SDK filtering, sorting, pagination, and summary helpers. The
opt-in Spot recommendation source defined in `spot-recommendations` is computed
instead of proxied and fails soft per target; its failure SHALL NOT change the
all-or-nothing result of the AWS Cost Explorer sources.

#### Scenario: Account and region filtering

- **WHEN** a request specifies an account and a region
- **THEN** reservation requests use AccountId rather than a positive dimension expression
- **AND** region filtering applies exactly to mapped AWS region values

#### Scenario: Source failure

- **WHEN** any selected reservation source fails or returns malformed data
- **THEN** the RPC fails with the mapped gRPC status and no partial recommendations

#### Scenario: Credentials and budget

- **WHEN** a request supplies credentials or reaches its AWS request budget
- **THEN** reservation calls use only that request's client and consume the same budget as other selected sources

#### Scenario: Spot source failure is isolated

- **WHEN** the opt-in Spot source fails for a target while reservation sources succeed
- **THEN** the RPC still returns the reservation recommendations
