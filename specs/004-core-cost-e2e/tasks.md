# Implementation Tasks: Core Cost Plugin & E2E Testing

**Feature**: 004-core-cost-e2e
**Branch**: `004-core-cost-e2e`
**Date**: 2026-01-21
**Status**: Ready for Implementation

## Overview

This document provides a dependency-ordered task breakdown for implementing the Core Cost Plugin with caching, retry logic, and E2E testing infrastructure. Tasks are organized by user story to enable independent implementation and incremental delivery.

**Total Tasks**: 22
**MVP Scope**: Phase 3 (User Story 1) - 9 tasks
**Parallel Opportunities**: 14 parallelizable tasks

---

## Implementation Strategy

### MVP-First Approach

**MVP = User Story 1 only** (Tasks T001-T011)

- Delivers core AWS Cost Explorer integration with retry logic
- Provides immediately testable value
- Estimated completion: 2-3 days

**Post-MVP Increments**:

- **User Story 2** (Tasks T012-T017): CI/CD automation
- **User Story 3** (Tasks T018-T021): Performance optimization via caching

### Parallel Execution

Each user story has tasks that can execute in parallel within that story's phase. See "Parallelization Opportunities" section below.

---

## Phase 1: Setup & Dependencies

**Goal**: Initialize project dependencies and update documentation.

**Dependencies**: None (can start immediately)

### Tasks

- [ ] T001 Update go.mod dependencies for finfocus-spec v0.5.2+ and aws-sdk-go-v2
- [ ] T002 [P] Update CLAUDE.md with E2E testing conventions from specs/004-core-cost-e2e/quickstart.md
- [ ] T003 [P] Update Makefile with new e2e target per specs/004-core-cost-e2e/plan.md

**Completion Criteria**: Dependencies resolved, documentation updated, `make e2e` target available.

---

## Phase 2: Foundational Infrastructure

**Goal**: Implement shared retry logic used across all user stories.

**Dependencies**: Phase 1 complete

### Tasks

- [ ] T004 Create internal/retry/retry.go with RetryConfig struct and WithRetry generic function per specs/004-core-cost-e2e/data-model.md#3-retry-state-machine
- [ ] T005 [P] Implement error classification in internal/retry/retry.go (classifyRetryable, isThrottleError, isConnectionRetryable)
- [ ] T006 [P] Implement exponential backoff calculation in internal/retry/retry.go (calculateDelay with jitter)
- [ ] T007 [P] Create internal/retry/retry_test.go with unit tests for retry logic, error classification, and backoff calculation

**Completion Criteria**: Retry package compiles, all unit tests pass, exported functions available for use.

---

## Phase 3: User Story 1 - Retrieve Actual Costs from AWS (P1)

**Goal**: Implement core AWS Cost Explorer integration with resilient retry logic and partial data handling.

**User Story**: As a developer or user, I want the plugin to successfully retrieve actual cost data from the real AWS Cost Explorer API so that I can see my true cloud spending.

**Dependencies**: Phase 2 complete

**Independent Test**: Run plugin locally with AWS credentials and execute `GetActualCost` query to verify:

1. Valid credentials return cost data
2. Invalid credentials return clear authentication error
3. Service filtering works correctly
4. Partial data returns with warning logs
5. Network failures trigger 3 retries with exponential backoff

### Tasks

- [ ] T008 [P] [US1] Update internal/client/client.go to integrate retry.WithRetry for GetCostAndUsage calls
- [ ] T009 [P] [US1] Add structured logging for retry attempts in internal/client/client.go using zerolog with error_type, attempt, and delay_ms fields
- [ ] T010 [P] [US1] Update internal/pricing/calculator.go GetActualCost to handle partial data responses per specs/004-core-cost-e2e/data-model.md#4-warning-metadata-model
- [ ] T011 [P] [US1] Add warning logs for partial data scenarios in internal/pricing/calculator.go (no_data_found, identifier_mismatch) using pluginsdk.FieldOperation constants
- [ ] T012 [US1] Update internal/client/client_test.go with test cases for retry behavior (throttling, network errors, non-retryable errors)
- [ ] T013 [US1] Update internal/pricing/calculator_test.go with test cases for partial data handling and warning log verification
- [ ] T014 [US1] Run make test to verify all unit tests pass
- [ ] T015 [US1] Run make lint to verify code quality standards
- [ ] T016 [US1] Manual smoke test: Run plugin with AWS credentials and verify GetActualCost returns data for last 7 days

