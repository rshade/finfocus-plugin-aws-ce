# Design

## Source isolation

`GetRecommendations` runs each selected source in a fixed order: rightsizing,
each Savings Plans plan type, each reservation service, Spot, then anomalies.
Anomalies run last because their mapped rows carry no `monthly_savings`
metadata, so projection must finish before they are appended.

- A source returns its mapped recommendations or an error. A nil error appends
  the recommendations and counts a success.
- A source error is hard when the context has ended or the status is
  `Canceled`, `DeadlineExceeded`, or `ResourceExhausted`. The RPC returns that
  error at once, so a budget that is already spent stops later calls.
- Any other source error is logged at warn level with the source name and
  dropped.
- After all sources run, if none succeeded and at least one failed, the RPC
  returns the first failure. That keeps existing status mapping for total
  outages, such as `AccessDenied` on every call or missing credentials.
- If no source was selected, the RPC returns an empty success, as before.

## Client initialization

The Cost Explorer client is built only when a Cost Explorer source is selected,
or when Spot needs its quote client. Errors that carry a gRPC status come from
request-scoped credentials and remain hard. A plain error from the default
client path becomes a soft `Internal` source failure.

## Rightsizing items

Each item maps on its own. `RightsizingType` is normalized with `strings.ToUpper`
before the switch. Any item error (missing identity, unknown type, missing
detail, invalid money, validation) skips that item with a warning, so one bad
item cannot erase the page. Savings Plans and reservation page mapping keep their
source-level errors.
