# gRPC API Contracts: CostSourceService

**Plugin**: aws-ce
**Protocol**: gRPC
**Base**: pulumicost-spec CostSourceService

## Service Definition

```protobuf
service CostSourceService {
  rpc Name(NameRequest) returns (NameResponse);
  rpc Supports(SupportsRequest) returns (SupportsResponse);
  rpc GetProjectedCost(GetProjectedCostRequest) returns (GetProjectedCostResponse);
  rpc GetActualCost(GetActualCostRequest) returns (GetActualCostResponse);
  rpc GetServiceActualCost(GetServiceActualCostRequest) returns (GetServiceActualCostResponse);
  rpc GetAccountActualCost(GetAccountActualCostRequest) returns (GetAccountActualCostResponse);
}
```

## Method Contracts

### Name
Returns the plugin identifier.

**Request**: `NameRequest{}`
**Response**: `NameResponse{name: "aws-ce"}`

**Contract**:
- Always returns "aws-ce"
- No errors possible
- Response time: < 1ms

### Supports
Checks if the plugin supports a given resource.

**Request**:
```protobuf
message SupportsRequest {
  ResourceDescriptor resource = 1;
}
```

**Response**:
```protobuf
message SupportsResponse {
  bool supported = 1;
  string reason = 2;
}
```

**Contract**:
- Returns `supported: true` only if `resource.provider == "aws"`
- Returns `supported: false, reason: "Only AWS provider supported"` otherwise
- No external API calls
- Response time: < 10ms

### GetProjectedCost
Returns an error (actual costs only plugin).

**Request**:
```protobuf
message GetProjectedCostRequest {
  ResourceDescriptor resource = 1;
  // ... other fields
}
```

**Response**:
```protobuf
message GetProjectedCostResponse {
  ErrorCode error_code = 1;
  string error_message = 2;
  FallbackHint fallback_hint = 3;
  // ... other fields
}
```

**Contract**:
- Always returns `error_code: ERROR_CODE_NOT_SUPPORTED`
- Always returns `error_message: "Plugin only supports actual costs from AWS Cost Explorer"`
- Always returns `fallback_hint: RECOMMENDED`
- No AWS API calls
- Response time: < 1ms

### GetActualCost
Retrieves historical cost data for a specific resource.

**Request**:
```protobuf
message GetActualCostRequest {
  ResourceDescriptor resource = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
  repeated string dimensions = 4;
  Granularity granularity = 5;
}
```

**Response**:
```protobuf
message GetActualCostResponse {
  repeated CostEntry entries = 1;
  ErrorCode error_code = 2;
  string error_message = 3;
  FallbackHint fallback_hint = 4;
}
```

**Contract**:
- Validates date range (max 14 months, not in future)
- Queries AWS Cost Explorer API with provided dimensions
- Returns cost entries grouped by requested dimensions
- Implements retry logic with exponential backoff for rate limits
- Returns `fallback_hint: NONE` on success, `RECOMMENDED` on no data
- Response time: < 10 seconds typical, < 30 seconds maximum
- Memory usage: Bounded with pagination for large result sets

### GetServiceActualCost
Retrieves cost data aggregated by AWS service.

**Request**:
```protobuf
message GetServiceActualCostRequest {
  string service_code = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
  Granularity granularity = 4;
}
```

**Response**:
```protobuf
message GetServiceActualCostResponse {
  repeated ServiceCostEntry entries = 1;
  ErrorCode error_code = 2;
  string error_message = 3;
  FallbackHint fallback_hint = 4;
}
```

**Contract**:
- Validates service code and date range
- Queries AWS Cost Explorer grouped by SERVICE dimension
- Filters results to specified service
- Returns `fallback_hint: NONE` on success, `RECOMMENDED` on no data
- Response time: < 10 seconds typical

### GetAccountActualCost
Retrieves cost data aggregated by AWS account.

**Request**:
```protobuf
message GetAccountActualCostRequest {
  string account_id = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
  Granularity granularity = 4;
}
```

