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

## Permissions

Each feature calls different AWS operations and some need account settings. See
[AWS permissions](permissions.md) for the action table, a ready-to-use policy, and
the trust policy a `role_arn` needs. Assuming a role requires `sts:AssumeRole` on
the source identity.

`GetBudgets` is the exception to host-supplied credentials. It always uses the
process credential chain in `us-east-1`, and ignores request keys and `role_arn`.
The account comes from `FINFOCUS_AWS_ACCOUNT_ID`, or from STS `GetCallerIdentity`
when that is empty.

## Failure handling

AWS access-denied errors return `PermissionDenied`. Expired or invalid AWS
sessions return `Unauthenticated`. Generic per-request client and request
failures return sanitized errors. Missing region configuration returns
`FailedPrecondition` with an `AWS_REGION=us-east-1` setup action. This known-safe
configuration hint contains no credential values. The plugin excludes credential
values from logs and statuses, including diagnostics that echo keys, tokens, or
role ARNs.

## Historical carbon permissions

Only explicit `GetCarbonEmissions` requests need Sustainability permission.
Enabling the setting, reading discovery metadata, and requesting monetary costs
do not call Sustainability. Reports use the same default credential chain and
per-request credential keys described above, including assume-role support.
The service endpoint is `us-east-1`; usage-region filters select the reported
regions without changing that endpoint.

A read-only policy for this report is:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "sustainability:GetEstimatedCarbonEmissions",
    "Resource": "*"
  }]
}
```

Sustainability does not support resource ARN restrictions for this action.
Account filters narrow a query within the caller's existing visibility; they
do not grant access to another account. No S3 export or legacy
`sustainability:GetCarbonFootprintSummary` permission is needed for this API.
See the [AWS authorization reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sustainability.html).
