# ARN parsing

## Purpose

Describe how the plugin parses supported AWS resource names and rejects malformed input.

## Requirements

### Requirement: Parse AWS ARN components

The plugin SHALL parse the partition, service, region, account ID, and resource
from supported AWS ARN forms, including EC2, S3, Lambda, and IAM. Covered by
`TestParseARN_Valid` in `internal/pricing/arn_test.go`.

#### Scenario: Valid ARN

- **WHEN** an AWS ARN has the expected six-part structure
- **THEN** parsing returns its components, including empty region or account fields where applicable

### Requirement: Reject malformed ARNs

The parser SHALL return an error for empty, non-ARN, and incomplete input.
Covered by `TestParseARN_Invalid` in `internal/pricing/arn_test.go`.

#### Scenario: Invalid ARN

- **WHEN** the input lacks the ARN prefix or required components
- **THEN** parsing returns an error
