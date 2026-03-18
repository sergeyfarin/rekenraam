# Import Functionality Plan (QIF/OFX-QFX/CSV/XLS/XLSX + Templates)

Date: 2026-02-11

Assumptions (can be adjusted):
- QFX is treated as OFX with the same parser path, plus optional file-extension hints.
- Templates are scoped to book + optional account/institution (so they can be reused per broker/bank).
- Initial scope covers bank and credit-card transactions; investment transaction import is a Phase 2+ extension.

## Goals
- Import QIF, OFX/QFX, CSV, XLS, XLSX reliably.
- Provide a user-defined template system for column mapping, normalization, and matching rules.
- Offer a safe, reviewable import flow with preview, duplicate matching, and audit trail.

## Phase 0: Fix Current Gaps (Schema and Validation)
1) Fix import_rules constraint mismatch.
   - Update the import_rules constraint to allow rule_kind values: payee, memo, amount, date, account.
   - Add migration and test that create_import_rule succeeds for all kinds.

2) Align import_sessions with actual flow.
   - Ensure that import sessions are created and committed during any import run.
   - Store session_id on imported transactions (new column or existing metadata structure).

3) TODO (from logic review): improve import session action tracking.
   - Decide whether duplicate session entries should update action instead of INSERT OR IGNORE.
   - Add validation tests for action transitions (validated -> updated).

4) TODO (from logic review): define multi-currency import policy.
   - Decide whether to reject mismatched currency rows, convert using FX, or route to a different account.
   - Add UI feedback for rows skipped due to currency mismatch.

## Phase 1: Unified Import Pipeline
1) Introduce a canonical import model.
   - ImportRow: raw fields + normalized fields (date, amount_minor, payee, memo, reference, import_id, currency_code, account_id, category_id, payee_id).
   - ImportContext: source format, template id, locale settings, file metadata.

2) Format registry and detection.
   - Add a registry for parsers: qif, ofx, qfx, csv, xls, xlsx, auto.
   - Auto detection by extension and content sniffing; QFX uses OFX parser.

3) Parsing adapters.
   - QIF: current parser extended to handle splits and categories.
   - OFX/QFX: replace line-based parsing with a robust OFX/SGML parser or a resilient tag reader.
   - CSV: keep strict header parsing, but allow template-driven mapping.
   - XLS/XLSX: read worksheet rows using a Rust Excel library (e.g., calamine) and send to CSV mapping logic.

4) Validation and normalization.
   - Normalize dates (explicit format support), amounts (decimal and locale), currency codes.
   - Validate mandatory fields and report row-level errors.

## Phase 2: Template System
1) Data model.
   - import_templates table: id, book_id, account_id (nullable), name, format, mapping_json, created_at, updated_at.
   - mapping_json includes: column->field map, date format, decimal separator, thousands separator, debit/credit sign rules, default currency, payee cleanup rules, regex extraction.

2) Template operations.
   - CRUD commands in Tauri: list/create/update/delete templates.
   - Export/import template JSON for sharing.

3) Template application.
   - The import pipeline applies mapping_json to produce ImportRow items.
   - Save "last used" template per account.

## Phase 3: Import Wizard UI
1) Flow steps.
   - Step 1: pick file + choose format (auto default).
   - Step 2: select or create template (CSV/XLS/XLSX), or use default for QIF/OFX/QFX.
   - Step 3: mapping preview for CSV/XLS/XLSX with row sampling.
   - Step 4: preview + validation errors + edit rows.
   - Step 5: apply import rules + duplicate matching.
   - Step 6: commit import session and show results.

2) UX details.
   - Highlight missing fields and parsing errors per row.
   - Provide bulk fix tools (set account, category, payee).
   - Show duplicate suggestions with accept/skip per row.

## Phase 4: Matching Improvements
1) Matching policy per template.
   - FITID/import_id exact match (highest priority).
   - Date window (e.g., +/- 3 days) + amount tolerance (e.g., 0.01 or currency scale).
   - Optional fuzzy payee matching.

2) Conflict resolution.
   - UI to confirm duplicates and allow manual override.
   - Persist user decisions for future runs.

## Phase 5: Test Coverage
1) Parser tests.
   - QIF/OFX/QFX happy paths + malformed input.
   - CSV/XLS/XLSX parsing with locale formats.

2) Template tests.
   - Column mapping, date parsing, amount normalization, sign rules, regex extraction.

3) End-to-end tests.
   - Import run: parse -> preview -> commit -> duplicates.

## Phase 6: Investment Transactions (Optional Extension)
- Extend OFX/QFX to parse investment transactions (trades, dividends, splits).
- Extend CSV/XLS/XLSX templates to map investment-specific fields.
- Store as proper accounting transactions with splits and commodity lots.

## Deliverables Checklist
- Schema migrations for templates and import sessions.
- Parsing registry with auto detection and XLS/XLSX support.
- Template CRUD and mapping logic.
- Import wizard UI with preview, validation, and matching.
- Tests for parsing and matching.

## Open Decisions
- Investment transactions: in scope now or later?
- Template scope: per account only, or allow global templates?
- Matching defaults: choose date window and amount tolerance.
