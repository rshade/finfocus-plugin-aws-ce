# Tasks: Add ARN to GetActualCostRequest

**Input**: Design documents from `/specs/002-add-arn-spec/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included per constitution requirement (II. Testing Standards)

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included in descriptions

## Path Conventions

This is a Go plugin project with structure:

- `internal/pricing/` - Core pricing/cost logic
- `internal/client/` - AWS Cost Explorer client
- `cmd/plugin/` - Plugin entry point

---

## Phase 1: Setup (Upstream Dependency)

**Purpose**: Upstream spec change must be completed and released before plugin work.
**NOTE**: These tasks are to be executed in the `rshade/finfocus-spec` repository. They cannot be directly executed within this project context.

- [x] T001 Create PR in rshade/finfocus-spec adding `string arn = 5` to GetActualCostRequest in proto/pulumicost/v1/costsource.proto
- [x] T002 Add documentation comment for arn field describing format and usage
- [x] T003 Run `make generate` in finfocus-spec to regenerate Go SDK code
- [x] T004 Merge PR and tag new spec release (e.g., v0.5.2 or v0.5.0)

---

## Phase 2: Foundational (Plugin Dependency Update)

**Purpose**: Update plugin to use new spec version - BLOCKS all user stories

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T005 Update go.mod to require new finfocus-spec version in go.mod
- [x] T006 Run `go mod tidy` to update dependencies
- [x] T007 Verify `req.GetArn()` method is available in pbc.GetActualCostRequest
- [x] T008 Run `make build` to confirm compilation succeeds
- [x] T009 Run `make test` to confirm existing tests still pass (backward compat baseline)

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Query Costs with ARN (Priority: P1) 🎯 MVP

**Goal**: Plugin can access ARN field and use it for cost lookups when provided

**Independent Test**: Provide ARN in request, verify plugin uses it for lookup. Requests without ARN continue working (backward compat).

### Tests for User Story 1

- [x] T010 [P] [US1] Add unit test for ARN field access in internal/pricing/calculator_test.go
- [x] T011 [P] [US1] Add backward compatibility test (empty ARN) in internal/pricing/calculator_test.go

### Implementation for User Story 1

- [x] T012 [US1] Update GetActualCost to read arn field via req.GetArn() in internal/pricing/calculator.go
- [x] T013 [US1] Add logging for ARN field when present in internal/pricing/calculator.go
- [x] T014 [US1] Implement resolveIdentifier helper to select ARN or resource_id in internal/pricing/calculator.go
- [x] T015 [US1] Update cost filter to use resolved identifier in internal/pricing/calculator.go

**Checkpoint**: ARN field accessible and used when provided. Backward compat verified.

---

## Phase 4: User Story 2 - Identity Verification (Priority: P2)

**Goal**: Plugin detects mismatch between resource_id and ARN, logs warning, uses ARN as source of truth

**Independent Test**: Provide mismatched resource_id and ARN, verify warning logged and ARN used.

### Tests for User Story 2

- [x] T016 [P] [US2] Add unit test for matching identifiers (no warning) in internal/pricing/calculator_test.go
- [x] T017 [P] [US2] Add unit test for mismatched identifiers (warning logged) in internal/pricing/calculator_test.go

### Implementation for User Story 2

- [x] T018 [US2] Implement identifier comparison logic in resolveIdentifier in internal/pricing/calculator.go
- [x] T019 [US2] Add warning log when resource_id does not match ARN resource portion in internal/pricing/calculator.go

**Checkpoint**: Mismatch detection working. ARN is source of truth when conflict detected.

---

## Phase 5: User Story 3 - Extract Context from ARN (Priority: P3)

**Goal**: Plugin parses ARN to extract region, account, service for enhanced Cost Explorer filtering

**Independent Test**: Provide ARN, verify plugin parses components correctly and uses them for filtering.

### Tests for User Story 3

- [x] T020 [P] [US3] Add unit tests for ParseARN function with valid ARNs in internal/pricing/arn_test.go
- [x] T021 [P] [US3] Add unit tests for ParseARN with invalid/empty ARNs in internal/pricing/arn_test.go
- [x] T022 [P] [US3] Add unit tests for various ARN formats (EC2, RDS, S3, Lambda) in internal/pricing/arn_test.go

### Implementation for User Story 3

- [x] T023 [US3] Create ParsedARN struct in internal/pricing/arn.go
- [x] T024 [US3] Implement ParseARN function using strings.SplitN in internal/pricing/arn.go
- [x] T025 [US3] Add error handling for invalid ARN format in internal/pricing/arn.go
- [x] T026 [US3] Integrate ParseARN into resolveIdentifier in internal/pricing/calculator.go
- [x] T027 [US3] Add account/region dimensions to Cost Explorer filter when parsed from ARN in internal/pricing/calculator.go

**Checkpoint**: ARN parsing complete. Filter enhanced with account/region context.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation, documentation, and quality assurance

- [x] T028 [P] Run `make lint` and fix any issues
- [x] T029 [P] Run `make test` and verify all tests pass
- [x] T030 [P] Run `make build` and verify binary compiles
- [x] T031 Update CLAUDE.md if new conventions emerged
- [x] T032 Validate implementation against quickstart.md checklist
- [x] T033 Create PR with conventional commit message format
- [x] T034 [P] [FR-009] Implement malformed ARN handling (log warning, fallback to resource_id) and add unit tests in internal/pricing/calculator_test.go

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: External - upstream spec repo work
- **Foundational (Phase 2)**: Depends on Setup (spec release) - BLOCKS all user stories
- **User Stories (Phase 3-5)**: All depend on Foundational phase completion
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - MVP, no dependencies on other stories
- **User Story 2 (P2)**: Can start after US1 - extends resolveIdentifier with comparison
- **User Story 3 (P3)**: Can start after US1 - extends with parsing, integrates back

### Within Each User Story

- Tests written FIRST (TDD approach per constitution)
- Core logic before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All tests marked [P] within a story can run in parallel
- All Phase 6 tasks marked [P] can run in parallel
- US2 and US3 can start in parallel after US1 (different concerns)

---

## Parallel Example: User Story 3 Tests

```bash
# Launch all US3 tests together:
Task: "Add unit tests for ParseARN function with valid ARNs in internal/pricing/arn_test.go"
Task: "Add unit tests for ParseARN with invalid/empty ARNs in internal/pricing/arn_test.go"
Task: "Add unit tests for various ARN formats (EC2, RDS, S3, Lambda) in internal/pricing/arn_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (upstream PR)
2. Wait for spec release
3. Complete Phase 2: Foundational (dependency update)
4. Complete Phase 3: User Story 1
5. **STOP and VALIDATE**: Test ARN access independently
6. Can ship MVP at this point

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add User Story 1 → Test → MVP complete!
3. Add User Story 2 → Test → Mismatch detection added
4. Add User Story 3 → Test → Full ARN parsing added
5. Polish → Ready for release

### Sequential Execution (Single Developer)

For this feature, sequential is recommended:

1. Phase 1-2: Upstream work and dependency update
2. Phase 3: US1 (basic ARN access)
3. Phase 4: US2 (mismatch detection)
4. Phase 5: US3 (ARN parsing)
5. Phase 6: Polish

---

## Notes

- Upstream spec PR must merge before plugin work begins
- Constitution requires tests for all new functionality
- Use `make lint`, `make test`, `make build` for validation
- Commit after each task or logical group
- Each user story delivers independent value
