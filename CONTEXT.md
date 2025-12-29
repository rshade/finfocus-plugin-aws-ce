# CONTEXT.md

## Core Architectural Identity
**Lightweight gRPC Plugin / Adapter**
This project is a stateless "Provider Plugin" for the PulumiCost engine. It acts as a translation layer between the `pulumicost-core` (gRPC client) and the AWS Cost Explorer API (External Service).

It is **NOT** a standalone application, CLI tool, or dashboard. It is a worker node in a plugin architecture.

## Technical Boundaries & Hard No's
To prevent scope creep and architectural drift, this project adheres to the following boundaries:

1.  **No "Fin" Logic (Math):** We do not invent forecasting algorithms, amortizations, or cost models. If AWS provides the number (e.g., via `GetCostForecast`), we use it. We only perform basic arithmetic (e.g., currency conversion, summing) if absolutely necessary to fit the FOCUS 1.2 schema.
2.  **No "Ops" Logic (Resource Management):** This plugin is **Read-Only**. It will NEVER create, modify, or delete AWS resources (EC2, S3, etc.). It only reads billing data.
3.  **No Durable State:** This plugin does not own a database. It utilizes a transient file-based cache for performance (to reduce API costs/latency) but assumes it can be restarted at any time with zero data loss.
4.  **No UI/UX:** This project does not render HTML, charts, or CLI tables. It returns raw structured data (Protobuf/Go structs) to the Core engine.

## Data Source of Truth
*   **Actual Costs:** AWS Cost Explorer API (`GetCostAndUsage`).
*   **Forecasts:** AWS Cost Explorer API (`GetCostForecast`).
*   **Budgets:** AWS Budgets API (Planned).
*   **Metadata:** AWS Tags & Organizations APIs.

**Responsibility:** AWS is responsible for the accuracy of the billing data. We are responsible for the accuracy of the **translation** to the FOCUS 1.2 standard.

## Interaction Model
*   **Inbound:** gRPC Server (listening on localhost, port assigned by Core).
*   **Outbound:** AWS SDK for Go v2 (authenticated via standard AWS chains).
*   **Data Format:** Returns data complying strictly with the `pulumicost-spec` (FOCUS 1.2 derived) Protocol Buffers.
