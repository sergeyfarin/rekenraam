# ADR 0001: Initial Scope Decisions

## Status

Accepted

## Date

2026-05-29

## Context

The active repo already locked architecture, deployment shape, localization direction, and theme foundations. Several product-scope decisions still needed to be made before Phase 0 and Phase 1 work could proceed without ambiguity.

The unresolved areas were:

- required export formats for the first usable release
- attachment scope
- minimum mobile support level
- whether naming should be future-proofed for household sharing before shared workflows exist

## Decision

The project will adopt the following scope decisions:

1. The first mandatory user-facing export formats are CSV export of core ledger data and QIF export.
2. Attachments such as statement PDFs and receipts are out of scope for now.
3. The minimum mobile requirement is responsive support for full core workflows, including transaction entry.
4. Product language may remain strictly single-user for now; household-sharing terminology will not be pre-optimized before shared workflows become real scope.

## Consequences

### Positive

- Export planning has a concrete minimum portability baseline.
- Attachment storage does not complicate the early schema, storage, backup, or UI model.
- Mobile cannot be treated as an afterthought for core workflows.
- Naming and authorization design can stay simpler in early slices.

### Negative

- Future household support may require more deliberate renaming work later.
- Users who want receipt or statement attachment workflows will need to wait until the core product is stable.
- JSON full-app export is not guaranteed in the first export milestone.

### Follow-Up

- Define the exact contents of the first CSV and QIF export milestone before export implementation starts.
- Revisit attachment policy only after core ledger, reconciliation, import, and reporting workflows are stable.
- Add mobile-specific acceptance criteria to each core workflow as those screens are implemented.
- If household features become real scope, create a new ADR before renaming schema or API surfaces.