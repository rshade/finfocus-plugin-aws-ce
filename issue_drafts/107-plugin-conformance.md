# Issue: Chore - Plugin Conformance Testing (#107)

**Status:** Planned
**Type:** Technical Debt / Quality
**Priority:** High

## User Story
As a plugin maintainer, I want to verify that my plugin strictly adheres to the `pulumicost-spec` contract, so that it guarantees interoperability with the Core engine.

## Technical Thesis
Integrate the official `Plugin Conformance Test Suite` provided by `github.com/rshade/pulumicost-spec/sdk/go/conformance`.

### Implementation Plan
1.  Create `cmd/conformance/main.go` or a test file `conformance_test.go`.
2.  Import the conformance suite.
3.  Configure the suite to run against the running `aws-ce` plugin binary.
4.  Add a `make conformance` target.

## Boundary Guardrails (Hard Constraints)
1.  **No Spec Modifications:** If a test fails, we must fix the *Plugin code*, not change the *Spec tests*.
2.  **Standard Environment:** Tests must pass using the standard Mock AWS Client (no live AWS calls required for basic conformance).

## Acceptance Criteria
- [ ] `make conformance` runs the official test suite.
- [ ] All mandatory compliance tests pass.
- [ ] CI pipeline runs conformance tests on every PR.
