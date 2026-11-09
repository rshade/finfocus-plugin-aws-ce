## MODIFIED Requirements

### Requirement: Preserve request isolation and complete-source semantics

Reservation requests SHALL share the RPC's client, credentials, and request budget, and SHALL use the existing retry, cancellation, token, and 100-page guards. The SDK filtering, sorting, pagination, and summary helpers still apply. Each selected source SHALL fail independently: the RPC SHALL return the sources that succeeded and SHALL fail only when every selected source fails. Cancellation, deadline, and request budget exhaustion SHALL fail the whole RPC at once.

#### Scenario: Account and region filtering

- **WHEN** a request specifies an account and a region
- **THEN** reservation requests use AccountId rather than a positive dimension expression
- **AND** region filtering applies exactly to mapped AWS region values

#### Scenario: Source failure

- **WHEN** a selected reservation or Savings Plans source fails or returns malformed data while other selected sources succeed
- **THEN** the RPC returns the recommendations of the succeeded sources
- **AND** the failed source is logged with its source name and no error text from AWS

#### Scenario: Every selected source fails

- **WHEN** every selected source fails
- **THEN** the RPC fails with the mapped gRPC status of the first failure and no recommendations

#### Scenario: Credentials and budget

- **WHEN** a request supplies credentials or reaches its AWS request budget
- **THEN** reservation calls use only that request's client and consume the same budget as other selected sources
- **AND** a `ResourceExhausted` result stops the RPC without further calls

#### Scenario: Spot source failure is isolated

- **WHEN** the opt-in Spot source fails for a target while reservation sources succeed
- **THEN** the RPC still returns the reservation recommendations

#### Scenario: Cost Explorer client unavailable

- **WHEN** the Cost Explorer client cannot be built and the opt-in Spot source has eligible targets
- **THEN** the Spot recommendations are still returned
- **AND** the RPC fails only when no source succeeds
