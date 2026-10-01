# Feature Specification: Implement Core Cost Plugin (Spec 001) & E2E Testing

**Feature Branch**: `004-core-cost-e2e`
**Created**: 2026-01-21
**Status**: Draft
**Input**: User description: "Implement Core Cost Plugin (Spec 001) & E2E Testing"

## Clarifications

### Session 2026-01-21

- Q: How should cached cost data be invalidated? → A: Time-based + manual invalidation (24h expiration with option to force refresh via flag/parameter)
- Q: How should the CI environment authenticate with AWS? → A: GitHub OIDC federation with AWS IAM role (short-lived tokens, no secrets)
- Q: How should the plugin handle partial/incomplete cost data from AWS? → A: Return partial data with warning metadata and log the issue (best-effort delivery)
- Q: What should be the timeout and dataset scope strategy for E2E tests? → A: Limit queries to small date ranges (7 days max, 2min timeout, minimal cost)
- Q: How should the plugin handle network connectivity issues to AWS? → A: Retry with exponential backoff (3 attempts), then fail with detailed network diagnostics

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Retrieve Actual Costs from AWS (Priority: P1)

As a developer or user, I want the plugin to successfully retrieve actual cost data from the real AWS Cost Explorer API so that I can see my true cloud spending.

**Why this priority**: This is the fundamental functionality of the plugin (Spec 001 implementation).

**Independent Test**: Can be tested by running the plugin locally with AWS credentials and executing a `GetActualCost` query via the E2E test suite.

**Acceptance Scenarios**:

1. **Given** valid AWS credentials, **When** `GetActualCost` is called for a valid date range, **Then** the plugin returns non-empty cost data from AWS.
2. **Given** invalid AWS credentials, **When** `GetActualCost` is called, **Then** the plugin returns a clear authentication error.
3. **Given** a request for a specific service (e.g., "AmazonEC2"), **When** `GetActualCost` is called, **Then** the results are filtered to that service.
4. **Given** AWS returns partial/incomplete cost data, **When** `GetActualCost` is called, **Then** the plugin returns available data with warning metadata and logs the incompleteness.
5. **Given** a transient network failure occurs, **When** `GetActualCost` is called, **Then** the plugin retries up to 3 times with exponential backoff before failing with network diagnostics.

---

### User Story 2 - Automated E2E Testing in CI (Priority: P1)

As a maintainer, I want E2E tests to run automatically in CI so that I can prevent regressions in the AWS integration.

**Why this priority**: Ensures the integration with AWS remains functional as the codebase evolves.

**Independent Test**: Can be tested by pushing a commit and verifying the GitHub Actions workflow executes the E2E tests successfully.

**Acceptance Scenarios**:

1. **Given** a pull request or merge to main, **When** the CI pipeline runs, **Then** the E2E tests execute and pass using GitHub OIDC-federated AWS credentials.
2. **Given** a CI run where AWS credentials are unavailable (e.g., fork PRs), **When** the pipeline runs, **Then** the E2E tests are skipped gracefully without failing the build.
3. **Given** the CI workflow uses OIDC, **When** authentication occurs, **Then** no long-lived AWS access keys are stored in GitHub Secrets.
4. **Given** E2E tests are running, **When** queries are executed, **Then** date ranges do not exceed 7 days and tests complete within 2 minutes.

---

### User Story 3 - Cost Data Caching (Priority: P2)

As a user, I want the plugin to cache cost data locally so that repeated queries are faster and do not consume my AWS API rate limits.

**Why this priority**: Improves performance and reliability (as defined in Spec 001).

**Independent Test**: Can be tested by running the same query twice and verifying the second request is served from cache (faster response, no API call logged).

**Acceptance Scenarios**:

1. **Given** a cold cache, **When** a query is made, **Then** data is fetched from AWS and stored in cache.
2. **Given** a warm cache, **When** the same query is repeated, **Then** data is returned from the cache immediately.
3. **Given** the plugin restarts, **When** a previously cached query is made, **Then** data is loaded from the persistent disk cache.
4. **Given** cached data older than 24 hours, **When** a query is made, **Then** fresh data is fetched from AWS and cache is updated.
5. **Given** a force-refresh flag is set, **When** a query is made, **Then** cached data is bypassed and fresh data is fetched from AWS.

### Edge Cases

- **Partial AWS Data**: When AWS Cost Explorer returns incomplete data, return available data with warning metadata and log the issue (best-effort delivery).
- **E2E Test Timeouts**: Queries are limited to 7-day maximum date ranges with 2-minute timeout to prevent long-running tests and minimize AWS costs.
- **Network Connectivity Issues**: On network failures, retry with exponential backoff (3 attempts max), then fail with detailed diagnostics distinguishing connectivity vs. authentication errors.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST implement `GetActualCost` logic to fetch data from AWS Cost Explorer API using the AWS SDK.
- **FR-001a**: System MUST retry failed AWS API calls using exponential backoff (3 attempts max) and return detailed network diagnostics on final failure.
- **FR-002**: System MUST support grouping and filtering by dimensions (Service, Region, Linked Account, Tag) as defined in Spec 001.
- **FR-002a**: When AWS returns partial/incomplete data, system MUST return available data with warning metadata and log the incompleteness issue.
- **FR-003**: System MUST implement hybrid caching (in-memory + disk persistence) with 24-hour automatic expiration and support for manual cache invalidation via flag/parameter.
- **FR-004**: System MUST include a dedicated E2E test suite that executes real API calls against AWS.
- **FR-004a**: E2E tests MUST limit queries to maximum 7-day date ranges with 2-minute timeout to minimize costs and prevent long-running tests.
- **FR-005**: E2E tests MUST be controlled by environment variables or flags (e.g., `FINFOCUS_E2E=true`) to prevent accidental execution.
- **FR-006**: System MUST provide a standard command target (e.g., `make e2e`) for easy local execution.
- **FR-007**: CI/CD pipeline MUST be updated to include E2E test execution.
- **FR-008**: CI pipeline MUST use GitHub OIDC federation with AWS IAM role for authentication (no long-lived credentials).
- **FR-009**: E2E tests MUST skip execution if required AWS credentials are not present in the environment.

### Key Entities

- **CostEntry**: Data structure representing a single cost record (from Spec 001).
- **CacheManager**: Component responsible for storing and retrieving cost data.
- **E2ETestConfig**: Configuration for running E2E tests (credentials, region, test mode flags).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `GetActualCost` successfully retrieves data for the last 7 days from a real AWS account.
- **SC-002**: E2E tests pass in GitHub Actions for the main branch.
- **SC-003**: Caching implementation persists data across plugin restarts (verified by test).
- **SC-004**: `make e2e` command executes successfully in a local development environment with AWS credentials.