**Response**:
```protobuf
message GetAccountActualCostResponse {
  repeated AccountCostEntry entries = 1;
  ErrorCode error_code = 2;
  string error_message = 3;
  FallbackHint fallback_hint = 4;
}
```

**Contract**:
- Validates account ID format and date range
- Queries AWS Cost Explorer grouped by LINKED_ACCOUNT dimension
- Filters results to specified account
- Returns `fallback_hint: NONE` on success, `RECOMMENDED` on no data
- Response time: < 10 seconds typical

## Common Data Types

### ResourceDescriptor
```protobuf
message ResourceDescriptor {
  string provider = 1;
  string type = 2;
  string id = 3;
  map<string, google.protobuf.Value> properties = 4;
}
```

### CostEntry
```protobuf
message CostEntry {
  google.protobuf.Timestamp timestamp = 1;
  string amount = 2;
  string currency = 3;
  string service = 4;
  string account_id = 5;
  string region = 6;
  string availability_zone = 7;
  map<string, string> tags = 8;
  string reservation_arn = 9;
  string savings_plan_arn = 10;
}
```

### ErrorCode
```protobuf
enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_INVALID_RESOURCE = 1;
  ERROR_CODE_NO_DATA = 2;
  ERROR_CODE_NOT_SUPPORTED = 3;
  // ... other codes
}
```

### FallbackHint
```protobuf
enum FallbackHint {
  FALLBACK_HINT_UNSPECIFIED = 0;
  FALLBACK_HINT_NONE = 1;
  FALLBACK_HINT_RECOMMENDED = 2;
  FALLBACK_HINT_REQUIRED = 3;
}
```

### Granularity
```protobuf
enum Granularity {
  GRANULARITY_UNSPECIFIED = 0;
  GRANULARITY_DAILY = 1;
  GRANULARITY_MONTHLY = 2;
}
```

## Error Handling Contracts

### Authentication Errors
- **Trigger**: Missing or invalid AWS credentials
- **Response**: `error_code: ERROR_CODE_NO_DATA, error_message: "AWS authentication failed: <details>"`
- **Fallback**: `RECOMMENDED`

### Rate Limit Errors
- **Trigger**: AWS Cost Explorer API rate limit exceeded
- **Behavior**: Automatic retry with exponential backoff (max 30 seconds)
- **Response**: Success after retry, or timeout error if retries exhausted

### Invalid Date Range
- **Trigger**: Date range exceeds 14 months or end date in future
- **Response**: `error_code: ERROR_CODE_INVALID_RESOURCE, error_message: "Date range exceeds AWS Cost Explorer limits"`
- **Fallback**: `RECOMMENDED`

### No Data Available
- **Trigger**: Valid query returns no cost data
- **Response**: Empty `entries` array, `error_code: ERROR_CODE_NO_DATA`
- **Fallback**: `RECOMMENDED`

### Service Unavailable
- **Trigger**: AWS Cost Explorer API returns 5xx errors
- **Behavior**: Retry with exponential backoff
- **Response**: Success after retry, or error if retries exhausted

## Performance Contracts

- **Startup Time**: < 500ms to PORT announcement
- **Supports()**: < 10ms per call
- **GetActualCost()**: < 10 seconds for typical queries (30 days, 1 dimension)
- **Concurrent Requests**: Support at least 100 simultaneous RPC calls
- **Memory Usage**: Bounded growth, no unbounded data structures
- **Cache Hit Ratio**: > 80% for repeated queries within session

## Security Contracts

- **Transport**: gRPC over loopback interface only (127.0.0.1)
- **Authentication**: AWS credentials via standard SDK chain only
- **Authorization**: Read-only ce:GetCostAndUsage permission required
- **Input Validation**: Reject malformed ResourceDescriptor gracefully
- **Logging**: No credentials or secrets in logs or responses</content>
<parameter name="filePath">/mnt/c/GitHub/go/src/github.com/rshade/pulumicost-plugin-aws-ce/specs/001-aws-ce-plugin/contracts/grpc-service-contracts.md