**Completion Criteria**:

- ✅ GetActualCost successfully retrieves data from real AWS account (SC-001)
- ✅ All unit tests pass with retry and partial data coverage
- ✅ Structured logging includes warning_type fields
- ✅ Network failures trigger 3 retries before failing
- ✅ Code passes golangci-lint

**Parallelization**: T008, T009, T010, T011 can run in parallel (different functions). T012, T013 depend on implementation tasks.

---

## Phase 4: User Story 2 - Automated E2E Testing in CI (P1)

**Goal**: Establish E2E testing infrastructure with GitHub OIDC federation for secure CI authentication.

**User Story**: As a maintainer, I want E2E tests to run automatically in CI so that I can prevent regressions in the AWS integration.

**Dependencies**: Phase 3 complete (requires working GetActualCost implementation)

**Independent Test**: Push a commit to main branch and verify:

1. E2E tests execute in GitHub Actions
2. Tests use OIDC federation (no secrets)
3. Fork PRs skip E2E tests gracefully
4. Tests complete within 2 minutes

### Tasks

- [ ] T017 [P] [US2] Create test/e2e/e2e_test.go with TestE2E function that skips if FINFOCUS_E2E != "true" per specs/004-core-cost-e2e/quickstart.md
- [ ] T018 [P] [US2] Implement E2E test cases in test/e2e/e2e_test.go for GetActualCost (7-day query, structure validation, timeout enforcement)
- [ ] T019 [P] [US2] Add credential availability check in test/e2e/e2e_test.go (skipIfNoAWSCreds helper) per specs/004-core-cost-e2e/research.md#2-e2e-test-framework-structure
- [ ] T020 [US2] Create .github/workflows/e2e.yml with GitHub OIDC configuration per specs/004-core-cost-e2e/contracts/aws-cost-explorer.md and specs/004-core-cost-e2e/research.md#5-github-oidc-federation-setup
- [ ] T021 [US2] Add fork PR skip logic to .github/workflows/e2e.yml using conditional steps (github.event.pull_request.head.repo.full_name check)
- [ ] T022 [US2] Run FINFOCUS_E2E=true make e2e locally to verify E2E tests execute and pass
- [ ] T023 [US2] Push to feature branch and verify E2E workflow executes in GitHub Actions (may skip if OIDC not yet configured)

**Completion Criteria**:

- ✅ E2E tests pass in GitHub Actions for main branch (SC-002)
- ✅ `make e2e` executes successfully locally with AWS credentials (SC-004)
- ✅ E2E tests complete within 2 minutes (FR-004a)
- ✅ Fork PRs skip E2E tests without failing build (FR-009)
- ✅ No long-lived AWS credentials in GitHub Secrets (FR-008)

**Parallelization**: T017, T018, T019, T020, T021 can run in parallel (different files/workflows). T022, T023 are sequential validation steps.

**Note**: OIDC IAM role setup is external (requires AWS admin). Document required setup in PR description.

---

## Phase 5: User Story 3 - Cost Data Caching (P2)

**Goal**: Implement hybrid caching with 24-hour TTL and manual invalidation to reduce AWS API calls and improve performance.

**User Story**: As a user, I want the plugin to cache cost data locally so that repeated queries are faster and do not consume my AWS API rate limits.

**Dependencies**: Phase 3 complete (requires working GetActualCost implementation)

**Independent Test**: Run same query twice and verify:

1. First request fetches from AWS (cache miss)
2. Second request returns from cache (< 100ms)
3. Cache persists across plugin restarts
4. 24-hour expiration works correctly
5. FINFOCUS_CACHE_BYPASS=true forces fresh fetch

### Tasks

