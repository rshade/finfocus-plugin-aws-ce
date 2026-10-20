# Cost Explorer errors

## Purpose

Describe tested mapping and redaction of Cost Explorer API errors.

## Requirements

### Requirement: Map AWS API error codes to gRPC status

The plugin SHALL map Cost Explorer rate limits, access denial, expired tokens,
validation errors, unavailable data, and unknown service errors to the gRPC
status codes exercised by `TestGetActualCost_AWSErrorCodes` in
`internal/pricing/aws_error_test.go`. It SHALL exclude AWS error bodies and
credential material from returned status and logs.

#### Scenario: AWS rate limit

- **WHEN** Cost Explorer returns `LimitExceededException`
- **THEN** the plugin returns `ResourceExhausted` with a rate-limit error detail

#### Scenario: AWS access denied

- **WHEN** Cost Explorer returns `AccessDenied`
- **THEN** the plugin returns `PermissionDenied` without exposing the AWS error body

#### Scenario: Expired token

- **WHEN** Cost Explorer returns `ExpiredToken`
- **THEN** the plugin returns `Unauthenticated` with an invalid-credentials detail

#### Scenario: Invalid AWS query

- **WHEN** Cost Explorer returns `ValidationException`
- **THEN** the plugin returns `InvalidArgument`

#### Scenario: Unavailable AWS data

- **WHEN** Cost Explorer returns `DataUnavailableException`
- **THEN** the plugin returns `NotFound` with a resource-not-found detail

#### Scenario: Unknown AWS error

- **WHEN** Cost Explorer returns an unrecognized service error
- **THEN** the plugin returns `Internal` without exposing the AWS error body
