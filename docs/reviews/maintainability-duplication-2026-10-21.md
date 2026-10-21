# Maintainability duplication pass — 2026-10-21

**Status: second slice of [#284](https://github.com/sergeyfarin/rekenraam/issues/284),
complete within the runtime duplication scope below. #284 remains partial.**
The [first slice](maintainability-audit-2026-10-21.md) records history, earlier
proposals and the provisional shortlist. This slice adds Go and frontend clone
measurements, a disposition for every reported group/pair, and narrower candidate
boundaries. No application, financial rule, API, schema or existing test changes.

Subsequent evidence: the [third slice](maintainability-invariants-2026-10-21.md)
maps selected rule owners and distinguishes deliberate writer/storage protection
from consolidation candidates. It narrows the policy candidate around existing
shared valuation/transfer owners; this report's raw clone results stay unchanged.

## Source and retained evidence

Source remains `13d15eabd68ed2d3b6177512c9c8fe56c528bbf0`, the first slice's
recorded SHA. At the start, `main` was clean at `2f5d51b8`; that intervening
commit only added the first audit artifacts and roadmap update. Inputs are
extracted from Git objects, not ignored/generated files in the working tree.

| Artifact | Purpose |
|---|---|
| [Input preparer](fixtures/maintainability-clones.py) | Extract selected runtime sources; split Svelte scripts/markup while retaining original line numbers |
| [Input manifest](fixtures/maintainability-clone-inputs-2026-10-21.json) | Source and projection SHA-256 hashes for every admitted file |
| [Tool provenance](fixtures/maintainability-clone-provenance-2026-10-21.json) | Versions, module checksum, binary hash, thresholds and totals |
| [Go raw output](fixtures/maintainability-dupl-2026-10-21.txt) | All 75 plumbing rows; 30 connected clone groups |
| [Frontend raw output](fixtures/maintainability-jscpd-2026-10-21.json) | All 57 reported pairs with source fragments and original line coordinates |
| [Disposition catalog](fixtures/maintainability-clone-triage-2026-10-21.csv) | All 87 groups/pairs, their locations, decision and reason |
| [Independent controls](fixtures/maintainability-clone-controls.mjs) and [result](fixtures/maintainability-clone-controls-2026-10-21.json) | Svelte compiler checks, source hashes and positive/negative scanner controls |
| [Tool package](fixtures/maintainability-tools/package.json) and [lock](fixtures/maintainability-tools/pnpm-lock.yaml) | Isolated jscpd dependency graph; no project dependency change |

`G01`–`G30` and `F01`–`F57` are catalog row references, not work-item IDs.
Go groups are connected components of identical reported `(file,start,end)`
spans, sorted lexically by the original path/range strings. Overlapping but
unequal ranges stay separate. Frontend row order follows the retained JSON.
These group counts and frontend pair counts are different units.

## Tools, scope and coverage checks

Go uses the cached [golangci dupl fork](https://github.com/golangci/dupl/tree/c99c5cf5c202),
`v0.0.0-20260401084720-c99c5cf5c202`, built with Go 1.27.0. The module checksum
and actual binary hash are retained. Threshold: **150 serialized AST nodes**,
`-plumbing -files`, with an explicit list of **311 runtime `.go` files** under
`backend/`. Tests are excluded. The tool compares node kinds, ignoring identifier
and literal values; superficially identical SQL readers can have different
queries and financial meaning. It is a candidate generator, not a policy oracle.

Frontend uses [jscpd](https://github.com/kucherenko/jscpd) **4.0.5**, tokenizer
**4.0.1**, with the retained dependency lock. Mode `mild`, minimum **100 tokens
and 5 lines**, maximum 100,000 lines / 2 MB per input. The raised maxima matter:
the default 1,000-line limit would omit some first-slice hotspots. The original
scope is **98 TypeScript files + 153 Svelte components = 251 files**, matching
the first slice's frontend runtime set except its one standalone CSS file.
Generated Paraglide/schema files, unit tests, E2E, contracts, translations and
archives are excluded.

jscpd 4.0.5 has no Svelte grammar. The preparer projects each component into
TypeScript scripts and markup, masking other regions with whitespace while
preserving lines. There are **154 script blocks** (the transaction workspace
has both module and instance scripts), plus the 98 plain TypeScript inputs and
153 markup inputs: **405 projections**. All 405 appear in the accepted report:
252 TypeScript and 153 markup. Neither scanner emitted a parse/skip diagnostic.
The 311 Go inputs were byte-compared with the original Go scan snapshot.

The independent check uses the installed **Svelte compiler 5.57.2** to parse
every component. Its script/style offsets verify exact script contents, masked
non-script regions and unmodified remaining markup. All 562 original input
hashes and all 405 projection hashes are checked. This caught an initial
extractor error on `transaction-table.svelte`: a quoted generic attribute
contained `>`. The quote-aware repair passes every compiler check. The initial
frontend result is rejected as evidence; the corrected full frontend run is
retained. It still finds the same 57 pairs, with corrected input totals of
43,998 lines / 445,237 tokens. The Go pass did not need repeating.

Independent scanner sensitivity controls pass: duplicated Go code is detected;
a short no-clone Go control is clean; copied TypeScript and markup are both
detected; admitted unique TypeScript produces zero matches. A too-short first
frontend negative fixture was skipped without producing a report, so the retained
control uses 20 unique declarations and explicitly reads its zero-match JSON.
A sandbox child-process restriction required the completed controls to run with
process permission; the restriction is not counted as a passing control.

These controls establish input fidelity and detector sensitivity. They do not
make a lexical markup scan a Svelte semantic analysis. Dynamic behavior, code
below the thresholds, cross-language duplication, styles and test duplication
remain outside these measurements. Overlapping matches are not additive lines
of removable code. Projected-source percentages are not a whole-app debt score;
the raw frontend per-match `tokens` field is zero in this reporter, so it is not
used as a clone-size measurement.

## Results and dispositions

| Scan | Result | Candidate rows | Carry to invariant inventory | Retain / no justified extraction |
|---|---:|---:|---:|---:|
| Go runtime | 30 groups / 75 occurrences | 1 | 2 | 27 |
| Frontend runtime | 57 pairs: 49 TypeScript, 8 markup | 11 | 6 | 40 |

Every row has a reason in the catalog. A “candidate” row is evidence for a
bounded proposal, not an accepted refactor or an independent defect. Several
rows support the same proposal. “Inventory” means that a repeated financial
selection/admission rule needs its boundary checked before deciding whether it
is truly common. “Retain” rejects extraction on this evidence; it does not
certify the underlying behavior for every input.

### Corporate-action preparation: retain the candidate, narrow the seam

`G21` finds the three application inverse-plan methods:
`shareExchangeInversePlan` (113–158), `spinOffInversePlan` (115–160), and
`cashStockMergerInversePlan` (124–169). They repeat owner/reason admission,
corrected/imported lineage checks, reading the original journal, posted/version/
date/lifecycle checks and building its inverse. This is concrete support for
the first slice's correction candidate, alongside #309/#310's divergence history.
It qualifies under #284 criteria **1 and 3**.

The useful boundary is a small preparation/verification leaf after the typed
operation lookup. Keep each operation's error mapping and entitlement/ratio/
cash/basis rules explicit. `G27`–`G29` also match typed database reversal and
replacement wrappers, but these **already use**
`executeInvestmentJournalsWithGuardTx` / its preview counterpart. Do not add
another generic writer or move its authoritative rechecks out of the transaction.
Preserve original source evidence, exact-number no-op rules, inverse journal
links, gain acknowledgements and replay of a replaced destination holding.

Recommendation remains **before R13: resolve this bounded preparation seam**;
no refactoring is accepted by this report. Cost M; primary risk is losing a
family-specific refusal or moving a check outside admission. Safety nets are
the three correction families' late-refusal/recheck tests,
`TestCorporateActionCorrectionDistinguishesExactSourceNumbers`, exact source-edit
tests and the #266/part-2 independent registers. They are named reuse targets,
not suites rerun by this documentation slice.

### Frontend review lifecycle: add a bounded candidate

`F03`–`F05`, `F08`, `F16`, `F23`, `F26`, `F34`, `F44`, `F45`, `F53` match
review/confirmation/stale-impact recovery across **13 distinct components**.
Seven use a preview/record flow, four use correction/resolution review state,
and two are provider payment completion forms. They repeatedly retain a payload,
review current checkpoints/gains, clear the dialog, mark pending, commit the
acknowledged set, refresh stale impact and restore an error or review state.
This qualifies under criterion **3**: a financial acknowledgement protocol is
implemented at several client call sites. The server still owns enforcement.

The existing `gain-impact.ts` already owns acknowledgement extraction,
checkpoint override fields, stale-error classification and display rows;
`ReconciliationConfirm` owns the dialog. The candidate is the remaining state
transition/controller plumbing, **not** another amount/gain helper, one merged
investment form, or a client financial calculator. The provider completion
flows are checkpoint-only; they cannot silently acquire gain requirements.
Do not infer that the 13 matched components are the complete caller inventory:
other `isStaleImpactRefusal` users, including cash-in-lieu and activity/reversal
views, exist below these clone thresholds.

Recommendation: **before R13, disposition a typed controller per genuinely
shared protocol or explicitly retain the local flows**. Cost M. Payoff is a
smaller place to maintain reviewed-set/stale-preview behavior as investment
entry and report consumers grow. Risk is reusing stale acknowledgements,
automatically retrying a write, losing errors, or changing pending/focus behavior.
The future slice must characterize cancel, fresh/stale gain and checkpoint sets,
failed refresh and successful exactly-once commit at each participating family.
Keep existing stale-review/reconciliation browser journeys and shared helper
unit tests; no blanket safety claim follows from the clone count.

### Deliberate differences and insufficient-payoff matches

- `G24` matches operation-evidence and commodity export SQL readers, while
  `G30` matches whole-ledger and security-only self-check streams. AST matching
  ignores the SQL literal: merging their scopes would change the contract.
- `G22` matches error switches whose fraction/no-holdings refusals intentionally
  differ. API matches mostly repeat typed decode/call/format adapters;
  authentication, JSON decoding and response leaves already have shared owners.
  `G03` even differs on optional versus required JSON. No generic CRUD/handler
  rewrite is justified.
- `G20`'s import source-revision callbacks and `G26`'s dated long-lot query
  feed the invariant inventory. The latter precedes different spin-off basis
  and merger cash-realization effects; matching selection does not make the
  arithmetic interchangeable.
- Frontend account/holding selection matches (`F11`, `F12`, `F19`, `F30`,
  `F40`, `F49`) also feed that inventory. Current eligibility, dated position
  data and keeping a recorded correction account visible are different concerns.
- `F15`/`F32`/`F51` call existing shared source-evidence, allocation and
  trade-economics helpers. Do not claim those implementations are copied.
  Many other matches are props/imports, autocomplete, first-focus behavior or
  field markup. `F54` deliberately renders recorded and estimated forecasts
  as separate cohorts.
- `F56` identifies the two identical private `apiFetch` helpers in
  `api/connections.ts` and `api/imports.ts`. This is real transport duplication,
  but this slice establishes no bug history, named upcoming N-site change or
  other qualifying payoff for a new ranked refactor. Preserve credential,
  header ordering, request-id and error behavior if later evidence promotes it.
- `F57` identifies account/category metadata assignment helpers. A pattern
  sweep finds a third `assignString` in the institution editor below the scan
  threshold. Likewise, the first slice's short checkpoint-candidate walkers
  and four error classifiers are not disproved by their absence from this scan.
  A future consolidation must sweep the pattern, not only these reported pairs.

The first slice's side/knowledge projection, test-ownership, checkpoint and
error-classification candidates remain provisional. This clone pass supplies
no reason to promote file splitting, schema cleanup or a new replay engine.

## Reproduction

Use temporary directories outside the project dependency graph. Install the
exact dupl version above with `GOBIN=/tmp/rekenraam-284-tools go install
github.com/golangci/dupl@v0.0.0-20260401084720-c99c5cf5c202` (or build that cached
module source, as this run did). Copy the retained tool package and lock into a
fresh `/tmp/rekenraam-284-tools/node` and run `pnpm install --frozen-lockfile
--ignore-scripts` there, using a writable temporary store. No scanner is added
to normal CI or the application manifests.

```sh
python3 docs/reviews/fixtures/maintainability-clones.py \
  --ref 13d15eabd68ed2d3b6177512c9c8fe56c528bbf0 \
  --output /tmp/rekenraam-284-clones-reproduction
```

From the prepared `source/` directory:

```sh
/tmp/rekenraam-284-tools/dupl -t 150 -plumbing -files < ../go-files.txt
```

From its `frontend/` directory:

```sh
node /tmp/rekenraam-284-tools/node/node_modules/jscpd/bin/jscpd . \
  --format typescript,markup --min-lines 5 --min-tokens 100 \
  --max-lines 100000 --max-size 2mb --mode mild \
  --reporters json --silent --output ../jscpd
```

From the repository root, with project frontend dependencies installed:

```sh
node docs/reviews/fixtures/maintainability-clone-controls.mjs \
  /tmp/rekenraam-284-clones-reproduction /tmp/rekenraam-284-tools/dupl \
  /tmp/rekenraam-284-tools/node/node_modules/jscpd/bin/jscpd
```

## Validation and remaining acceptance

Validation **passed**: the accepted scanner runs, independent compiler/hash
checks and four sensitivity outcomes above; independent Git-blob checks for all
562 originals; all 87 catalog rows against raw outputs and source line bounds;
71 relative links/anchors across both reports and the roadmap; both helper syntax
checks; `./scripts/check-post-merge.sh HEAD` (including generated-client stability
and no test loss); and `git diff --check`. A second preparation produced a
byte-identical input manifest. Application suites were not rerun for these
artifact-only changes, and earlier correctness evidence is not re-certified.

The **runtime** duplication pass and its bounded triage are delivered. Still
open at the end of this slice: the money/ledger/lot/security enforcement inventory,
fresh layer/frontend arithmetic/fan-out checks, dead-code and single-caller
analysis, broader test-fixture/oracle and test-duplication work, and final ranked
investment/reporting dispositions. This report neither creates accepted refactor
issues nor clears R13 implementation. Owner review and the issue's remaining
acceptance govern; separate owner/release gates remain unchanged.
The linked third-slice report supersedes this historical remaining-work list
for its bounded invariant matrix; the roadmap retains the current work order.
