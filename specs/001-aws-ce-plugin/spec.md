# Feature Specification: AWS Cost Explorer Plugin for PulumiCost

**Feature Branch**: `001-aws-ce-plugin`
**Created**: 2025-12-05
**Status**: Draft
**Input**: User description: "Create pulumicost-plugin-aws-costexplorer for real AWS billing data"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Retrieve Historical Billing Data (Priority: P1)

As a cloud cost analyst, I want to retrieve actual historical billing data from AWS Cost Explorer so that I can understand real spending patterns and validate cost optimization efforts.

**Why this priority**: This is the core value proposition of the plugin. Without access to actual AWS billing data, the plugin provides no differentiated value over the existing public pricing plugin.

**Independent Test**: Can be fully tested by configuring AWS credentials and querying cost data for a specific date range. Delivers immediate value by showing actual spending.

**Acceptance Scenarios**:

1. **Given** valid AWS credentials with Cost Explorer permissions, **When** I request actual costs for the past 30 days, **Then** I receive a breakdown of costs by service
2. **Given** valid AWS credentials, **When** I request costs for a specific date range (e.g., 2024-01-01 to 2024-03-31), **Then** I receive accurate cost data matching the AWS Console
3. **Given** invalid or missing AWS credentials, **When** I attempt to retrieve costs, **Then** I receive a clear error message explaining the authentication failure

---

### User Story 2 - Query Costs by Dimensions (Priority: P2)

As a finance team member, I want to group and filter costs by various dimensions (service, account, tags, availability zone) so that I can allocate costs to specific projects, teams, or business units.

**Why this priority**: Cost allocation is essential for enterprise cost management, enabling chargeback and showback reporting. This builds on P1 by adding critical filtering capabilities.

**Independent Test**: Can be tested by querying costs grouped by a specific dimension (e.g., "service" or "tag:Environment") and verifying the breakdown matches AWS Console.

**Acceptance Scenarios**:

1. **Given** valid credentials and cost data exists, **When** I request costs grouped by service, **Then** I receive costs itemized by each AWS service used
2. **Given** valid credentials and tagged resources, **When** I request costs grouped by a specific tag key, **Then** I receive costs allocated to each tag value
3. **Given** a multi-account AWS organization, **When** I request costs grouped by account, **Then** I receive costs broken down by each member account

---

### User Story 3 - View Reserved Instance and Savings Plan Discounts (Priority: P3)

As a cloud architect, I want to see the impact of Reserved Instances and Savings Plans on my costs so that I can validate that purchased commitments are being utilized effectively.

**Why this priority**: Understanding RI/SP utilization is valuable but secondary to basic cost retrieval and allocation. This provides optimization insights beyond raw cost data.

**Independent Test**: Can be tested by querying reservation coverage and utilization metrics, comparing against AWS Cost Explorer reservation reports.

**Acceptance Scenarios**:

1. **Given** an account with active Reserved Instances, **When** I request reservation utilization, **Then** I see the percentage of RI hours utilized
2. **Given** an account with Savings Plans, **When** I request savings plan coverage, **Then** I see how much of my spend is covered by Savings Plans
3. **Given** no reservations or savings plans, **When** I request reservation data, **Then** I receive an appropriate response indicating no active commitments

---

### User Story 4 - Handle API Rate Limits Gracefully (Priority: P4)

As a developer integrating with PulumiCost, I want the plugin to handle AWS API rate limits gracefully so that my cost analysis workflows don't fail unexpectedly.

**Why this priority**: Operational reliability is important but becomes critical only at scale. Initial users may not hit rate limits, but enterprise adoption requires this resilience.

**Independent Test**: Can be tested by simulating rate-limited API responses and verifying the plugin retries appropriately without data loss.

**Acceptance Scenarios**:

1. **Given** the AWS Cost Explorer API returns a rate limit error, **When** the plugin receives this error, **Then** it automatically retries with exponential backoff
2. **Given** multiple consecutive rate limit errors, **When** the maximum retry count is exceeded, **Then** a clear error is returned explaining the rate limit issue

---

### Edge Cases

