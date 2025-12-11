# Research Findings: AWS Cost Explorer Plugin for PulumiCost

**Date**: 2025-12-10
**Researcher**: opencode
**Context**: Implementation planning for AWS Cost Explorer integration with PulumiCost plugin SDK

## Decision: AWS Cost Explorer API Integration Approach

**Chosen**: Direct AWS SDK v2 integration with lazy client initialization and structured error handling

**Rationale**: AWS SDK v2 provides the most reliable and up-to-date Cost Explorer API access. Lazy initialization ensures fast plugin startup while maintaining connection efficiency. Structured error handling allows proper translation to PulumiCost error codes.

**Alternatives considered**:
- Third-party Cost Explorer libraries: Rejected due to maintenance overhead and potential compatibility issues
- REST API calls: Rejected due to complexity of authentication and response parsing compared to SDK benefits

## Decision: Caching Strategy Implementation

**Chosen**: Hybrid in-memory/disk caching with filesystem timestamp validation

**Rationale**: Balances performance (in-memory for active sessions) with persistence (disk for restarts). Filesystem timestamps provide reliable cache freshness checks without additional metadata storage.

**Alternatives considered**:
- Pure in-memory caching: Rejected due to data loss on plugin restart
- Database caching: Rejected due to unnecessary complexity for a plugin that should remain stateless
- No caching: Rejected due to AWS API rate limits and performance requirements

## Decision: Error Handling and Retry Logic

**Chosen**: Exponential backoff with configurable retry limits, AWS-specific error code translation

**Rationale**: AWS Cost Explorer API has rate limits that require intelligent retry logic. Exponential backoff prevents thundering herd problems. Error code translation ensures PulumiCost core receives actionable error information.

**Alternatives considered**:
- Simple retry with fixed delays: Rejected due to inefficient rate limit handling
- No retry logic: Rejected due to poor user experience with transient AWS API failures

## Decision: Authentication and Credential Management

**Chosen**: Standard AWS SDK credential chain (environment variables, shared credentials file, IAM roles)

**Rationale**: Follows AWS best practices and user expectations. Supports all standard authentication methods without custom credential handling logic.

**Alternatives considered**:
- Custom credential prompts: Rejected due to security risks and poor UX
- Limited credential sources: Rejected due to reduced flexibility for enterprise users

## Decision: Cost Data Granularity and Grouping

**Chosen**: Support both daily and monthly granularity with dimension-based grouping

**Rationale**: Matches AWS Cost Explorer capabilities while providing flexibility for different analysis needs. Dimension grouping enables cost allocation by service, account, tags, and availability zones.

**Alternatives considered**:
- Fixed granularity: Rejected due to reduced analytical flexibility
- Limited dimensions: Rejected due to inability to support enterprise cost allocation requirements

## Decision: Reservation and Savings Plan Data Integration

**Chosen**: Include reservation utilization and coverage metrics in cost responses

**Rationale**: Provides complete cost optimization insights beyond raw spending data. Helps users validate commitment utilization effectiveness.

**Alternatives considered**:
- Exclude reservation data: Rejected due to incomplete cost analysis capabilities
- Separate API endpoints: Rejected due to added complexity without clear benefits

## Decision: Plugin SDK Integration Pattern

**Chosen**: Embed pluginsdk.BasePlugin and implement Calculator interface methods

**Rationale**: Follows established PulumiCost plugin patterns. Ensures compatibility with core gRPC protocol expectations and standard error handling.

**Alternatives considered**:
- Custom gRPC implementation: Rejected due to protocol compatibility risks
- Minimal SDK usage: Rejected due to duplicated boilerplate code

## Decision: Logging and Observability

**Chosen**: zerolog structured JSON logging to stderr with component identification

**Rationale**: Meets PulumiCost protocol requirements for gRPC services. Structured logging enables better debugging and monitoring without stdout pollution.

**Alternatives considered**:
- Standard library log: Rejected due to lack of structured fields and protocol compliance
- External logging services: Rejected due to added complexity and dependencies

## Decision: Testing Strategy

**Chosen**: Unit tests for pure functions, integration tests with pluginsdk.NewTestPlugin, mocked AWS API calls

**Rationale**: Balances test coverage with execution speed. Mocks external dependencies while validating end-to-end gRPC flows.

**Alternatives considered**:
- Full AWS API integration tests: Rejected due to test flakiness and external dependencies
- No mocking: Rejected due to unreliable test execution and AWS API costs

## Key Integration Points Identified

1. **PulumiCost Core Protocol**: gRPC CostSourceService with specific method signatures and error codes
2. **AWS Cost Explorer API**: Rate-limited service requiring pagination and proper error handling
3. **Plugin Lifecycle**: Lazy initialization, graceful shutdown, and resource cleanup
4. **Caching Layer**: Filesystem-based persistence with timestamp validation
5. **Credential Chain**: Multiple AWS authentication methods with fallback logic

## Performance Considerations

- AWS API calls typically take 2-5 seconds for cost queries
- Plugin startup should remain under 500ms with lazy client initialization
- Memory usage must stay bounded to support concurrent requests
- Rate limiting requires intelligent retry logic to avoid user-facing failures

## Security Boundaries

- Plugin runs as separate process communicating via gRPC on loopback
- AWS credentials accessed only through standard SDK chain
- No credential storage or caching in plugin memory/disk
- Input validation prevents malformed requests from causing issues

## Migration and Compatibility Notes

- Plugin supports only actual costs (GetProjectedCost returns error)
- FallbackHint enum signals core when to try alternative plugins
- Error codes must match proto definitions for proper core handling
- gRPC protocol compatibility is critical for integration</content>
<parameter name="filePath">/mnt/c/GitHub/go/src/github.com/rshade/pulumicost-plugin-aws-ce/specs/001-aws-ce-plugin/research.md