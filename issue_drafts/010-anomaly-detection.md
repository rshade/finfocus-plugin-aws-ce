# Issue: Feature - Anomaly Detection (#10)

**Status:** Researching
**Type:** Feature
**Priority:** Medium

## User Story
As a cloud admin, I want to be notified of unexpected cost spikes (anomalies) detected by AWS, so that I can investigate root causes immediately.

## Technical Thesis
Implement functionality to retrieve pre-detected anomalies using the AWS Cost Explorer Anomaly Detection API.

### API Mapping
*   **RPC:** `GetAnomalies(GetAnomaliesRequest)` (Proposed)
*   **AWS API:** `costexplorer.GetAnomalies`
*   **Input:** `AnomalyDateInterval` (Start/End)
*   **Output:** Map `types.Anomaly` to the plugin's anomaly response format.

## Boundary Guardrails (Hard Constraints)
1.  **No Local ML:** We absolutely DO NOT implement anomaly detection algorithms (e.g., Z-score, IQR) locally. We only report what AWS has already flagged.
2.  **Thresholds:** We do not filter anomalies by "Severity" unless the User explicitly requests a threshold (e.g., "Show only High impact"). We default to showing what AWS returns.
3.  **Feedback:** We do not support submitting feedback ("False Positive") back to AWS in v1.

## Acceptance Criteria
- [ ] Successfully calls `GetAnomalies` with a valid date range.
- [ ] Maps `AnomalyScore`, `Impact`, and `RootCauses` to the gRPC response.
- [ ] Handles pagination (`NextPageToken`) for large sets of anomalies.
