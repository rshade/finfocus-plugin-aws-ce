<!--
Sync Impact Report - Constitution v1.0.0
========================================
Version Change: 0.0.0 → 1.0.0
Rationale: MAJOR - Initial constitution establishment with 4 core principles
           focused on code quality, testing standards, user experience
           consistency, and performance requirements.

Modified Principles: N/A (initial creation)

Added Sections:
  - I. Code Quality & Simplicity
  - II. Testing Standards
  - III. User Experience Consistency
  - IV. Performance Requirements
  - Security Requirements
  - Development Workflow
  - Governance

Removed Sections: None

Templates Requiring Updates:
  ⚠ .specify/templates/plan-template.md - Constitution Check section pending
  ✅ .specify/templates/spec-template.md - No changes required
  ✅ .specify/templates/tasks-template.md - No changes required

Follow-up TODOs:
  - None
-->

# PulumiCost Plugin AWS CE Constitution

## Core Principles

### I. Code Quality & Simplicity

**MUST enforce:**

- Keep It Simple, Stupid (KISS): No premature abstraction or over-engineering
- Single Responsibility Principle: Each package, type, and function does ONE
  thing well
- Explicit is better than implicit: No magic, hidden behavior, or surprising
  side effects
- All code MUST pass `make lint` before commit (golangci-lint with project
  config)
- Error handling MUST be explicit; never ignore returned errors
- All exported functions and types MUST have documentation comments
- All files MUST end with a newline character

**File size guidance:**

- Aim for focused, single-purpose files (typically <300 lines)
- Prefer logical separation over arbitrary line limits
- Large files are acceptable when they serve a single, cohesive purpose
  (e.g., comprehensive test suites, well-structured service implementations)

**Rationale:** This plugin is called as an external gRPC service by PulumiCost
core. Complexity compounds debugging difficulty when troubleshooting RPC
interactions. Simple, obvious code reduces maintenance burden and makes
contribution easier.

### II. Testing Standards

**MUST enforce:**

- All new functionality MUST have corresponding unit tests
- Unit tests for pure transformation functions and stateless logic (cost
  calculations, response builders)
- Integration tests for gRPC service methods using
  `pluginsdk.NewTestPlugin(t, plugin)` pattern
- Mock external dependencies (AWS Cost Explorer API) in unit tests
- No mocking of dependencies you don't own (e.g., proto messages, pluginsdk)
- Test quality indicators:
  - Each test has a distinct, clear purpose
  - Table-driven tests for variations on the same behavior
  - Simple setup, clear assertions
  - Fast execution (< 1s for unit suite, < 5s for integration suite)
- Tests MUST run via `make test` and pass before any commit
- Test coverage goal: Focus on critical paths (Cost Explorer API calls,
  response formatting) rather than arbitrary percentage targets

**What NOT to test:**

- Proto message serialization (trust the proto compiler)
- pluginsdk.Serve() lifecycle (trust the SDK)
- Over-engineered mocking infrastructure (no `unsafe.Pointer` conversions,
  no complex helper functions wrapping struct literals)

