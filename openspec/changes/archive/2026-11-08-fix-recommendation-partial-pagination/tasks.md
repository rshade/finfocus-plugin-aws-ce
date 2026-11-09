# Tasks

- [x] Add failing tests: a partial result longer than one page, and a partial
      result on a request with a page token, both return the first failure; a
      one-page partial result and pagination without a failure are unchanged.
- [x] Return the first failure from `recommendationSourceRun.paginationResult`
      when a source failed and the call is paginated.
- [x] Update `docs/recommendations.md` (Errors table, Pagination).
- [x] `make test`, `make lint`, `make lint-md`, `openspec validate --all --strict`.
