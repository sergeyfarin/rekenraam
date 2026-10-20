# Bundled native SQLite audit — 2026-10-20

**Complete: #354's bounded audit.** One application defect, **#355, is fixed**.
Two reproducible upstream engine correctness defects remain in **#356**, blocked
on a released driver carrying the fixes. No affected financial application
consumer or native memory exploit was reproduced. This does not close
**#337's owner release-disposition blocker** or authorize release.

Tracked by [#354](https://github.com/sergeyfarin/rekenraam/issues/354) under
[#260](https://github.com/sergeyfarin/rekenraam/issues/260). The clean starting
commit was `18c16d0f722a56bee3e0d7e0eb729ed8a6c573be`. This delivery adds the
named consumer probes and the NUL search validation below. The native driver,
engine source, compile tags, schema and ADR 0014/0015 policies are unchanged.
All records, files and source inputs exercised here are synthetic.

## Exact source and inventory

**S1 — source authentication.** The admitted driver is
`github.com/mattn/go-sqlite3 v1.14.52`, bundling SQLite **3.53.4**, source ID
`2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`.
The [published upstream amalgamation](https://sqlite.org/2026/sqlite-amalgamation-3530400.zip)
has SHA3-256
`67f423e9ebbbdc473cbc4772c872ee6b89f31fde4ed0279a5c25d5f65c043a16`, matching
the [release record](https://sqlite.org/changes.html). Removing only the
driver's outer `USE_LIBSQLITE3` conditional wrapper yields **byte-identical
engine source**. There are no engine-code differences to hide a missing or
private patch. The unstripped bundled file's SHA-256 and complete runtime
fingerprint/compile options are retained in the
[structured evidence](native-sqlite-evidence-2026-10-20.json).

The [complete advisory matrix](native-sqlite-advisory-matrix-2026-10-20.csv)
reconciles all **54 CVE IDs** in SQLite's current
[recent-CVE catalogue](https://sqlite.org/cves.html) with all **70 advisory
IDs** in [Debian's SQLite source inventory](https://security-tracker.debian.org/tracker/source-package/sqlite3).
Their union is **89 IDs**, including Debian's historical TEMP record; five
additional current non-CVE entries make **94 rows**. Dated source-page hashes
are in the JSON. Rows retain the published fix label, source, classification,
evidence keys, rationale and direct references. No severity filter or
Debian-system-library-to-bundled-engine equivalence is applied.

| Classification | Rows | Disposition |
|---|---:|---|
| Fixed before admitted source | 60 | Published historical fixes precede the authenticated source, including current FTS/JSON fixes and WAL reset; not individual exploit reproductions |
| Feature/path excluded | 14 | Required Session, ZIPFILE, SQLAR, shell/dump/sqldiff/debug consumer absent; actual options/modules/functions and loading refusal checked |
| Unreleased development snapshot excluded | 1 | Published affected development snapshot is distinct from the admitted official source |
| Different component | 9 | Upstream identifies another application/binding, not this native engine |
| Unverified, disputed upstream claim | 7 | Six 2026 claims and CVE-2021-45346 are disputed by upstream; not independently reproduced or silently marked fixed. Debian still lists the latter as unimportant/open |
| Reproduced native-engine defect | 2 | JSON validation cache and expression-index constant propagation; #356 owns driver admission, with no application financial error reproduced |
| Other-version regression excluded | 1 | Upstream explicitly excludes 3.53.4 from the 3.54.0 32-bit VDBE regression; this audit additionally validates Linux/amd64 only |

Important distinctions in the matrix:

- **CVE-2026-11822/11824:** FTS5 is enabled and relevant; upstream identifies
  its fix in 3.53.2, before the authenticated 3.53.4 source. It is not excluded
  merely because FTS receives bound SQL parameters: FTS still parses its own
  query language.
- **CVE-2026-50812/50813:** Session changeset interfaces are not enabled or
  used. In particular, Debian's `3.53.4-2` fixed package for
  [50813](https://security-tracker.debian.org/tracker/CVE-2026-50813) does
  **not** establish that stock upstream 3.53.4 contains its patch. Feature
  exclusion, not that version comparison, is the basis here.
- **CVE-2025-70873 / CVE-2026-39113:** optional ZIPFILE/SQLAR implementations
  and their SQL functions are absent. Go's ZIP export code is a different
  implementation. SQLite's CVE catalogue labels the ZIP fix 3.52.0, while the
  release history marks that release withdrawn; no released-fix claim relies
  on that label.
- The [3.54.1 release](https://sqlite.org/releaselog/3_54_1.html) contains newer
  fixes, so a current version number alone does not certify this older engine.
  Its [32-bit regression report](https://sqlite.org/bugs/info/2026-10-10T18:05:46Z)
  specifically excludes 3.53.4. The two reproduced correctness problems below
  do affect 3.53.4 and are retained rather than hidden by CVE-only inventory.

## Actual consumers and controls

**C1 — engine/connection boundary.**
`TestNativeSQLiteAuditFeatureAndExtensionBoundaries` exercises the production
writer, read-only pool and auxiliary backup connection, then deliberately
discards each physical connection and repeats against its replacement.
FTS3/FTS5/RTREE are present; `ENABLE_SESSION` is absent. ZIPFILE/dbpage modules
and readfile/writefile/sqlar_uncompress functions are absent. SQL
`load_extension` fails **with authorization refusal before looking for a
library** on all six connection paths. This does not claim the C loader is
compiled out: `dlopen` is present in #337's artifact. The registered app driver
has no configured extensions or custom SQL callbacks, and production code
does not call `LoadExtension`, `RegisterFunc` or enable SQL extension loading.

Malformed bound JSON causes a native `malformed JSON` error, then a valid
38-digit **string** coefficient survives `json_extract` unchanged on the same
connection. These are native diagnostic probes, distinct from the real JSON
consumer checked next. `TestNativeSQLiteEngineFeaturesAndFingerprint` records
the engine source ID and all options; the prior image inspection establishes
that Debian's `libsqlite3-0` is not the app's loaded engine.

**C2 — application-owned SQL/FTS/JSON.**
`TestNativeSQLiteAuditSearchAndJSONConsumers` creates balanced transactions
through authenticated HTTP, then reads the actual FTS-backed list and the
JSON-backed category reader. The category's quote/SQL-looking name returns
verbatim while its metadata-derived type remains `expense`. The normal
search finds two independently seeded transactions; literal `OR` and embedded
quotes find exactly the expected row. Operator, column-grammar and SQL-statement
injection controls cannot broaden the result or change any transaction field.
The raw native FTS control **does** return two rows for the same operator
input that the app correctly treats as an unmatched literal phrase, proving
that the test distinguishes parsed grammar from literal input.

The repository binds the quoted FTS phrase; JSON paths, query structure and
sort/filter clauses in these consumers are application-owned. This review
does not claim to audit every endpoint's possible query shape. Stored JSON
and schema expressions are still parsed by native SQLite, including on
operator restore; parameter binding is not a native-parser sandbox.

**C3 — verification and restore.**
`TestNativeSQLiteAuditBackupRefusalsPreserveBothFiles` drives **both real CLI
consumers**, `verify-backup` and `restore`, against five independent cases:
truncated database, invalid page-one b-tree type, malformed schema, a migration
view requesting extension loading, and a deliberately unbounded recursive
migration view. All **ten attempts refuse**. The extension view fails as unsafe
schema use; no library is loaded. The recursive view is interrupted by a
200 ms caller context. In each case, SHA-256 of the source and live database
is unchanged, no preservation/replacement starts, and no `.restoring` file
appears. Context timing is a cancellation control, not a performance claim.

Positive controls use the existing verify/restore CLI tests and the full
backend gate's `TestRecoveryAuditReferenceBookRestoreDrill`: real product
backup, `InspectBackup`, fresh restore, sign-in, every independent #266
register figure and normalized export comparison. Existing foreign-key
violation, newer-schema, WAL-preservation, process-lock and retained-key
controls pass in that gate too. A passing b-tree/integrity check alone is
not evidence that an arbitrary supplied schema is a safe application backup.

## Findings

### #355 — malformed search returns a server error, fixed

With a **populated** transaction index, `q=%00`, `q=AuditAlpha%00` and
`q=AuditAlpha%00+OR+missing` each returned HTTP **500 INTERNAL_ERROR** at the
baseline. Native FTS sees end-of-input at NUL and loses the closing phrase
quote. An initially empty fixture returned 200 without invoking the failing
parser; the corrected negative control reaches real indexed data.

The application service now rejects NUL before FTS using the existing
**400 VALIDATION_FAILED** contract; all three cases pass without exposing
parser details or changing the transaction. The OpenAPI description, generated
client documentation, governing ledger plan and recurring-bug checklist agree.
`TestNativeSQLiteAuditSearchRejectsNULBeforeFTS` is the named before/after
regression. **P3 application defect, fixed; no native memory exploit observed.**

### #356 — two upstream correctness defects, retained

**P1 — independent native probe.** The
[retained source](fixtures/native-sqlite-upstream-probe.go.txt) runs against
the admitted driver and prints independently expected and observed values;
the JSON evidence retains its output.

1. [JSON validation cache](https://sqlite.org/bugs/info/2026-10-11T18:32:23Z):
   the editor produces canonical `[31,2]`, whose validity must be **1**;
   the same statement returns **0**. The current app's source-revision reads
   use `json_remove`, but do not combine it with the affected cached
   `json_valid` statement shape. **Engine bug reproduced; affected application
   consumer not reproduced.**
2. [Expression-index constant propagation](https://sqlite.org/bugs/info/2026-10-11T19:26:40Z):
   a one-row indexed join/subquery must return **1** and update `old` to
   `new`; it returns **0** and leaves `old`. The independent `NOT INDEXED`
   control returns **1**. The current synthetic schema has **one expression
   key**, `IFNULL(account_id,0)` in `investment_fee_policy_scope_idx`.
   Reviewed fee-policy reads use account equality/NULL predicates, not the
   affected expression-valued join constraint. Existing fee-treatment and
   policy-version tests pass. **Engine bug reproduced; no incorrect app fee
   selection or financial figure established.**

Upstream fixes both in SQLite 3.54.1. The official driver's release/tag APIs
still report **v1.14.52** as latest on 2026-10-20; it bundles 3.53.4.
[#356](https://github.com/sergeyfarin/rekenraam/issues/356) requires a released
driver carrying those fixes, explicit source/feature admission and replay of
the probes and financial gates. It is **P3 dependency work, blocked on that
upstream release**. No private engine patch, untagged fork or unreviewed
feature change is substituted here. This independent finding does not leave
the original #354 audit acceptance unfinished.

## Validation and disposition

- Focused native/API/CLI probes and source fingerprint pass after #355;
  the three populated-index controls fail before that fix and pass after it.
- `./scripts/test-backend.sh` **passes**: formatting, vet, and the full
  race gate with two concurrent packages/five-minute package deadline.
  Fresh measured packages: CLI 74.398 s, API 112.642 s, app 198.510 s,
  DB 118.212 s. Unchanged helper packages may use Go's test cache.
- `./scripts/test-frontend.sh` **passes**: 66 catalog cases, generated API
  types, zero Svelte diagnostics and **691 unit cases**. No UI runtime change
  or fresh browser-suite claim is made.
- Downloaded official source matches exactly after the wrapper removal;
  matrix keys/counts reconcile to both inventories, CSV/JSON parse, local
  report links resolve, retained Go probe is formatted and `git diff --check`
  passes. Upstream probe deviations remain **expected finding evidence**,
  not falsely reported as passing engine behavior.

**Disposition:** #354's bounded review is complete with #355 fixed and the
explicit #356 dependency finding retained. No further independent audit was
identified in this scope. #260 and release guidance record both outcomes.
#337 remains blocked on its separate owner decision; this audit supplies
additional evidence, not that decision.

The seven disputed claims remain **unverified**, not disproved by these tests.
This is not ASAN/UBSAN/fuzz coverage or certification against unknown native
bugs; Go's race detector does not instrument the C engine. Writer/auxiliary
connections retain existing SQLite defaults for defensive/trusted-schema
configuration; no speculative hardening rule is introduced. Restore is an
operator-selected local file workflow, not an unauthenticated upload or an SQL
sandbox. The CLI honors SIGINT/SIGTERM/caller context, but does not impose the
probe's 200 ms default on arbitrary backups. Opening a supplied database
necessarily parses its schema, even for read-only integrity verification.

## Reproduction

```sh
./scripts/go-backend.sh test ./internal/api ./internal/db ./cmd/rekenraam \
  -run 'TestNativeSQLiteAudit|TestNativeSQLiteEngineFeaturesAndFingerprint|TestVerifyBackupCommand|TestRestoreCommandInstallsAndPreserves' \
  -count=1 -v
./scripts/test-backend.sh
./scripts/test-frontend.sh
cp docs/reviews/fixtures/native-sqlite-upstream-probe.go.txt /tmp/native-sqlite-upstream-probe.go
./scripts/go-backend.sh run /tmp/native-sqlite-upstream-probe.go
```

The last command intentionally emits the two known wrong native results plus
their independent expected values; it is evidence for #356, not a normal green
regression test. Its optional argument is the path to a **synthetic** current
backup, opened read-only to count expression-index keys. This audit used
#337's synthetic post-upgrade book; migration trees are unchanged. No raw
financial data, keys, environment values or owner backup paths are published.