**Rationale:** Testing validates correctness of cost retrieval logic, which is
the core value proposition. Poor tests (redundant, over-complicated, or "AI
slop") waste time and create false confidence. Good tests enable safe
refactoring and catch regressions early.

### III. User Experience Consistency

**MUST enforce:**

- **gRPC CostSourceService protocol is sacred:**
  - NEVER log to stdout except PORT announcement
  - Use zerolog for structured JSON logging to stderr
  - Log entries MUST include `[pulumicost-plugin-aws-ce]` component identifier
  - Use `pluginsdk.Serve()` for lifecycle management
- **Error codes MUST use proto ErrorCode enum:**
  - `ERROR_CODE_INVALID_RESOURCE`: Missing required ResourceDescriptor fields
  - `ERROR_CODE_NO_DATA`: No cost data available for requested period
  - `ERROR_CODE_NOT_SUPPORTED`: Resource type not supported by this plugin
  - NO custom error codes outside the proto enum
- **Error messages MUST be actionable:**
  - Include context for troubleshooting (resource ID, date range, region)
  - Gracefully handle missing AWS credentials with clear error messages
- **API responses MUST use standard SDK types:**
  - Use `pluginsdk.NotSupportedError()`, `pluginsdk.NoDataError()` for
    standard errors
  - Use `pluginsdk.Calculator()` response builders

**gRPC Method Requirements:**

- `Name()` → returns `NameResponse{name: "aws-ce"}`
- `Supports(ResourceDescriptor)` → checks provider (aws only), returns
  `SupportsResponse{supported, reason}`
- `GetProjectedCost(ResourceDescriptor)` → returns error (actual costs only
  plugin)
- `GetActualCost(GetActualCostRequest)` → retrieves historical costs from
  AWS Cost Explorer API
- `GetServiceActualCost()`, `GetAccountActualCost()` → service-level and
  account-level queries

**Rationale:** PulumiCost core depends on predictable gRPC protocol behavior.
Breaking protocol compatibility breaks the integration. Using proto-defined
types ensures compatibility across all PulumiCost plugins. Consistent error
messages reduce user confusion and support burden.

### IV. Performance Requirements

**MUST enforce:**

- **AWS Cost Explorer API optimization:**
  - Client initialization MUST be lazy (on first API call)
  - Batch operations MUST be used when querying multiple resources
  - API calls MUST implement appropriate timeouts (default: 30 seconds)
- **Latency targets:**
  - Plugin startup time: < 500ms
  - PORT announcement: < 1 second after start
  - GetActualCost() RPC: < 10 seconds for typical date ranges (30 days)
  - Supports() RPC: < 10ms per call
- **Resource limits:**
  - Memory usage MUST remain bounded; avoid loading unbounded data sets
  - Implement pagination for large result sets from Cost Explorer
  - Support at least 100 concurrent RPC calls

**Performance monitoring:**

- Log warnings via zerolog if Cost Explorer API call takes > 5 seconds
- Use zerolog structured fields for RPC timing if observability is added

**Rationale:** The plugin may handle multiple concurrent RPC calls during cost
analysis. Slow startup or inefficient API calls create poor user experience.
Lazy initialization and batching ensure predictable performance without
unnecessary AWS API costs.

## Security Requirements

**MUST enforce:**

- No credentials or secrets in logs, error messages, or gRPC responses
- AWS credentials MUST use standard SDK credential chain (env vars, shared
  config, IAM roles)
- Cost Explorer API calls MUST use read-only permissions (ce:GetCostAndUsage,
  ce:GetCostForecast)
- Input validation: Reject malformed ResourceDescriptor gracefully (return
  gRPC InvalidArgument error)
- Dependency scanning: Use `govulncheck` in CI to detect known vulnerabilities
- **gRPC security:** Serve on loopback only (127.0.0.1), no TLS required for
  local communication

**Rationale:** The plugin processes user infrastructure definitions via gRPC
and queries AWS Cost Explorer. Leaking credentials or allowing arbitrary code
execution via malformed input is unacceptable. Standard credential chain
provides flexibility while maintaining security. Loopback-only serving
prevents unauthorized network access.

## Development Workflow

**MUST enforce:**

- Feature branches named `###-feature-name` (where ### is issue/feature number)
- Commits MUST follow conventional commit format (verified via commitlint):
  - `feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`
  - No "🤖 Generated with [Claude Code]" or "Co-Authored-By: Claude" in
    commit messages
- Pull requests MUST:
  - Reference related issue/feature number
  - Include updated tests if logic changes
  - Pass all CI checks (lint, test, build)
  - Update CLAUDE.md if new conventions or patterns emerge
- Markdown files MUST be linted with markdownlint after editing
- **gRPC changes:** Update proto definitions in finfocus-spec if protocol
  changes needed

**Code review requirements:**

- At least one approval before merge
- Verify constitution compliance (simplicity, testing, gRPC protocol adherence)
- Check for "AI slop": redundant tests, unused fields, over-complicated helpers
- **Protocol compatibility:** Verify no breaking changes to gRPC interface

**Rationale:** Consistent workflow reduces friction in collaboration and code
review. Conventional commits enable automated changelog generation.
Constitution compliance checks ensure long-term maintainability. gRPC protocol
compatibility is critical for integration with PulumiCost core.

## Governance

**Amendment procedure:**

1. Propose amendment via GitHub issue or PR with rationale
2. Document impact on existing code and templates
3. Update version per semantic versioning:
   - MAJOR: Backward incompatible principle removals or redefinitions
   - MINOR: New principle/section added or materially expanded guidance
   - PATCH: Clarifications, wording, typo fixes
4. Propagate changes to dependent templates (plan, spec, tasks)
5. Update this file with Sync Impact Report (HTML comment at top)

**Versioning policy:**

- Constitution version MUST increment with each substantive change
- Version MUST be documented in Sync Impact Report
- RATIFICATION_DATE is the original adoption date (does not change)
- LAST_AMENDED_DATE updates to today's date when amended

**Compliance review:**

- All PRs MUST verify compliance with constitution principles
- Use `.specify/templates/plan-template.md` Constitution Check section as gate
- Complexity violations MUST be justified in plan.md Complexity Tracking table
- Constitution supersedes ad-hoc practices; when in doubt, refer to this
  document

**Runtime development guidance:**

- Use CLAUDE.md for agent-specific guidance and project conventions
- Constitution defines non-negotiable rules; CLAUDE.md provides practical
  implementation details
- When CLAUDE.md conflicts with constitution, constitution wins

**Version**: 1.0.0 | **Ratified**: 2025-12-05 | **Last Amended**: 2025-12-05
