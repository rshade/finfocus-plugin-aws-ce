# Data Model: Add ARN to GetActualCostRequest

**Date**: 2025-12-15
**Feature**: 002-add-arn-spec

## Entities

### GetActualCostRequest (Modified)

The protobuf message for requesting actual historical costs from a plugin.

| Field | Type | Number | Required | Description |
|-------|------|--------|----------|-------------|
| resource_id | string | 1 | Yes | Pulumi/cloud resource identifier (short ID or URN) |
| start | google.protobuf.Timestamp | 2 | Yes | Time range start (inclusive) |
| end | google.protobuf.Timestamp | 3 | Yes | Time range end (exclusive) |
| resource_type | string | 4 | No | Resource type hint for cost attribution |
| **arn** | **string** | **5** | **No** | **NEW: Cloud canonical identifier (ARN for AWS)** |

**Validation Rules**:

- `arn`: If provided, must start with `arn:` prefix
- `arn`: If invalid format, log warning and fall back to `resource_id`
- When both `resource_id` and `arn` are provided, ARN is source of truth

### ParsedARN (New - Plugin Internal)

Internal Go struct for parsed ARN components. Not exposed via gRPC.

| Field | Type | Description |
|-------|------|-------------|
| Partition | string | AWS partition (`aws`, `aws-cn`, `aws-us-gov`) |
| Service | string | AWS service namespace (`ec2`, `rds`, `s3`, etc.) |
| Region | string | AWS region or empty for global resources |
| AccountID | string | 12-digit AWS account ID |
| Resource | string | Resource identifier (format varies by service) |

**State Transitions**: N/A (stateless data structure)

## Relationships

```text
GetActualCostRequest
    │
    ├── resource_id (existing)
    │       │
    │       └── Used for cost lookup when arn is empty
    │
    └── arn (NEW)
            │
            ├── Parsed into ParsedARN components
            │
            ├── Resource portion compared with resource_id
            │       │
            │       ├── Match: Proceed with lookup
            │       └── Mismatch: Log warning, use ARN
            │
            └── AccountID/Region used for filter enhancement
```

## Data Volume Assumptions

- ARN string length: 50-200 characters typical
- Parse operation: O(1) string split
- Memory overhead: Negligible (single string field per request)
- No persistence: Request-scoped only
