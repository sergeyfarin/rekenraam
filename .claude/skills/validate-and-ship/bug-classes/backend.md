# Bug classes — Backend, SQLite and API contract

Part of the `validate-and-ship` review checklist. Services, repositories,
schema, HTTP decoding, uploads, process locks. Every entry shipped as a real
bug in this repo. Numbers are stable IDs cited from code comments and docs;
they are not an order and are never reused.

## 1. Silent limit clamping

Internal full-set reads using a paginated repo method whose limit gets clamped
(import commit processed only 200 of 201 rows). Internal reads use explicit
`ListAll*` methods.

## 2. PATCH omission overwrites

Optional update fields must be pointer types end-to-end; a plain `bool` made
every rename silently disable auto-refresh.

## 4. Split-transaction crash holes

A row and its idempotency marker written in different transactions (T-06, T-26
— both closed; the pattern recurs).

## 8. Error-envelope drift

New error codes not added to the OpenAPI enum; raw Go errors leaking to
clients.

## 10. Logging financial content

Forbidden at every level.

## 36. Canonical text CHECKs that inspect only a valid prefix

SQLite text `length` and `GLOB` stop at an embedded NUL, so a digit-only CHECK
admitted `1\0abc` and trailing NULs in every coefficient column (#192).
Canonical ASCII coefficient CHECKs also require `instr(value, char(0)) = 0`,
outside any zero allowance; this works across SQLite text encodings. Test
installed CHECKs through actual insertions, including embedded/trailing NULs,
signed boundaries and nullable amounts. Named regression:
`TestExactNumberColumnChecks`.

## 40. Checking a process lock without holding it through replacement

`CheckAvailable` releases its lock immediately; it cannot protect a later
restore from concurrent startup (#227). A destructive file replacement needs
ownership throughout the operation, and lock handoffs must preserve one
lock-file inode: unlinking after unlock can split contenders across different
files. #227 retains ownership and keeps the file in place.
`TestTwoProcessRestoreOwnsLockThroughoutReplacement` pauses before/after
replacement and refuses startup and another restore;
`TestTwoProcessLockHandoffsKeepOneFileIdentity` stages pre-opened contenders;
`TestRestoreCommandReleasesLockOnFailure` preserves error-path recovery.

## 41. Upload limits that silently return a prefix

`ParseMultipartForm`'s argument is a memory threshold, not a request limit,
and `LimitReader` alone silently truncates oversized files (#228). Bound the
entire request with `MaxBytesReader`, including unknown-length bodies and
multipart epilogues; check file size and read at most limit + 1 before
accepting. Remove multipart spill files on every path. Prove the exact
supported size works and one byte over an independently parseable prefix
refuses without durable changes. Both staging and CSV analysis must show the
translated size envelope. Named regressions:
`TestImportUploadFileSizeBoundary`, `TestImportUploadMultipartResourceLimits`,
`import-upload-limit.spec.ts`.

## 45. Filesystem paths treated as SQLite URI syntax

Concatenating `file:` with a path lets `#`, `?` and percent escapes select a
different file (#128, #239). Build file URIs with `db.SQLiteFileURL` before
adding query options, including template and auxiliary test connections.
Exercise unnamed subtests (`#00`) and punctuation in the temporary root;
assert migration, copy isolation and reopening the returned URL. Named
regressions: `TestOpenEscapesFixturePaths`,
`TestTemplateAndFixturesEscapeTemporaryRoot`,
`TestFreshDatabaseHelpersEscapeTemporaryRoot`,
`TestRecoveryAndRestoreHelpersEscapeTemporaryRoot` and
`TestRestoreDrillMatchesSourceTrialBalance`.

## 56. Empty data bypassing a native parser negative control

Native text parsers may treat embedded NUL as end-of-input even when Go
strings and SQL bindings retain it. Transaction search rejects NUL before
quoting its literal FTS phrase (#355). Exercise parser refusals against a
**populated** index: an empty FTS table returned 200 without reaching the
failing parser. Named regression:
`TestNativeSQLiteAuditSearchRejectsNULBeforeFTS`.

## 58. Generated types mistaken for runtime schema evidence

OpenAPI 3.1 ignored `nullable: true`, setup omitted implemented-step enum
members, posting tags emitted null arrays, and spending disclosed stale role
aliases (#367–#369). Use explicit null unions, normalize empty response
collections, and check actual HTTP JSON against resolved schemas. The bounded
`scripts/audit-api-contract.py` audit now rejects the repaired #350/#370
discrepancies without exceptions. Generation alone cannot prove requiredness,
enum, status or nullability conformity.

## 59. Mutation journals bypassing display enrichment

Investment and reconciliation responses converted raw records directly, losing
account class, names, commodity codes and symbols (#370). Use the shared owner
for singles and a two-lookup batch for compound results. Compare real HTTP
mutation postings with canonical GETs, including journal-free/null results.
Inject display read failures after COMMIT and preserve the saved operation
exactly once. Internal import receipts need IDs, not display metadata; leave
their commit outcome independent from optional enrichment reads.

## 61. JSON decoder limits drifting from operations and EOF checks

A raw backup-policy decoder bypassed the 1 MiB cap and strictness (#375), and
the shared decoder mapped a trailing-whitespace size error to 400 (#374). Use
the shared required/optional readers, map size errors at both decode steps,
and document 413 only on operations that read capped JSON (#350). Preserve
multipart's distinct 50/51 MiB limits. Keep
`TestCappedJSONOperationsDeclare413`, `TestCappedJSONOperationRefusals`,
`TestJSONBodyLimitTrailingWhitespace`, `TestOptionalJSONBodyLimitBoundary` and
`TestBackupPolicyStrictJSONRefusals` green. Check exact limits and unknown
content lengths through real consumers, including no-write refusal.

## 66. A fresh connection per call hiding connection-lifetime state

Tests opened a new read-only pool per self-check, while the server keeps one
for its lifetime. SQLite 3.53.4's FTS5 `integrity_check` hook then reported a
false checksum mismatch on the reused reader after new searchable entries
(#403). Test helpers reuse a pool for a test's lifetime, as production does.
Lifetime tests need writes that change the state: a transaction without payee
or description leaves the FTS5 index unchanged. Keep
`TestSQLiteIntegrityOnAReusedReaderAfterSearchWrites` (with its canary) and
`TestRepeatedSelfCheckRunsPassAfterNewEntries` green.

## 82. A reference read before the owner's day is pinned

Current account, category, institution, commodity, payee and instrument
versions are selected on the owner's local day (#385), but only when the
context carries it; without one the SQL falls back to `date('now')`, SQLite's
UTC day. The register's account-class lookup (#598) ran before
`enrichRegisterPostings` pinned the day, so just after midnight in Amsterdam
a card created today was invisible and the register returned 500. The
statement rows' commodity lookup (#595) had the same gap. A service method
that makes reference reads pins the day once at its top with
`s.settings.referenceContext(ctx, s.now())` and passes that context to every
read; a helper pinning it for itself does not cover reads made before it.
An unpinned read uses SQLite's real UTC day, not the test's frozen Go clock,
so a test of this class freezes an instant derived from the real day, never a
hard-coded date: a fixed future instant stops exposing the fallback once the
calendar passes it (#601 review). It reads the real day once
(`testdb.RealUTCToday`, or `realUTCToday` in
`e2e/playwright/support/reference-date.ts`) and derives the instant and every
expected date from that reading (`testdb.UTCDayInstant`, `testdb.UTCDay`);
separate clock reads disagree when UTC midnight falls between them. Leave
headroom (owner day a week past the real day) so the gap the test relies on
survives a run that crosses midnight, and check that every case fails with
`referenceContext` disabled: an owner day before SQLite's day is visible to
an unpinned read, so such a case catches nothing without a later version. Named checks:
`TestAccountRegisterReadsTheCardCreatedJustAfterOwnerMidnight`,
`TestOwnerCurrentReferenceHTTPCreateListAndFutureExclusion`,
`TestCurrentReferenceReadsFollowOwnerDay`,
`TestCurrentReferenceSiblingsAndComposedReadsUseOwnerDay`,
`owner-current-reference.spec.ts`.

Gains snapshots also need the pin before their first reference read (#617).
Resolve it from the snapshot's saved zone at the captured instant and give
every reader the resulting context. `TestGainsSnapshotsUseTheOwnerDayForCapitalReturnLabels`
derives an Amsterdam midnight boundary from the real UTC day. It checks the
current holding label, valuation date and UTC cutoff in live and saved gains
and statements.

## 83. A second clock reading after the owner day is pinned

A write pinned the owner day for its reads with `referenceContext`, then
resolved "today" again with `OwnerTodayAt(ctx, …, s.now())` for date
validation and defaults (#566 review, PR #604). A request that crossed
midnight between the two readings accepted, or defaulted to, a date its own
reads treated as the future. An ordinary rename with no date then wrote and
returned a version dated the 16th, while a read pinned to the 15th still
returned the previous version, so the response disagreed with the day the
request was pinned to. Validate and default on the pinned
day (`pinnedOwnerDay(ctx)`), and keep `s.now()` only for instants such as
`recorded_at`. A test arms a clock that crosses midnight right before the
service call and asserts all three effects: a date of tomorrow is refused, an
omitted date defaults to the pinned day, and the response equals the read on
that day. A helper that resolves "today" reads its caller's day:
`pinnedOwnerDay` alone when every caller pins (`cleanLedgerAsOfAndStatus`,
whose ledger overview caller now pins first), or `referenceContext` then
`pinnedOwnerDay` when some caller does not (`AccountReconciliationAges`)
(#605). Named checks:
`TestAccountWritesUseThePinnedOwnerDay`,
`TestCategoryWritesUseThePinnedOwnerDay`,
`TestPayeeWritesUseThePinnedOwnerDay`,
`TestInstitutionWritesUseThePinnedOwnerDay`,
`TestInstrumentWritesUseThePinnedOwnerDay`,
`TestTransactionDefaultDateUsesThePinnedOwnerDay`,
`TestLedgerReadsDefaultAsOfToThePinnedOwnerDay`,
`TestReconciliationAgesJudgeOnTheCallersPinnedDay`,
`TestUndatedFXSourceSelectionUsesTheDayTheRunPinned`,
`TestUndatedFXCutoffUsesTheDayTheRunPinned`,
`TestFXRefreshDemandEligibilityUsesTheDayTheRunPinned` and
`TestFXCoverageUsesTheZoneTheWorkerReadAtEntry`. Resolving the day again
from a captured instant can reread the owner's zone (`OwnerTodayAt` or a
direct `BookOwnerTimeZone` read), so a zone change mid-run moves the day. The FX
refresh did this for source selection, demand eligibility and its
future-date cutoff, and the FX coverage worker for due dates and wakeups
(#605 review). A caller that also needs the zone, for example to schedule a
wakeup at the owner's midnight, takes day and zone from one read with
`referenceContextInZone`. When two checks guard one outcome, test each with
inputs the other accepts: a selection test whose fallback provider answers
with a future date passes on the cutoff alone. A zone-change test changes the
saved zone after the code under test read it (the worker test uses a trigger
on its run row); a pre-pinned context with the new zone already saved cannot
tell one zone read from two. With settings wired, `referenceContext` uses
`db.WithOwnerToday` to pin current reference selection and the live
valuation ceiling together.
`db.WithReferenceDate` changes reference selection alone. Call
`pinnedOwnerDay` with the operation context before applying such an override.
Live observation boundaries keep the supplied recording instant (#609,
`TestRateTableUsesThePinnedOwnerDay`). Blank pricing workspace diagnostics use
the same ceiling, and an explicit diagnostic date retains its meaning
(`TestPricingWorkspaceUsesThePinnedOwnerDay`). #608 (transaction response
enrichment) and #610 (other services' own zone reads) track the remaining
instances. Guard:
`TestOwnerTodayCallersHaveNoPinnedDay` fails when a function both pins and
calls `OwnerTodayAt`, and requires every other `OwnerTodayAt` caller to be
listed with why it has no pinned day to share. It scans one function at a
time, so a helper reached from a pinned caller still needs review.
