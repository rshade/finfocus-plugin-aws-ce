# Credentials and assume role

## Purpose

Describe tested request credentials and role assumption behavior.

## Requirements

### Requirement: Per-request AWS credentials

The plugin SHALL use credentials supplied with an actual-cost request for that
request and SHALL keep credential values out of logs. Covered by
`TestGetActualCost_PerRequestCredentials` and
`TestGetActualCost_CredentialShapes` in `internal/pricing/credentials_test.go`.

#### Scenario: Distinct credentials on consecutive requests

- **WHEN** two requests supply different AWS credentials
- **THEN** each request creates a client using its own credentials and logs neither secret

### Requirement: Assume the requested role

The AWS client SHALL use STS AssumeRole when configured with a role ARN. Covered
by `TestNewClient_RoleARNAssumeRoleLoopback` in
`internal/client/assume_role_test.go`.

#### Scenario: Role credential source

- **WHEN** a client is configured with a role ARN
- **THEN** its AWS request uses credentials obtained by assuming that role