- [ ] T024 [P] [US3] Create internal/pricing/cache.go with CacheManager, CacheEntry, and CacheKeyComponents structs per specs/004-core-cost-e2e/data-model.md#1-cache-entry-structure
- [ ] T025 [P] [US3] Implement cache key generation in internal/pricing/cache.go (CacheKeyComponents.String method with versioning)
- [ ] T026 [P] [US3] Implement cache directory resolution in internal/pricing/cache.go using os.UserCacheDir per specs/004-core-cost-e2e/research.md#1-cache-storage-format--location
- [ ] T027 [P] [US3] Implement cache Get/Set operations in internal/pricing/cache.go with dual-layer TTL validation (embedded timestamp + file mtime)
- [ ] T028 [P] [US3] Add FINFOCUS_CACHE_BYPASS environment variable support in internal/pricing/calculator.go GetActualCost method
- [ ] T029 [P] [US3] Create internal/pricing/cache_test.go with unit tests for cache operations (set, get, expiration, bypass, directory creation)
- [ ] T030 [US3] Integrate CacheManager into internal/pricing/calculator.go (initialize in NewCalculator, use in GetActualCost)
- [ ] T031 [US3] Add E2E cache test in test/e2e/e2e_test.go to verify cache hit/miss behavior and persistence
- [ ] T032 [US3] Run make test to verify all cache unit tests pass
- [ ] T033 [US3] Run FINFOCUS_E2E=true make e2e to verify cache E2E test passes
- [ ] T034 [US3] Manual verification: Run same query twice and confirm second request is < 100ms (cache hit)

**Completion Criteria**:

- ✅ Caching persists data across plugin restarts (SC-003)
- ✅ Cache hits return results in < 100ms
- ✅ 24-hour TTL expires stale data automatically
- ✅ FINFOCUS_CACHE_BYPASS=true bypasses cache
- ✅ All unit and E2E tests pass

**Parallelization**: T024, T025, T026, T027, T028, T029 can run in parallel (different functions/files). T030-T034 are sequential integration/validation steps.

---

## Phase 6: Polish & Documentation

**Goal**: Finalize documentation and ensure all success criteria are met.

**Dependencies**: All user stories complete

### Tasks

- [ ] T035 Update README.md with cache usage, E2E test execution, and OIDC setup instructions from specs/004-core-cost-e2e/quickstart.md
- [ ] T036 [P] Run make lint and fix any remaining issues
- [ ] T037 [P] Run make test and verify 100% pass rate
- [ ] T038 Run FINFOCUS_E2E=true make e2e and verify all E2E tests pass
- [ ] T039 Create PR with conventional commit message referencing feature 004 and all completed user stories

**Completion Criteria**:

- ✅ All success criteria met (SC-001 through SC-004)
- ✅ Documentation complete and accurate
- ✅ Code quality standards met
- ✅ PR ready for review

---

## Dependencies Graph

```
Phase 1 (Setup)
    ↓
Phase 2 (Foundational - Retry)
    ↓
    ├─→ Phase 3 (US1: Core AWS Integration) ─────┐
    │                                              │
    ├─→ Phase 4 (US2: E2E CI) ←──────────────────┘
    │
    └─→ Phase 5 (US3: Caching) ←─────────────────┘

All Phases ──→ Phase 6 (Polish)
```

**User Story Dependencies**:

- US1 (P1): No dependencies - can start after foundational phase
- US2 (P1): Depends on US1 (needs working GetActualCost for E2E tests)
- US3 (P2): Depends on US1 (needs working GetActualCost to cache results)

**Suggested Execution Order**:

1. MVP: Complete Phase 1, 2, 3 (US1)
2. Post-MVP Increment 1: Complete Phase 4 (US2) - CI automation
3. Post-MVP Increment 2: Complete Phase 5 (US3) - Performance optimization
4. Finalize: Complete Phase 6 - Polish and documentation

---

## Parallelization Opportunities

### Phase 1: Setup (2 parallel tracks)

- T001 (dependencies) + T002 (CLAUDE.md) + T003 (Makefile)

### Phase 2: Foundational (3 parallel tracks)

- T004 (structs) → T005 (error classification) + T006 (backoff) + T007 (tests)

### Phase 3: User Story 1 (4 parallel tracks)

- T008 (client retry) + T009 (client logging) + T010 (calculator partial data) + T011 (calculator logging)
- Then: T012 (client tests) + T013 (calculator tests)
- Finally: T014 → T015 → T016 (sequential validation)

