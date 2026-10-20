# Runtime API response-contract audit — 2026-10-20

Issue [#349](https://github.com/sergeyfarin/rekenraam/issues/349), parents
[#280](https://github.com/sergeyfarin/rekenraam/issues/280) and
[#260](https://github.com/sergeyfarin/rekenraam/issues/260).

## Scope and reproducible evidence

The original baseline is `02f1c54eca6ac502428b32ba3590d055dd8ddd2f`, with the
new opt-in test capture wrapper added. The first run reused 22 existing named
HTTP consumers: 292 responses, 69 sampled operations and 201 distinct
operation/status/test/error rows; 91 rows failed the declared response schema.
[Baseline results](evidence/api-contract-audit-2026-10-20/baseline-responses.csv)
retain those failures. The later expanded selection adds independent regressions,
registers, backup/optional-body consumers, ZIP checksums and saved report runs.

Final source SHA, counts, versions and installed format checkers are in
[summary.json](evidence/api-contract-audit-2026-10-20/summary.json).
The source commit includes the harness and the small repairs; the subsequent
publication commit adds evidence/current-state documentation only.

- [Every declared operation](evidence/api-contract-audit-2026-10-20/operations.csv):
  declared/observed statuses, unverified declared statuses and disposition.
- [Every sampled response group](evidence/api-contract-audit-2026-10-20/responses.csv):
  operation/status, exact named consumer, sample count, independent OpenAPI
  expectation, actual root JSON types or download size, schema failures and
  issue disposition. Different nested failures remain distinct rows.
- [Sensitivity controls](evidence/api-contract-audit-2026-10-20/controls.csv):
  eight controls each admit a valid independent payload, reject an invalid
  payload, and reject the valid payload against a deliberately damaged schema.

The final sample has **425 responses across 80 of 332 operations**, grouped into
281 operation/status/test/error rows. Sixteen rows retain known failures
(#350: one; #370: fifteen); zero discrepancies are untracked. The other
**252 operations** and every unobserved declared status remain unverified.

The current checked contract has **332 operations**, rather than the issue's
historical 329. This inventory is over declared method/path pairs; it does not
replace the earlier independent router-parity audit. An observed status does not
imply that every possible payload for that status conforms. Unobserved statuses
and operations are explicitly **unverified**, not failed or certified.

## Method and independent expectations

`python3 scripts/audit-api-contract.py --output /tmp/rekenraam-api-audit`
runs the named consumer selection through real `NewHandler` middleware,
application services, isolated native SQLite books and the production read-only
export pool. Existing consumer assertions remain in place. Handler wiring is
wrapped only when `REKENRAAM_CONTRACT_AUDIT_DIR` is set; ordinary tests and the
production server do not capture responses.

The Python runner resolves only checked-in references inside `api/openapi`,
then uses jsonschema's Draft 2020-12 validator for the declared OpenAPI 3.1
schemas. It checks actual status and content type before validating JSON.
It does not translate obsolete `nullable` annotations into a different contract.
JSON coefficients are validated as strings, independently of Go DTO decoding.
Every refusal is also checked against the shared error schema, including the
undocumented 413 sample. No response schema is inferred from the implementation.

Raw synthetic bodies, which may include test credentials, are written as 0600
files in a private temporary directory and removed after validation. Published
results contain field names/types and error locations, without response bodies,
cookies, tokens, passwords or financial amounts.

| Concern | Independent expectation and named consumer | Actual / disposition |
|---|---|---|
| Requiredness and refusal envelope | `TestSetupStatusReturnsInitialSteps`, `TestLoginRejectsInvalidCredentials`, `TestLogoutRejectsInvalidCSRFToken`, `TestTransactionValidationAndLifecycleGuards`; required envelope/code/message from OpenAPI | Sampled refusals validate. Removing required envelope data, inventing a code, adding an impossible schema requirement and changing the allowed code all fail controls. |
| Exact coefficients | `TestInvestmentMoneyAboveJavaScriptSafeIntegerRoundTripsAsStrings`, `TestPriceCoefficientsRemainExactAboveBrowserSafeInteger`, `TestForecastAPIReturnsExactWireQuantities`, reference register | Real consumers retain exact string coefficients beyond browser-safe integers. Numeric coefficients and noncanonical `01` fail controls; changing the contract type to integer or its pattern to zero-only rejects valid controls. |
| Unknown versus absent | `TestUnknownProjectedBasisAPIKeepsQuantityAndReturnsNullBasis` | Lots, positions and unrealized gains explicitly return null basis pairs for unknown basis, preserve quantity, and omit unavailable unrealized gain. No unknown-to-zero coercion. Explicit-null and nonnullable controls reject opposite schema mutations. |
| Nullable display fields | Real transaction/create/read/list and system-posting responses | #367 replaces eleven obsolete annotations with 3.1 type/null unions. The standalone explicit-null control fails before the repair and passes after it. Requiredness is unchanged. |
| Omitted versus cleared PATCH fields | `TestRecurringTemplateAPICreatePatchReadArchive` | A name-only rename preserves enabled, postings, payee, tags and schedule; explicit null clears the nullable fields. Independent omission and explicit-null payload/schema controls pass. |
| Cursor responses | `TestListDeletedTransactionsCursorPagination`, `TestInvestmentGainsRunCursorPagination` | Existing consumers check page boundaries/order; captured pages validate against their response schemas. Other cursor operations remain unverified unless listed in the matrix. |
| Setup and exclusion enums | Named setup consumers; `TestSpendingResponseContractNamesActualExcludedSystemRoles` | #368 adds system_accounts/categories to implemented_steps and corrects stale spending metadata. The independent eight-role assertion fails before the fix. The repository already excludes every system account; report amounts and role definitions are unchanged. |
| Empty and nonempty tags | `TestPostingResponseContractEmptyAndNonemptyTags` | #369 fixes transaction and register posting DTOs to emit [] for empty tags, preserving nonempty IDs. Both mapper paths and register transaction_tag_ids have recorded failing-before assertions. |
| Binary/download media | `TestLedgerExportRendersStoredScaleWithoutFloat`, `TestQIFRoundTripsUnderTheDeclaredLayout`, `TestExportBundleChecksumsVerify`, `TestInvestmentGainsRunReproducesBeforeSaleReversal` | CSV, QIF, ZIP and saved JSON media match their contracts. Existing tests parse/reconstruct bytes and verify checksums/saved document equality; the harness checks media/schema and nonempty binary payloads, not arbitrary binary structure. |
| Capped JSON / multipart refusal | `TestOversizedJSONRequestUsesDocumentedError`, `TestImportUploadFileSizeBoundary`, `TestImportUploadMultipartResourceLimits`, `TestRequestBackupReportsAcceptedWorkNotACompletedBackup` | #350 still owns undocumented owner-setup JSON 413; its actual error envelope conforms and no owner is created. Import 50 MiB file / 51 MiB request and resource-limit consumers retain refusal/no-write controls. Empty optional backup body returns accepted work. This is not the complete capped-decoder inventory required by #350. |
| Nested mutation display data | Reference-book, unknown-basis, exact-money and saved-run consumers | #370 retains blank account_class/commodity_code and missing display enrichment in sampled buy/sell/short/inbound/split/capital-return/reversal and reconciliation mutation responses. Strict schema failures stay visible in the matrix; durable register checks pass. No financial loss was reproduced. |

## Findings and disposition

- **#367 fixed:** eleven 3.1 null unions across transaction, book, backup, export
  and import-cursor schemas. Type generation preserves nullable unions and adds
  the newly declared setup-step members.
- **#368 fixed:** setup-step enum plus truthful spending exclusion metadata,
  derived from the existing eight-role catalog, replacing obsolete aliases.
- **#369 fixed:** empty posting tags serialized as arrays at both DTO mappers, plus empty
  register transaction_tag_ids normalized to arrays.
- **#370 open:** broader shared-writer response enrichment repair. Its issue
  requires the producer inventory, mutation-versus-GET checks, bounded query
  counts and explicit treatment of enrichment failures after committed writes.
- **#350 open:** existing capped-JSON 413 contract inventory/repair. The audit
  reproduces it without expanding unrelated multipart limits.

Default audit exit status fails for new discrepancies or failed controls; known
#350/#370 failures are narrowly classified by method/path/status and exact field
failure. They remain failures in the CSV and operation disposition. `--strict`
also exits unsuccessfully for those retained findings. No entire operation is
excluded from validation, including the nullable split transaction: its object
branch's leaf errors are inspected. Required fields, other enums, error codes,
media types and other statuses cannot be silently waived by these classifications.

No additional independent audit is required to finish #349's agreed bounded
sample. Unverified operations/statuses are retained as coverage limits, and #370
owns its repair inventory. #349 can close as an executed audit with all confirmed
findings individually tracked. This does **not** certify universal API conformity
or close #260, #350 or #370.

## Validation and retained limits

Validation completed before publication:

| Command / evidence | Result |
|---|---|
| `./scripts/test-backend.sh` | Format, vet and all native race packages pass; app 265.213s, API 129.287s, DB 197.695s. |
| Final whole API race pass | `./scripts/go-backend.sh test -race ./internal/api -count=1 -timeout=5m`: passes, 182.344s. |
| Final register-collection extension | Both named tag/exclusion contract regressions pass under race, 4.520s. This runs after the last register transaction-tag normalization. |
| `./scripts/test-frontend.sh` | Generated client, zero Svelte errors/warnings, 66 catalog cases and 691 unit cases pass. |
| Focused response/schema runner at `a4e488a22bb4e79d6621b014ea53fd25238e5f1b` | 425 actual responses, 80 operations, eight controls, zero new discrepancies. Clean committed source; known failures remain visible. |
| Source/evidence review | `git diff --check`, local evidence links and matrix/disposition consistency pass. |

The full backend gate and whole API race pass preceded the final small register
transaction-tag normalization; the final named race regression and committed-SHA
HTTP/schema run exercise that extension. No repeated whole-app gate is implied.
Remote CI is pending at push, not a remote-green claim.

This is synthetic, provider-free evidence. No live provider, owner deployment,
release-signing or household-custody evidence is inferred. No complete enum
coverage, every refusal status, fault injection into every handler, streaming
latency, or universal format validation is claimed. JSON Schema formats are
annotations unless an installed checker asserts them; the recorded environment
has no RFC 3339 `date-time` checker. Unsupported date-time/other optional formats
remain unverified. The runner records installed checkers for reproducibility.
Binary content assertions are bounded to the named existing consumers. Full
browser, large-book and release gates were not repeated for this bounded audit;
the earlier #260 evidence retains its original SHA and limits.


## Follow-up delivery — #370, 2026-10-20

[#370 is fixed](mutation-response-enrichment-2026-10-20.md), including the
producer inventory, canonical-GET comparisons, two-lookup batches, post-commit
read-failure controls and internal import receipt isolation. Its expanded audit
records 1,108 responses across 165 of 332 operations with eight schema controls.
All fifteen original enrichment failures are repaired, and the #370 exception
is removed from the runner. Small findings #371/#372 are fixed; only #350's
known 413 status gap remains. The original 425-response matrix above remains
historical evidence; the new report explicitly retains its 167 unverified
operations, unobserved statuses and format limits.
