## ADDED Requirements

### Requirement: Explicit historical carbon opt-in

The plugin SHALL advertise carbon_enabled as a boolean setting backed by
FINFOCUS_AWS_CARBON_ENABLED, default false. Disabled operation SHALL make no
Sustainability calls and SHALL omit the historical carbon capability. Invalid
nonempty boolean values SHALL fail startup. Enabled discovery SHALL make no
AWS calls and SHALL not advertise projected carbon support.

#### Scenario: Feature disabled by default

- **WHEN** carbon_enabled is unset or false
- **THEN** discovery omits historical carbon and a report request fails before credential extraction or AWS access

#### Scenario: Config or environment opt-in

- **WHEN** FinFocus resolves plugins.aws-ce.carbon_enabled or the user sets FINFOCUS_AWS_CARBON_ENABLED to true
- **THEN** the restarted plugin advertises historical carbon and accepts explicit report requests

#### Scenario: Invalid opt-in value

- **WHEN** FINFOCUS_AWS_CARBON_ENABLED contains a nonempty value other than true or false, ignoring case and surrounding whitespace
- **THEN** startup fails with an error naming the setting

### Requirement: Monthly aggregate report contract

The implementation SHALL expose historical emissions through an optional
historical reporting RPC and host report command, separately from monetary
resource rows and projected carbon. Requests SHALL select whole UTC months and
one accounting method and emissions scope. Responses SHALL retain account,
region, service, original period, method, scope, AWS model version and source.

#### Scenario: Historical report

- **WHEN** an enabled caller requests a valid monthly report
- **THEN** the plugin returns AWS-reported estimates grouped by usage account, region and service with complete provenance

#### Scenario: Unsupported period or attribution

- **WHEN** a report requests a partial UTC month or resource attribution
- **THEN** validation rejects the request without issuing an AWS call

### Requirement: Correct units and availability

The plugin SHALL convert MTCO2e to gCO2e using a factor of 1000000 and SHALL
reject malformed, nonfinite or negative values. An absent result SHALL NOT be
reported as zero; an explicitly returned zero SHALL be preserved. Reports
SHALL identify the values as estimates.

#### Scenario: Missing and explicit zero

- **WHEN** one fixture period has no result and another contains an explicit zero
- **THEN** only the explicitly returned result has a zero emissions record

### Requirement: Bounded and credential-isolated AWS access

The reader SHALL use the official AWS Go SDK, the us-east-1 Sustainability
endpoint, request-scoped credentials, bounded retries and cancellation. It
SHALL enforce a 100-page walk limit, reject repeated tokens, and bind opaque
tokens to the query and credential context. It SHALL NOT cache caller reports
across requests or expose raw credential-bearing AWS errors.

#### Scenario: Later page failure

- **WHEN** an AWS report page fails after earlier pages succeeded
- **THEN** the host fails the report without printing a successful partial report

#### Scenario: Token misuse

- **WHEN** a token is replayed with another query or credential context
- **THEN** the request fails without returning another caller's report

### Requirement: Disjoint report aggregation

The host SHALL reject duplicate record identities and mixed accounting methods
or scopes before totaling records. It SHALL NOT add historical reports to
projected estimates, assign aggregate emissions to resources, or introduce
implicit allocation or partial-month proration.

#### Scenario: Duplicate aggregate

- **WHEN** two pages contain the same account, region, service, period, method and scope identity
- **THEN** the report fails instead of multiplying the aggregate

### Requirement: Existing monetary operations remain independent

Ordinary actual cost, projected cost, budgets and recommendations SHALL NOT
invoke Sustainability or require its permission, regardless of the opt-in.

#### Scenario: Carbon enabled during a cost request

- **WHEN** carbon_enabled is true and a caller requests monetary resource costs
- **THEN** the existing monetary path completes without Sustainability calls
