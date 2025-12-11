# Data Model: AWS Cost Explorer Plugin for PulumiCost

**Date**: 2025-12-10
**Context**: Entity definitions and relationships for AWS Cost Explorer cost data integration

## Core Entities

### CostEntry
Represents a single cost data point returned from AWS Cost Explorer API.

**Fields**:
- `timestamp` (time.Time): When the cost was incurred (start of period)
- `amount` (decimal.Decimal): Cost amount in USD
- `currency` (string): Currency code (always "USD" for AWS)
- `service` (string): AWS service name (e.g., "Amazon Elastic Compute Cloud - Compute")
- `account_id` (string): AWS account ID where cost was incurred
- `region` (string): AWS region (may be empty for global services)
- `availability_zone` (string): Specific AZ if applicable
- `tags` (map[string]string): Resource tags as key-value pairs
- `reservation_arn` (string): ARN of applied reservation if any
- `savings_plan_arn` (string): ARN of applied savings plan if any

**Validation Rules**:
- Amount must be non-negative
- Currency must be "USD"
- Service name cannot be empty
- Account ID must be valid AWS account format (12 digits)

**Relationships**:
- Belongs to a CostQuery (many-to-one)
- May reference ReservationData or SavingsPlanData

### ResourceDescriptor
Identifies the AWS resource or scope being queried (from PulumiCost SDK).

**Fields**:
- `provider` (string): Cloud provider ("aws" for this plugin)
- `type` (string): Resource type (e.g., "aws:ec2/instance", "aws:s3/bucket")
- `id` (string): Resource identifier within the provider
- `properties` (map[string]interface{}): Additional resource properties

**Validation Rules**:
- Provider must be "aws"
- Type and ID cannot be empty
- Properties map may be empty

**Relationships**:
- Used as input to cost queries
- Determines which AWS resources to analyze

### DateRange
Defines the time period for cost queries.

**Fields**:
- `start` (time.Time): Start of the query period (inclusive)
- `end` (time.Time): End of the query period (exclusive)

**Validation Rules**:
- Start must be before end
- Date range cannot exceed 14 months (AWS Cost Explorer limit)
- End cannot be in the future
- Start cannot be before account creation date

**Relationships**:
- Used by CostQuery to filter results
- Affects cache key generation

### Dimension
A grouping category for organizing cost data.

**Fields**:
- `type` (DimensionType): Type of dimension (SERVICE, LINKED_ACCOUNT, TAG, AZ)
- `key` (string): Dimension key (tag key for TAG type, empty for others)
- `value` (string): Dimension value

**Validation Rules**:
- Type must be valid DimensionType enum value
- Key required only for TAG dimension type
- Value cannot be empty

**Relationships**:
- Used by CostQuery for grouping results
- Determines how costs are aggregated

### ReservationData
Information about Reserved Instance or Savings Plan utilization and coverage.

**Fields**:
- `reservation_arn` (string): Unique identifier for the reservation
- `instance_type` (string): EC2 instance type (for RI) or service (for SP)
- `region` (string): AWS region where reservation applies
- `utilization_percentage` (float64): Percentage of reservation hours utilized (0-100)
- `coverage_percentage` (float64): Percentage of applicable spend covered (0-100)
- `total_cost` (decimal.Decimal): Total reservation cost
- `unused_cost` (decimal.Decimal): Cost of unused reservation hours
- `start_date` (time.Time): When reservation became active
- `end_date` (time.Time): When reservation expires

**Validation Rules**:
- Utilization and coverage percentages must be 0-100
- ARN must be valid AWS ARN format
- Dates must form valid interval

**Relationships**:
- Referenced by CostEntry for applied reservations
- Queried separately for reservation analysis

### CacheEntry
Represents a cached cost query result with metadata.

**Fields**:
- `query_key` (string): Unique key identifying the query parameters
- `results` ([]CostEntry): Cached cost data
- `created_at` (time.Time): When cache entry was created
- `expires_at` (time.Time): When cache entry should be refreshed
- `file_path` (string): Filesystem path for persistent storage

**Validation Rules**:
- Query key cannot be empty
- Results slice may be empty (for no-data responses)
- Created/expiry dates must form valid interval

**Relationships**:
- Managed by CacheManager
- Used by Calculator for performance optimization

### FallbackHint
Enum signaling whether PulumiCost core should try fallback plugins.

**Values**:
- `NONE`: Data returned successfully, no fallback needed
- `RECOMMENDED`: No data available, fallback plugins may provide estimates
- `REQUIRED`: Plugin unable to service request, fallback mandatory

**Validation Rules**:
- Must match proto enum definition
- Used in all gRPC responses

## Entity Relationships

```
ResourceDescriptor ──┬─── DateRange ───┬─── CostQuery ───┬─── CostEntry
                     │                 │                 │
                     │                 │                 └─── ReservationData
                     │                 │
                     │                 └─── Dimension
                     │
                     └─── CacheEntry (for persistence)
```

## State Transitions

### CostQuery States
1. **Created**: Query parameters validated
2. **Executing**: AWS API call in progress
3. **Completed**: Results retrieved and cached
4. **Failed**: Error occurred during execution
5. **Expired**: Cache entry no longer valid

### ReservationData States
1. **Active**: Reservation is currently active
2. **Expired**: Reservation term has ended
3. **Scheduled**: Reservation is purchased but not yet active

## Data Flow

1. **Input**: ResourceDescriptor + DateRange + optional Dimensions
2. **Validation**: Check date range limits, resource format, credentials
3. **Cache Check**: Look for existing cached results
4. **API Call**: Query AWS Cost Explorer if cache miss
5. **Processing**: Transform AWS response to CostEntry format
6. **Caching**: Store results for future queries
7. **Response**: Return CostEntry array with FallbackHint

## Storage Considerations

- **In-Memory Cache**: LRU cache for active plugin session
- **Disk Cache**: Filesystem-based persistence with timestamp validation
- **No Database**: Plugin remains stateless, all data from AWS API
- **Memory Bounds**: Implement pagination for large result sets

## Error Handling

- **Invalid Date Range**: Return validation error with helpful message
- **No AWS Credentials**: Clear authentication error
- **Rate Limited**: Implement exponential backoff retry
- **No Data Available**: Return empty results with RECOMMENDED fallback
- **API Errors**: Translate to appropriate PulumiCost error codes</content>
<parameter name="filePath">/mnt/c/GitHub/go/src/github.com/rshade/pulumicost-plugin-aws-ce/specs/001-aws-ce-plugin/data-model.md