- What happens when the requested date range has no cost data (e.g., before account creation)?
- How does the system handle accounts without Cost Explorer enabled?
- What happens when a requested dimension (e.g., tag key) doesn't exist in the data?
- How does the plugin respond when querying future dates?
- What happens when Cost Explorer returns partial data due to ongoing billing processing?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST authenticate with AWS using standard credential providers (environment variables, shared credentials file, IAM roles)
- **FR-002**: System MUST retrieve historical cost data from AWS Cost Explorer API
- **FR-003**: System MUST support date range queries with start and end dates
- **FR-004**: System MUST support grouping costs by standard dimensions (SERVICE, LINKED_ACCOUNT, TAG, AZ)
- **FR-005**: System MUST return costs in USD currency format
- **FR-006**: System MUST implement the PulumiCost plugin SDK interface (gRPC)
- **FR-007**: System MUST return appropriate errors when GetProjectedCost is called (actual costs only)
- **FR-008**: System MUST translate AWS API errors into meaningful user-facing error messages
- **FR-009**: System MUST implement retry logic with exponential backoff for rate-limited requests
- **FR-010**: System MUST support querying reservation utilization and coverage data
- **FR-011**: System MUST support daily and monthly granularity for cost queries
- **FR-012**: System MUST implement hybrid caching: in-memory cache while running, persist to disk on shutdown, load from disk on startup
- **FR-013**: System MUST use filesystem timestamps to determine cache freshness
- **FR-014**: System MUST validate date range requests do not exceed 14 months lookback and return helpful error if exceeded
- **FR-015**: System MUST include `FallbackHint` enum value in gRPC response (RECOMMENDED when returning empty results, NONE when returning data) to signal core whether to try fallback plugins (blocked by: [pulumicost-spec#124](https://github.com/rshade/pulumicost-spec/issues/124))

### Security Requirements

- **SR-001**: System MUST implement comprehensive input validation to prevent malformed requests from causing issues
- **SR-002**: System MUST isolate AWS credentials using standard SDK credential chain only (no custom credential storage)
- **SR-003**: System MUST implement audit logging of all API access attempts for security monitoring
- **SR-004**: System MUST serve gRPC on loopback interface only (127.0.0.1) to prevent unauthorized network access
- **SR-005**: System MUST use read-only AWS Cost Explorer permissions (ce:GetCostAndUsage, ce:GetCostForecast)
- **SR-006**: System MUST use ZeroLog as defined by the pluginsdk for structured JSON logging with request tracing, performance metrics, and error monitoring

### Key Entities

- **CostEntry**: Represents a single cost data point with timestamp, amount, currency, and associated dimensions/tags
- **ResourceDescriptor**: Identifies the AWS resource or scope being queried (from plugin SDK)
- **DateRange**: Defines the time period for cost queries with start and end timestamps
- **Dimension**: A grouping category for costs (service, account, tag key, availability zone)
- **ReservationData**: Information about Reserved Instance or Savings Plan utilization and coverage
- **CacheEntry**: Cached cost query result with query parameters as key, uses filesystem modification time for freshness
- **FallbackHint**: Enum signaling whether core should try fallback plugins (NONE, RECOMMENDED, REQUIRED)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Cost calculations match AWS Console values within 1% accuracy
- **SC-002**: System returns cost data within 5 seconds for typical queries (single month, single dimension)
- **SC-003**: Plugin successfully authenticates and retrieves data using all standard AWS credential methods
- **SC-004**: 95% of API requests succeed without user intervention when proper credentials are configured
- **SC-005**: Rate-limited requests are automatically retried and succeed within 30 seconds
- **SC-006**: All functional requirements have corresponding automated tests with mocked AWS APIs
- **SC-007**: Plugin supports at least 100 concurrent gRPC requests with bounded memory usage

## Clarifications

### Session 2025-12-05

- Q: Should the plugin cache Cost Explorer responses? → A: Hybrid caching - in-memory while plugin is alive, persist to disk on close, load from disk on startup. Use filesystem timestamps for freshness checks.
- Q: What is the maximum historical data lookback period? → A: 14 months (AWS Cost Explorer maximum).
- Q: How should the plugin respond when a valid query returns no data? → A: Return empty result set (not an error) with `FallbackHint.RECOMMENDED` in the gRPC response to signal pulumicost-core to try fallback plugins (e.g., aws-public). When data exists, use `FallbackHint.NONE`.

### Session 2025-12-10

- Q: What security requirements should the plugin implement? → A: Implement comprehensive security controls including input validation, credential isolation, and audit logging of all API access attempts (no encrypted storage needed as plugin is stateless).
- Q: What observability requirements should the plugin implement? → A: Use ZeroLog as defined by the pluginsdk for structured JSON logging with request tracing, performance metrics, and error monitoring.
- Q: What scalability requirements should the plugin support? → A: Support at least 100 concurrent gRPC requests with bounded memory usage and automatic request queuing during peak loads.
- Q: What lifecycle/state transitions should be defined for entities? → A: Requires research from AWS Cost Explorer API documentation to determine proper state transitions for CacheEntry and ReservationData entities.
- Q: Are there any specific compliance or regulatory constraints? → A: No specific compliance constraints required for standard cloud cost monitoring.

## Assumptions

- Users have an AWS account with Cost Explorer enabled (Cost Explorer must be explicitly enabled in AWS)
- Users will configure appropriate IAM permissions before using the plugin
- The plugin operates as a read-only integration with no ability to modify AWS resources or billing
- AWS Cost Explorer data may have up to 24-hour delay for recent costs (standard AWS behavior)
- The plugin will be distributed as a single binary compatible with the PulumiCost plugin system
- The pulumicost-spec SDK requires `FallbackHint` enum addition ([pulumicost-spec#124](https://github.com/rshade/pulumicost-spec/issues/124)) - FR-015 is blocked until this is merged
