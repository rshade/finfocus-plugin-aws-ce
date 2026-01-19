# Research: Add ARN to GetActualCostRequest

**Date**: 2025-12-15
**Feature**: 002-add-arn-spec

## Research Questions

### 1. AWS ARN Format and Parsing

**Decision**: Use standard ARN format parsing with 6-7 colon-separated components

**Rationale**: AWS ARNs follow a well-documented format. Go's `strings.SplitN` is
sufficient for parsing without external dependencies.

**ARN Format**:

```text
arn:partition:service:region:account-id:resource
arn:partition:service:region:account-id:resource-type/resource-id
arn:partition:service:region:account-id:resource-type:resource-id
```

**Components**:

| Index | Component | Example | Notes |
|-------|-----------|---------|-------|
| 0 | Prefix | `arn` | Always "arn" |
| 1 | Partition | `aws`, `aws-cn`, `aws-us-gov` | Standard AWS partitions |
| 2 | Service | `ec2`, `rds`, `s3`, `lambda` | AWS service namespace |
| 3 | Region | `us-east-1`, `` (empty for global) | May be empty for global resources |
| 4 | Account ID | `123456789012` | 12-digit account ID |
| 5+ | Resource | `instance/i-123abc`, `function:myFunc` | Varies by service |

**Alternatives Considered**:

- AWS SDK ARN parsing: Not available in aws-sdk-go-v2 as standalone utility
- Third-party library: Unnecessary complexity for simple string parsing
- Regex: Overkill for well-structured format

### 2. Protobuf Field Addition Best Practices

**Decision**: Add `string arn = 5;` as optional field with documentation comment

**Rationale**: Protobuf3 fields are optional by default. Field number 5 is the next
available in `GetActualCostRequest`. Adding a new field is backward compatible.

**Current GetActualCostRequest Fields** (from finfocus-spec):

```protobuf
message GetActualCostRequest {
  string resource_id = 1;      // Pulumi/cloud resource identifier
  google.protobuf.Timestamp start = 2;  // Time range start
  google.protobuf.Timestamp end = 3;    // Time range end
  string resource_type = 4;    // Resource type hint
  // Field 5 is next available
}
```

**Proposed Addition**:

```protobuf
  // Cloud canonical identifier (ARN for AWS, Resource ID for Azure/GCP).
  // When provided, takes precedence over resource_id for cost lookups.
  // Format: arn:partition:service:region:account-id:resource
  string arn = 5;
```

**Alternatives Considered**:

- `canonical_id` as field name: Less intuitive for AWS users, ARN is widely understood
- Separate `CanonicalId` message type: Over-engineering for a string field
- Using `oneof`: Unnecessary since both fields can coexist

### 3. Mismatch Detection Strategy

**Decision**: Extract resource identifier from ARN and compare with `resource_id`

**Rationale**: The clarification session established "log warning, use ARN as source
of truth" behavior. Detection involves string comparison of resource portions.

**Implementation Approach**:

```go
// ParseARN extracts components from an AWS ARN string.
type ParsedARN struct {
    Partition string
    Service   string
    Region    string
    AccountID string
    Resource  string  // Full resource portion (may contain / or :)
}

func ParseARN(arn string) (*ParsedARN, error) {
    parts := strings.SplitN(arn, ":", 6)
    if len(parts) < 6 || parts[0] != "arn" {
        return nil, fmt.Errorf("invalid ARN format: %s", arn)
    }
    return &ParsedARN{
        Partition: parts[1],
        Service:   parts[2],
        Region:    parts[3],
        AccountID: parts[4],
        Resource:  parts[5],
    }, nil
}
```

**Mismatch Handling**:

```go
func (c *Calculator) validateIdentifiers(resourceID, arn string) string {
    if arn == "" {
        return resourceID  // No ARN, use resource_id
    }

    parsed, err := ParseARN(arn)
    if err != nil {
        c.logger.Warn().Err(err).Str("arn", arn).Msg("Invalid ARN format, falling back to resource_id")
        return resourceID
    }

    // Check if resource_id appears in the ARN resource portion
    if !strings.Contains(parsed.Resource, resourceID) {
        c.logger.Warn().
            Str("resource_id", resourceID).
            Str("arn_resource", parsed.Resource).
            Msg("Identifier mismatch: using ARN as source of truth")
    }

    return parsed.Resource  // Use ARN resource as lookup key
}
```

**Alternatives Considered**:

- Strict equality check: Too rigid, short IDs may be substrings of ARN resource
- Fail on mismatch: Per clarification, log and proceed with ARN

### 4. Cost Explorer Filter Integration

**Decision**: Use ARN-derived account/region for filter construction when available

**Rationale**: Cost Explorer supports filtering by LINKED_ACCOUNT and REGION dimensions.
ARN provides these values even when resource_id is a short ID like `i-123abc`.

**Filter Enhancement**:

```go
// If ARN provides account/region, add to filter
if parsed != nil {
    if parsed.AccountID != "" {
        filter = addDimensionFilter(filter, types.DimensionLinkedAccount, parsed.AccountID)
    }
    if parsed.Region != "" {
        filter = addDimensionFilter(filter, types.DimensionRegion, parsed.Region)
    }
}
```

**Alternatives Considered**:

- Always require ARN: Breaks backward compatibility
- Ignore ARN context: Loses precision benefit

### 5. Upstream Release Coordination

**Decision**: Sequential release (spec first, then plugin update)

**Rationale**: Plugin cannot consume a field that doesn't exist in the proto. The
release sequence must be: spec PR → spec release → plugin update → plugin release.

**Release Sequence**:

1. Create PR in `rshade/finfocus-spec` adding `arn` field
2. Release new spec version (e.g., v0.5.2 or v0.5.0)
3. Update plugin's `go.mod` to new spec version
4. Implement ARN consumption in plugin
5. Release plugin version

**Alternatives Considered**:

- Parallel development: Causes build failures until spec released
- Monorepo: Not applicable, separate repos by design

## Summary

| Topic | Decision | Complexity |
|-------|----------|------------|
| ARN Parsing | `strings.SplitN` with 6 parts | Low |
| Proto Field | `string arn = 5;` optional | Low |
| Mismatch Handling | Log warning, use ARN | Low |
| Filter Integration | Add account/region from ARN | Low |
| Release Sequence | Spec → Plugin | Sequential |

**All NEEDS CLARIFICATION items resolved. Ready for Phase 1 design.**
