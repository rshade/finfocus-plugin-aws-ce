## ADDED Requirements

### Requirement: Decline projected cost the plugin cannot price

When a `Supports` request names `PLUGIN_CAPABILITY_PROJECTED_COSTS`, the plugin
SHALL answer `supported: false` with a reason for any resource `GetProjectedCost`
would reject before an AWS call. Other capabilities, an unspecified capability,
and unknown values SHALL keep the existing answer. Covered by
`TestSupportsProjectedCostCapability` in `internal/pricing/supports_test.go`.

#### Scenario: S3 bucket asked about projected cost

- **WHEN** a host asks whether an `aws:s3/bucket:Bucket` supports projected cost
- **THEN** the plugin returns `supported: false` with a reason

#### Scenario: EC2 instance without Spot comparison

- **WHEN** a host asks about projected cost for an EC2 instance and Spot comparison is off
- **THEN** the plugin returns `supported: false` with a reason naming `FINFOCUS_AWS_CE_SPOT_COMPARISON`

#### Scenario: Eligible EC2 instance with Spot comparison

- **WHEN** Spot comparison is on and the EC2 instance has an instance type and a region or zone
- **THEN** the plugin returns `supported: true` for projected cost

#### Scenario: Actual cost and older hosts are unchanged

- **WHEN** a host asks about actual cost, or sends no capability
- **THEN** the plugin answers exactly as before this change
