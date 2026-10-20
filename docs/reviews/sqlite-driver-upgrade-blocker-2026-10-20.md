# SQLite driver upgrade prerequisite — 2026-10-20

**#356 remains OPEN / BLOCKED.** The prerequisite was rechecked at clean main
`7b07a2f68bff878a45905b67294fa5135fc2b0ce`. There is still no released
`github.com/mattn/go-sqlite3` candidate carrying the required fixes. This
review supplies a functional admission probe and verifies the official fixed
engine independently; it does not deliver the application upgrade.

## Upstream evidence

- The official [latest release](https://github.com/mattn/go-sqlite3/releases/latest)
  and tag APIs still identify **v1.14.52**, released 2026-09-05.
  `./scripts/go-backend.sh list -m -json github.com/mattn/go-sqlite3@latest`
  independently resolves the same version and module checksum.
- The current upstream `sqlite3-binding.c` blob is
  `4a3437f57df4d9a3f3c922d457904a7f42474ba1`, **identical** to the admitted
  module's bundled source. Upstream master therefore does not supply a newer
  engine candidate either. The recorded blob identity is point-in-time
  evidence, not a promise about future master.
- Official [SQLite 3.54.1](https://sqlite.org/releaselog/3_54_1.html) exists.
  Its downloaded amalgamation's SHA3-256 is
  `e2b869d3003b33c942b7dd7063604fbc53553e1b66609570df39b9b7b558ee81`, matching
  the published release. That is an engine release, not a Go driver release.

[Structured evidence](sqlite-driver-upgrade-blocker-2026-10-20.json) records
the check timestamp, release/module/source identities, checksums, compiler,
all explicit C defines, both engine fingerprints and actual probe results.
No module cache, application dependency, SQLite schema or runtime setting was
modified. No private fork or system-library linkage was introduced.

## Independent functional check

The [retained C probe](fixtures/native-sqlite-engine-admission-probe.c.txt)
uses the same synthetic SQL and independently expected results as #354's
Go-driver probe. It compiles separately with each authenticated official
amalgamation, using the driver's explicit C feature defines plus FTS5.
Both compiled engines use their own matching headers. All data is in memory.

| Result | Expected | Official 3.53.4 | Official 3.54.1 |
|---|---|---|---|
| Edited canonical JSON is valid | 1 | **0** | **1** |
| Indexed join/subquery matches its sole eligible row | 1 | **0** | **1** |
| Independent `NOT INDEXED` control | 1 | **1** | **1** |
| Eligible row changed from `old` to `new` | 1 | **0** | **1** |
| `--check-fixed` exit code | 0 | **1**, correct negative control | **0**, passes |

The unchanged admitted Go driver's retained probe also reproduces validity 0,
indexed count 0 and unchanged `old`, with its independent control 1. Thus the
new functional gate detects the demonstrated problems; it does not pass just
because an engine advertises a newer version.

The standalone 3.54.1 engine reports source ID
`2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76`.
Its options add `MAX_SCHEMA=10000000` relative to the matched 3.53.4 build;
no options disappear. These are standalone native-engine options, not an
admitted Go-driver fingerprint. Neither WAL/FULL connection hooks nor real
application/migration/backup behavior is established by this C diagnostic.

## Unblock and remaining acceptance

[#356](https://github.com/sergeyfarin/rekenraam/issues/356) explicitly requires
a **released upstream driver** carrying both fixes. Its three original
acceptance items remain unchecked. A separately reviewed backport/fork is a
different scope decision, as that issue already states.

When a released candidate exists, verify its exact bundled source and module
checksum, run both functional probes, record options, then run the existing
search/JSON/cancellation/backup/restore/fee-policy/reference-book consumers,
the shared backend race/format/vet and integrated build gates, and reconcile
the native advisory matrix. None of those application-upgrade gates is
claimed complete against 3.54.1 here. No full application suite is repeated
against the unchanged production dependency for this evidence-only change.

The [#354 report](native-sqlite-advisory-audit-2026-10-20.md) remains the
application-consumer/advisory evidence: two native correctness defects were
reproduced, with no affected financial app consumer or native memory exploit
demonstrated. No new independent defect or additional audit was identified
by this prerequisite check. #260 and #356 record the retained blocker;
#337's owner release-disposition requirement remains separate.

## Reproduction

Extract the official 2026 amalgamation archives for 3.53.4 (`3530400`) and
3.54.1 (`3540100`) into separate private temporary directories. Copy the probe
to a `.c` path and compile separately against each directory's `sqlite3.c`
and `sqlite3.h`. The exact defines are retained in the JSON; for example,
with the fixed source extracted as `/tmp/sqlite-amalgamation-3540100`:

```sh
cp docs/reviews/fixtures/native-sqlite-engine-admission-probe.c.txt /tmp/sqlite-admission-probe.c
cc -O2 -std=gnu99 -DSQLITE_ENABLE_FTS5 -DSQLITE_ENABLE_RTREE \
  -DSQLITE_THREADSAFE=1 -DHAVE_USLEEP=1 -DSQLITE_ENABLE_FTS3 \
  -DSQLITE_ENABLE_FTS3_PARENTHESIS -DSQLITE_TRACE_SIZE_LIMIT=15 \
  -DSQLITE_OMIT_DEPRECATED -DSQLITE_DEFAULT_WAL_SYNCHRONOUS=1 \
  -DSQLITE_ENABLE_UPDATE_DELETE_LIMIT \
  -I /tmp/sqlite-amalgamation-3540100 \
  /tmp/sqlite-amalgamation-3540100/sqlite3.c /tmp/sqlite-admission-probe.c \
  -lm -ldl -lpthread -o /tmp/sqlite-admission-probe
/tmp/sqlite-admission-probe --check-fixed
```

Repeat with 3.53.4 in a separate directory: expected exit **1**, not a passing
upgrade. An invocation without `--check-fixed` only prints observations.
Compiler errors or native SQL errors produce exit **2**. This is an isolated
diagnostic, not an alternative production build workflow.