### Phase 4: User Story 2 (5 parallel tracks)

- T017 (test skeleton) + T018 (test cases) + T019 (skip logic) + T020 (workflow) + T021 (fork PR skip)
- Then: T022 → T023 (sequential validation)

### Phase 5: User Story 3 (6 parallel tracks)

- T024 (structs) + T025 (key gen) + T026 (directory) + T027 (get/set) + T028 (bypass) + T029 (tests)
- Then: T030 → T031 → T032 → T033 → T034 (sequential integration)

### Phase 6: Polish (2 parallel tracks)

- T035 (README) → T036 (lint) + T037 (test)
- Then: T038 → T039 (sequential validation)

---

## Task Estimates

| Phase | Tasks | Estimated Duration | Parallelizable |
|-------|-------|-------------------|----------------|
| Phase 1: Setup | 3 | 30 min | 2/3 |
| Phase 2: Foundational | 4 | 2 hours | 3/4 |
| Phase 3: US1 (MVP) | 9 | 1 day | 4/9 |
| Phase 4: US2 | 7 | 4 hours | 5/7 |
| Phase 5: US3 | 11 | 1 day | 6/11 |
| Phase 6: Polish | 5 | 1 hour | 2/5 |
| **Total** | **39** | **~3 days** | **22/39** |

**Note**: Estimates assume familiarity with Go, AWS SDK, and the existing codebase. Actual time may vary.

---

## Testing Strategy

### Unit Tests (Phases 2, 3, 5)

- **Retry logic**: Test error classification, backoff calculation, max retries
- **Partial data handling**: Test warning logs, structured logging fields
- **Cache operations**: Test get/set, TTL expiration, bypass, directory creation

### E2E Tests (Phase 4)

- **AWS integration**: Real API calls with 7-day queries, < 2min timeout
- **Credential handling**: Skip logic when credentials unavailable
- **Cache behavior**: Verify cache hit/miss, persistence, expiration

### Manual Testing (Phases 3, 5)

- **Smoke test**: Run plugin with AWS credentials, verify cost data retrieval
- **Cache verification**: Run same query twice, confirm second is fast (< 100ms)

---

## Success Criteria Mapping

| Success Criteria | Verified By |
|------------------|-------------|
| SC-001: GetActualCost retrieves 7-day data | Phase 3 (T016) + Phase 4 (T022) |
| SC-002: E2E tests pass in GitHub Actions | Phase 4 (T023) |
| SC-003: Cache persists across restarts | Phase 5 (T031, T034) |
| SC-004: make e2e works locally | Phase 4 (T022) |

---

## Implementation Notes

### Retry Logic Best Practices

- Use type assertions for error classification (not string matching)
- Include error_type in structured logs for observability
- Cap backoff at 20 seconds per AWS SDK defaults

### Cache Implementation Best Practices

- Use JSON for debuggability (human-readable)
- Store cache in os.UserCacheDir for cross-platform compatibility
- Lazy eviction (no background cleanup needed)

### E2E Testing Best Practices

- Limit queries to 7 days max (minimize AWS costs)
- Use context.WithTimeout for 2-minute overall constraint
- Skip gracefully when credentials unavailable

### CI/CD Best Practices

- GitHub OIDC eliminates long-lived secrets
- Fork PRs skip E2E (no credentials by design)
- Workflow should document IAM role ARN requirement

---

## Risk Mitigation

| Risk | Mitigation | Task |
|------|------------|------|
| AWS credentials not available in CI | Document OIDC setup requirement, implement skip logic | T020, T021 |
| E2E tests exceed 2-minute timeout | Limit queries to 7 days, enforce timeout in test | T018 |
| Cache directory permissions | Handle errors gracefully, log warnings | T027 |
| Network failures block development | Implement comprehensive retry with clear diagnostics | T005, T009 |

---

## Next Steps

1. **Review this task breakdown** with team/stakeholders
2. **Start with MVP** (Phase 1-3, tasks T001-T016)
3. **Verify constitution compliance** at each phase completion
4. **Document learnings** in CLAUDE.md for future features
5. **Create PR** after Phase 6 completion

**Ready to begin implementation!**
