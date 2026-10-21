# Capped JSON request contract — #350

Date: 2026-10-21. Parent audits: #260, #280 and #349.
Baseline: `a54df1e113b0313d3178708ebbe0e892388603f0`.
Audited implementation: `957b60393733d5cd3c566b558ff55c3fa4e98289` (clean committed tree).

## Result and disposition

All **215 capped JSON operations** declare and return HTTP 413 with the actual
`REQUEST_TOO_LARGE` error envelope. The source-to-contract inventory covers all
332 declared operations: 206 required JSON readers, nine optional readers,
116 operations without a capped JSON reader, and the multipart-only analyze
operation. `/imports` is counted among the required JSON readers and separately
retains multipart support. Its 413 describes both branches; the 214 JSON-only
operations reuse `JSONBodyTooLarge`.

The sweep found and repaired two small independent defects, both logged before
repair: **#374**, oversized trailing whitespace returning 400 instead of 413;
and **#375**, backup policy bypassing the shared decoder. The route now shares
JSON content-type, unknown-field, single-object and 1 MiB validation. Valid
omitted-field policy updates retain their existing behavior. The prior decoder
and the raw backup-policy consumer were the only API JSON readers found by the
source sweep; the inventory test now guards against introducing another raw
reader.

Validation also exposed the previously shipped enrollment fixture's clock
boundary #376. Ten isolated repetitions passed; a controlled stale first clock
sample then reproduced the same 401 before the fix. The helper now refreshes
only the expected invalid-code refusal when the sampled TOTP step advances,
with at most three attempts. Its real-handler regression passes without sleeps
or any production auth-clock/skew change; non-boundary failures remain hard
failures. The original full gate passed all other packages and failed only this
fixture; the final full gate passes after its repair in `a95260918bdb750fe8fc22f41c0170fe3df1cbdb`.

**#350, #374, #375 and #376 meet their acceptance.** No independent additional audit
was identified. Existing broader #260 release holds and unobserved operation
statuses are unaffected. This is a bounded status/decoder delivery.

## Inventory and independent controls

[Source/contract inventory](evidence/json-body-contract-2026-10-21/body-operations.csv)
records every method, route, decoder, path document and declared 413. The Go AST
walk follows handler closures, shared lifecycle helpers and generic
`correctionRoute[T]`. The only action refinement is the event-suggestion status
helper: accept reads optional JSON, link requires JSON, ignore reads no body.
No 1 MiB or 413 claim is added to bodyless actions. All documented JSON request
consumers now map to a shared reader; all registered capped readers map to a
contract operation.

| Named control | Independent expectation and actual result |
|---|---|
| `TestCappedJSONOperationsDeclare413` | Every mapped reader has a 413; only the multipart-only analyze operation has a 413 without a capped JSON reader. The original spec fails this check; the delivered spec passes. |
| `TestCappedJSONOperationRefusals` | Each of the 215 real registered operations refuses an oversized JSON-whitespace prefix with known and unknown content lengths: **430 refusals**, exact error envelope, zero SQLite writes. Public owner/login, authenticated mutations/previews and all nine optional consumers are included. The separate import-connection fixture supplies its actually registered service; no provider call occurs. |
| `TestOversizedJSONRequestUsesDocumentedError` | A large value returns 413 with `REQUEST_TOO_LARGE`, the stable message and zero owner rows. |
| `TestJSONBodyLimitTrailingWhitespace` | A valid public owner request padded to exactly 1,048,576 bytes succeeds; 1,048,577 bytes returns 413 and creates no owner, with either length mode. Before #374, both oversize controls returned 400; the exact-limit controls passed. |
| `TestOptionalJSONBodyLimitBoundary` | A real authenticated institution archive accepts absent JSON and an exact-1-MiB object; one byte more returns 413 without any SQLite write, with either length mode. |
| `TestBackupPolicyStrictJSONRefusals` | Oversized input returns 413; unknown fields, multiple objects and non-JSON content return 400. All preserve the canonical policy response and SQLite change counter. Before #375 every one returned 200 and mutated policy. |
| `TestBackupPolicyUpdateKeepsOmittedFields` | Valid partial policy updates preserve omitted values. |
| `TestImportUploadFileSizeBoundary` | Parseable QIF/CSV files at exactly 50 MiB succeed; one byte more refuses with `IMPORT_UPLOAD_TOO_LARGE`, without truncation or writes, with either length mode. |
| `TestImportUploadMultipartResourceLimits` | Separate 51 MiB request/field/part/header/epilogue limits retain their named refusals, no durable changes and multipart-spill cleanup. |
| `request-body-contract.test.ts` | Generated public, authenticated, optional and backup response unions expose 413 through the shared component; both upload 413s retain the error-envelope type. Frontend type checking and both named type cases pass. |
| `json_413_status` schema sensitivity | A 413 envelope validates against the real owner operation; deleting its 413 entry must produce `undocumented status 413`. All nine schema sensitivity controls pass. |

The no-write checks use SQLite `total_changes()` on each isolated fixture's
single writer connection, sampled immediately around the refused request. This
also catches attempted writes that roll back; there is no worker or competing
writer in these controls. Owner boundary controls independently count users;
backup controls independently compare canonical GET output.

## Validation and reproduction

Run from the repository root; use `GOCACHE=/tmp/rekenraam-go-cache` when the
normal cache is outside the writable sandbox. No script or scheduling changes
are needed.

```sh
./scripts/go-backend.sh test ./internal/api -count=1 \
  -run '^(TestCappedJSONOperationsDeclare413|TestCappedJSONOperationRefusals|TestOversizedJSONRequestUsesDocumentedError|TestJSONBodyLimitTrailingWhitespace|TestOptionalJSONBodyLimitBoundary|TestBackupPolicyStrictJSONRefusals|TestBackupPolicyUpdateKeepsOmittedFields|TestImportUploadFileSizeBoundary|TestImportUploadMultipartResourceLimits)$'
./scripts/test-backend.sh
./scripts/test-frontend.sh
REKENRAAM_BODY_INVENTORY=/tmp/body-operations.csv \
  ./scripts/go-backend.sh test ./internal/api -count=1 \
  -run '^TestCappedJSONOperationsDeclare413$'
python3 scripts/audit-api-contract.py --output /tmp/json-body-audit --strict
```

Backend format/vet/race: pass. Frontend: zero diagnostics, 66 catalog
cases, 693 unit cases. The final committed-source strict audit records
1582 responses / 251 of 332 operations / nine sensitivity
controls / **zero discrepancies**. The #350 exemption has been removed. Full
body-free [response/status/test evidence](evidence/json-body-contract-2026-10-21/responses.csv),
[all-operation status matrix](evidence/json-body-contract-2026-10-21/operations.csv),
[controls](evidence/json-body-contract-2026-10-21/controls.csv) and
[source/environment summary](evidence/json-body-contract-2026-10-21/summary.json)
are retained with LF-normalized CSV line endings; temporary synthetic response
bodies are deleted by the runner.

## Limits

All capped operations' 413 status is observed; this does not prove their success
or other error branches. The 81 unsampled operations and every unobserved
status are explicitly listed in the operation matrix. The resolver validates
local OpenAPI JSON Schema, requiredness, enums and installed format checks;
optional `date-time`/URI format checks are not installed in this environment.
No universal API conformity, actual deployment, live-provider, native-language,
financial recalculation or public-release claim is made. Existing localized
size guidance is retained in all six catalogs. No limit is increased, multipart
parser behavior is unchanged, and no production schema or ledger rules change.
