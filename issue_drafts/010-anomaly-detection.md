# Anomaly detection, issue 10

**Status:** researching
**Type:** feature
**Priority:** medium

## User story

A cloud administrator needs notifications of unexpected cost spikes that AWS detects to investigate their causes promptly.

## Technical approach

Retrieve anomalies that AWS Cost Explorer has already detected through its anomaly detection API.

### API mapping

- **Proposed remote procedure call:** `GetAnomalies(GetAnomaliesRequest)`.
- **AWS API:** `costexplorer.GetAnomalies`.
- **Input:** `AnomalyDateInterval` for the start and end dates.
- **Output:** map `types.Anomaly` to the plugin's anomaly response format.

## Constraints

1. Report only anomalies that AWS flags. Don't implement local detection algorithms, such as Z-scores or interquartile ranges.
2. Show all anomalies that AWS returns unless the user requests a severity threshold, such as high-impact anomalies only.
3. Don't submit false-positive feedback to AWS in version 1.

## Acceptance criteria

- [ ] Call `GetAnomalies` with a valid date range.
- [ ] Map `AnomalyScore`, `Impact`, and `RootCauses` to the gRPC response.
- [ ] Handle `NextPageToken` pagination for large anomaly sets.
