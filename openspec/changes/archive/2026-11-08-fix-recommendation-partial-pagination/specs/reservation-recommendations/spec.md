## ADDED Requirements

### Requirement: Do not paginate partial results

When any selected source fails, the RPC SHALL return the remaining recommendations only if the request has no page token and they fit on one page. Otherwise it SHALL fail with the mapped gRPC status of the first failure, because page tokens are offsets into the full result and the response cannot name a failed source.

#### Scenario: Partial result on a paginated call

- **WHEN** a selected source fails and the request has a page token or the remaining recommendations need more than one page
- **THEN** the RPC fails with the mapped gRPC status of the first failure and no recommendations
- **AND** the same request with no failed source paginates as before
