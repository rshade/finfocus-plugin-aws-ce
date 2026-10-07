# AWS credentials

A request with no host-supplied credentials uses the AWS SDK default credential
chain. It supports environment variables and shared AWS profiles. Set `AWS_REGION`
to a region such as `us-east-1` and authenticate with your usual AWS profile or
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and optional `AWS_SESSION_TOKEN`.
Cost Explorer uses its global endpoint. Region configuration doesn't select
an independent regional billing dataset.

## Per-request credential keys

The FinFocus host can supply these plugin-specific names through the SDK's
credential context or credential metadata transport:

| Key | Meaning | Requirement |
| --- | --- | --- |
| `access_key_id` | AWS access key id | Supply with `secret_access_key` |
| `secret_access_key` | Matching AWS secret access key | Supply with `access_key_id` |
| `session_token` | AWS temporary-session token | Optional. Requires both access and secret keys |
| `role_arn` | IAM role ARN to assume using STS | Optional with keys, or alone using the default AWS chain |

Names are case-insensitive in the SDK. Unsupported names, incomplete key pairs,
whitespace-only values and malformed IAM role ARNs return `InvalidArgument`
with `ERROR_CODE_INVALID_CREDENTIALS`. A role ARN must identify an IAM role in
a 12-digit account. The service verifies its existence and permissions.

A nonempty invalid credential set never falls back to process credentials.
Each valid credential-bearing request builds its own client and bypasses the
shared cache, so another request can't reuse its costs or credentials.

## Permissions and failure handling

Allow `ce:GetDimensionValues` for commitment discovery,
`ce:GetCostAndUsage` for service totals and
`ce:GetCostAndUsageWithResources` for EC2 resource queries. Resource-level
queries require the account's Cost Explorer resource opt-in and cover only
the last 14 days. Assuming a role also requires `sts:AssumeRole` permission
and an appropriate role trust policy.

AWS access-denied errors return `PermissionDenied`. Expired or invalid AWS
sessions return `Unauthenticated`. Generic per-request client and request
failures return sanitized errors. Missing region configuration returns
`FailedPrecondition` with an `AWS_REGION=us-east-1` setup action. This known-safe
configuration hint contains no credential values. The plugin excludes credential values from logs
and statuses, including diagnostics that echo keys, tokens, or role ARNs.
