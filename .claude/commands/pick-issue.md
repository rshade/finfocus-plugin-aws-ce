# Pick an issue

Given a GitHub issue URL or number, read the issue and inspect the relevant code,
tests, and current [`openspec/specs/`](../../openspec/specs/) before editing.

Choose the smallest workflow that fits:

- **Behavior change:** run `/opsx:propose` to describe the change, then
  `/opsx:apply` to implement it, `openspec validate --all --strict` and the
  relevant tests to verify it, and `/opsx:archive` when complete. The archive
  updates current behavior in `openspec/specs/`.
- **Contained change:** edit directly when no behavior contract changes, such as
  a typo, documentation correction, or narrow build fix. Run the relevant
  verification command before finishing.

For each task, record a `Verify:` command with its expected result and a
`Break check:` describing the nearby behavior that could regress. Run both
checks and report their outcomes. Keep changes scoped to the issue; do not edit
the frozen history in `specs/` or `.specify/`.
