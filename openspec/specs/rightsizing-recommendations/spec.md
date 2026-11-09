# rightsizing-recommendations Specification

## Purpose

Map Cost Explorer rightsizing items from `GetRecommendations` into rightsize and terminate recommendations. Cost Explorer returns `RightsizingType` in title case, and an item that cannot be mapped is skipped rather than failing the page.

## Requirements

### Requirement: Map Cost Explorer rightsizing items case-insensitively

The plugin SHALL map a rightsizing item whose `RightsizingType` is `Modify` or
`Terminate` in any letter case, because Cost Explorer returns title case. The
mapped recommendation SHALL be a rightsizing recommendation for `Modify` and a
terminate recommendation for `Terminate`.

#### Scenario: Title-case type from Cost Explorer

- **WHEN** an item has `RightsizingType` `Modify` or `Terminate`
- **THEN** the item maps to a rightsize or terminate recommendation with the same identity as the upper-case form

#### Scenario: Upper-case type

- **WHEN** an item has `RightsizingType` `MODIFY` or `TERMINATE`
- **THEN** the item maps as it does for title case

### Requirement: Skip rightsizing items that cannot be mapped

An item that cannot be mapped (unknown type, missing identity, missing detail,
or an invalid amount) SHALL be skipped with a warning log. The other items on the
same page SHALL still be returned, and the RPC SHALL NOT fail for that item alone.

#### Scenario: Unknown type on a page

- **WHEN** a page contains an item with an unknown `RightsizingType` and other valid items
- **THEN** the unknown item is skipped with a warning log
- **AND** the valid items are returned

#### Scenario: Only item is unmappable

- **WHEN** every item on the page cannot be mapped
- **THEN** the rightsizing source returns no recommendations and counts as a successful source
