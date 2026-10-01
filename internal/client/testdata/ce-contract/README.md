# Cost Explorer contract fixtures

Owner-owned, read-only for run agents. **Do not edit or regenerate `ce-contract.json`.** A
failing case is a finding: fix the plugin, or list it in the "Not delivered" register.

There is no AWS account to test against, so these fixtures replace one. They are Cost
Explorer wire-format responses (the official AWS API reference samples, two requests and
two responses, are real; the rest are synthetic) with the exact result a correct plugin must produce, computed
by an independent script with Decimal arithmetic.

## What they prove, and what they do not

- **Prove:** the plugin parses, pages, sums, converts dates, and fails correctly.
- **Do not prove:** that real Cost Explorer returns these shapes for a given account. Only the
  official samples are real. The opt-in live test (needs credentials, skipped by default, never
  in CI) is the only check of that.

## Case kinds

| Kind | How to test it |
| --- | --- |
| `response` | Serve `pages` from a fake HTTP Cost Explorer endpoint, one page per call, chained by `NextPageToken`. Call `GetActualCost` through a real gRPC server. Compare with `expect` |
| `request_period` | Convert `start_unix` and `end_unix` into the Cost Explorer `TimePeriod`. Run each case under `TZ=UTC`, `TZ=America/Los_Angeles` and `TZ=Pacific/Auckland`: the answer must not change |
| `resource_id` | Assert which operation, dimension and value the plugin sends for the input. `Unverified` cases need a documented choice or an explicit error, never a silent zero |
| `official_request` | The plugin's request for this shape must carry the same operation and keys |

## Rules

1. `ok` cases: the plugin's total equals `expect.total` exactly when compared as decimals (use a
   decimal type, not float64), and `per_service`, `currency`, `estimated` and `page_count` match.
2. `error` cases: the plugin returns the stated gRPC code or ErrorCode. It must never return a
   zero cost, a partial total presented as complete, or panic.
3. `estimated: true` means the plugin lowers its confidence and uses a short cache TTL.
4. A case the plugin cannot express is a finding named in the report, not a skip.
5. Write the results (case, expected, actual, verdict) to `.superpowers/contract-results.md` and
   paste the table in the run report.
