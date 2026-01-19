# Feature Specification: Add ARN to GetActualCostRequest

**Feature Branch**: `002-add-arn-spec`
**Created**: 2025-12-15
**Status**: Draft
**Input**: GitHub Issue #14 - Upstream Spec Update: Add ARN to GetActualCostRequest

## Clarifications

### Session 2025-12-15

- Q: When the plugin detects a mismatch between `resource_id` and the resource portion of the `arn`, what should be the default behavior? → A: Log warning and use ARN as source of truth (proceed with canonical ID)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Query Costs with Cloud Canonical Identifier (Priority: P1)

As a plugin developer querying actual costs for a cloud resource, I need access to the resource's canonical cloud identifier (ARN for AWS) so that I can definitively identify and query costs for the correct resource, even when multiple identifiers exist.

**Why this priority**: This is the core value of the feature - providing robust resource identification for accurate cost tracking. Without the canonical identifier, cost queries may target the wrong resource when short IDs are ambiguous.

**Independent Test**: Can be fully tested by providing an ARN in the request and verifying the plugin can use it for cost lookups. Delivers immediate value for unambiguous resource identification.

**Acceptance Scenarios**:

1. **Given** a GetActualCostRequest with both `resource_id` and `arn` populated, **When** the plugin processes the request, **Then** the plugin has access to both identifiers for cost lookup.
2. **Given** a GetActualCostRequest with only `resource_id` (arn is empty), **When** the plugin processes the request, **Then** the plugin continues to work using only the resource_id (backward compatibility).
3. **Given** a GetActualCostRequest with a valid ARN, **When** the plugin parses the ARN, **Then** it can extract region, account, and resource type information for filtering cost data.

---

### User Story 2 - Identity Verification Between Identifiers (Priority: P2)

As a plugin developer, I need to compare the `resource_id` against the `arn` to verify I am querying costs for the intended resource, preventing cost attribution errors when identifiers from different sources may not align.

**Why this priority**: Identity verification prevents silent cost attribution errors. Without verification, costs could be attributed to the wrong resource if the `resource_id` doesn't match the expected resource in the ARN.

**Independent Test**: Can be tested by providing mismatched `resource_id` and `arn` values and verifying the plugin detects the inconsistency.

**Acceptance Scenarios**:

1. **Given** a GetActualCostRequest where the `resource_id` matches the resource portion of the `arn`, **When** the plugin validates the request, **Then** the verification passes and cost lookup proceeds.
2. **Given** a GetActualCostRequest where the `resource_id` does not match the resource portion of the `arn`, **When** the plugin validates the request, **Then** the plugin logs a warning about the mismatch and uses the ARN as the source of truth for cost lookup.

---

### User Story 3 - Extract Context from ARN for Cost Filtering (Priority: P3)

As a plugin developer querying AWS Cost Explorer, I need to extract region and account information from the ARN to construct accurate cost filters, especially when the `resource_id` is a short ID (e.g., `i-123abc`) that lacks context.

**Why this priority**: ARN parsing enables more precise cost filtering. Short resource IDs don't contain region or account context, which may be needed for accurate Cost Explorer queries.

**Independent Test**: Can be tested by providing an ARN and verifying the plugin correctly parses out region, account, service, and resource identifiers.

**Acceptance Scenarios**:

1. **Given** an ARN in the format `arn:aws:service:region:account:resource`, **When** the plugin parses the ARN, **Then** it can extract the region, account ID, service name, and resource identifier as separate values.
2. **Given** a short `resource_id` like `i-123abc` with a full ARN, **When** the plugin needs to filter by account or region, **Then** it can use values parsed from the ARN.

---

### Edge Cases

- What happens when the ARN format is invalid or malformed? (Addressed by FR-009)
- How does the system handle ARNs from non-AWS cloud providers (e.g., if a future plugin uses this field)?
- What happens when the ARN contains a resource type that doesn't support Cost Explorer cost allocation tags?
- How does the system handle resources that have been deleted (ARN exists but resource doesn't)? (Behavior should align with AWS Cost Explorer API's default for non-existent resources; no special plugin handling required.)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The upstream spec MUST add a new `arn` field to the `GetActualCostRequest` message definition
- **FR-002**: The `arn` field MUST be optional, and plugins MUST maintain backward compatibility by continuing to function correctly when the `arn` is not provided.
- **FR-003**: Plugins MUST be able to access the `arn` field value when processing `GetActualCostRequest`
- **FR-004**: The spec MUST document the expected format for the `arn` field (cloud-specific canonical identifier)
- **FR-005**: The AWS CE plugin MUST be updated to consume the new `arn` field after spec release
- **FR-007**: The plugin SHOULD use the `arn` for cost lookups when available, falling back to `resource_id` when not
- **FR-008**: When `resource_id` and `arn` mismatch, the plugin MUST log a warning and use the ARN as the authoritative source of truth for cost lookup
- **FR-009**: When a malformed or invalid `arn` is provided, the plugin MUST log a warning and proceed with the `resource_id` for cost lookup if available, otherwise return an error.

### Key Entities

- **GetActualCostRequest**: The request message for retrieving actual historical costs. Currently contains `resource_id`, `from_time`, `to_time`, `resource_type`. Will add `arn` as a new field.
- **ARN (Amazon Resource Name)**: The canonical cloud identifier format for AWS resources (format: `arn:partition:service:region:account-id:resource`). Provides complete context for resource identification.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Plugin developers can access the ARN field from GetActualCostRequest messages after updating to the new spec version
- **SC-002**: 100% of existing cost queries continue to work without modification (backward compatibility verified)
- **SC-003**: Cost queries using ARN achieve the same accuracy as queries using resource_id alone
- **SC-004**: Plugin can successfully parse ARN components (region, account, resource) for at least the common AWS resource types (EC2, RDS, S3, Lambda)

## Assumptions

- AWS ARN format is well-defined and stable (follows `arn:partition:service:region:account-id:resource` pattern)
- The calling system (PulumiCost) will populate the ARN field when available from Pulumi state
- The `arn` field will use protobuf field number 5 as specified in the original issue
- ARN parsing logic will be implemented in the plugin, not in the SDK

## Dependencies

- **Blocks**: Issue #11 (GetActualCost implementation) - robust implementation requires ARN field
- **Upstream**: Requires PR to `rshade/finfocus-spec` repository
- **Release sequence**: Spec release → Plugin update → Integration testing

## Out of Scope

- Support for non-AWS canonical identifiers (e.g., Azure Resource IDs, GCP resource names) - future consideration
- Automatic ARN discovery from short resource IDs - plugin responsibility
- Changes to other request message types (GetProjectedCostRequest, etc.)
