## ADDED Requirements

### Requirement: Retrieve non-EC2 reservation purchase recommendations

The plugin SHALL proxy GetReservationPurchaseRecommendation for Amazon RDS,
Amazon Redshift, Amazon ElastiCache, and Amazon OpenSearch Service using billing
service names, payer scope, a 30-day lookback, a one-year term, and no upfront
payment. The plugin SHALL NOT request EC2 reservation recommendations.

#### Scenario: Unfiltered optimization request

- **WHEN** GetRecommendations requests cost recommendations without a resource type
- **THEN** the plugin queries all four reservation services alongside rightsizing and Savings Plans

#### Scenario: Service selection

- **WHEN** a filter or target selects rds:Account, redshift:Account, elasticache:Account, or es:Account
- **THEN** only matching reservation sources are selected before credential initialization
- **AND** short service names and AWS service code aliases match the canonical type

### Requirement: Return SDK-compatible purchase opportunities

The plugin SHALL map reservation details into PURCHASE_COMMITMENT actions with
commitment type reserved_instance, AWS quantity and terms, account-level resource
identity, service-specific SKU, region, savings, and currency. The plugin SHALL
preserve original monthly savings, upfront cost, and service identity in metadata
and validate populated records with pluginsdk.ValidateRecommendation.

#### Scenario: AWS reservation detail

- **WHEN** AWS returns a valid reservation detail for a supported service
- **THEN** the response reports AWS estimated monthly savings without local savings calculations
- **AND** the record preserves the service code, quantity, purchase terms, and instance details

#### Scenario: Identity stability

- **WHEN** records are retrieved in a different order or with a new generation timestamp
- **THEN** their recommendation IDs remain stable
- **AND** different engine, license, or deployment details produce distinct IDs

#### Scenario: Empty dataset

- **WHEN** selected AWS datasets contain no recommendations
- **THEN** the SDK RPC succeeds with an empty list, an empty next-page token, and a non-nil zero summary

### Requirement: Preserve request isolation and complete-source semantics

Reservation requests SHALL share the RPC's client, credentials, and request
budget. The plugin SHALL use existing retry, cancellation, token repetition, and
100-page guards and SDK filtering, sorting, pagination, and summary helpers.

#### Scenario: Account and region filtering

- **WHEN** a request specifies an account and a region
- **THEN** reservation requests use AccountId rather than a positive dimension expression
- **AND** region filtering applies exactly to mapped AWS region values

#### Scenario: Source failure

- **WHEN** any selected reservation source fails or returns malformed data
- **THEN** the RPC fails with the mapped gRPC status and no partial recommendations

#### Scenario: Credentials and budget

- **WHEN** a request supplies credentials or reaches its AWS request budget
- **THEN** reservation calls use only that request's client and consume the same budget as other selected sources
