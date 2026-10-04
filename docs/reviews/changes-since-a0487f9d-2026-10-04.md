# Review of changes since a0487f9d (2026-10-04)

## Scope and conclusion

Reviewed `a0487f9d35e2066a2118785bc1134e582b539c31..49cbc88b61498a2674dd602c98e20c98d38cadc2`:
31 commits, 485 changed files. The base commit is excluded. GitHub closure
history was read using the base commit's timestamp, 2026-10-02 18:04:14 UTC,
rather than the whole calendar day: 18 issues closed after that timestamp.
Issues #99 and #142 closed earlier that day and are outside this closure audit.

**This is work in progress.** R16 is still current, and GitHub has 26 open
issues at the review snapshot, including the roadmap index. Outbound and
unknown-basis transfers, return of capital, cash in lieu, short sale/cover and
compound actions are not completed by these commits. Their absence is tracked
scope, not evidence that the closed correction slices never worked.

The direction is sound: immutable operations and correction roots, exact
arithmetic, one audited writer transaction, writer-derived rolled-back previews,
gain acknowledgement independent of reconciliation, and current-state SQL views
form a coherent foundation. The new replay-equivalence verifier is particularly
useful: it already caught a lost method-lock defect. No confirmed destructive
ledger mutation or silent gain-acknowledgement bypass was found in this review.
Three behavioral defects were reproduced, and documentation/closure records need
cleanup. The review is not a release certification or proof of every possible
history; the remaining work and limits below still matter.

## Findings

### 1. P2 — reinvestment correction leaves the cached gains report unchanged

Location: `frontend/src/lib/investments/dividend-correction-form.svelte:180–191`.

The correction commits successfully and invalidates positions, lots and forecast,
but never invalidates `investmentGainsQueryKey`. Its parent callback refreshes
transaction data and the correction chain, not gains. The root query client
survives client-side navigation, and gains have a 30-second freshness window.

**Reproduced in a browser:** reinvest 10 shares for 20.00, buy another 10
for 200.00, and sell 5 for 150.00. Open Gains to cache the FIFO gain of 140.00;
navigate within the app to its reinvestment; correct the reinvested quantity
from 10 to 4 and amount from 20.00 to 40.00; explicitly accept the disclosed
restatement; return to Gains. The API reports 90.00 while the table still shows
140.00. The temporary Playwright probe passed the API assertion and failed the
final displayed-value assertion. This affects trust immediately after the user
has acknowledged the new value, although the stored financial result is correct.

The reversal dispatcher at
`frontend/src/lib/transactions/investment-correction-section.svelte:176–192`
also only calls the transaction refresh callback. New split, reinvestment,
write-off and transfer reversal paths repeat that omission for investment
positions, lots and gains. Buy/sale reversals already had this pattern before
the base; the new families extend it. Sweep all investment mutation paths,
invalidate the affected investment query families centrally, and add a test
that preloads Gains, corrects/reverses through client-side navigation and
checks the rendered result. Existing correction tests primarily check the
post-correction API result and miss this cache state.

### 2. P2 — split source linking does not recheck a discarded batch

Locations: `backend/internal/app/import_trading212_split.go:52–84,116–135`
and `backend/internal/db/import_split_link.go:85–100`.

The service checks that the batch is not discarded/rolled back before preparing
its link parameters. The repository transaction rechecks the staged row and the
effective split, but never the parent batch's status. A discard between the
service read and the write can therefore admit source evidence after the batch
has become unavailable. SQLite's one-connection pool serializes transactions,
but it does not make the separate preparation read and write atomic.

**Reproduced with a deterministic interleaving:** run the service's split-row
preparation, discard the previewing batch through `DiscardImportBatch`, then run
`LinkStagedRowToSplit` with exactly the prepared parameters. The writer succeeds;
the batch is `discarded`, and its row newly becomes `committed`. No raw database
mutation was used. This does not create a second split journal, but it creates
committed identity/evidence after the service's admission condition ceased to
hold. The accepted identity can affect later deduplication and source fences.

Revalidate the batch inside the same writer transaction, scoped to book and
batch, and retain a named discard/link interleaving regression. Review the other
batch lifecycle transitions for the same separation of admission and mutation;
this finding specifically concerns the newly added split-link writer.

### 3. P3 — a valid long transfer chain is treated as nonconvergence

Location: `backend/internal/db/investment_replay_propagation.go:14–19,45–47`.

Propagation stops after a fixed 256 rounds. The comment says hitting this bound
means inconsistent inputs, but a valid chain longer than the bound needs more
rounds by the algorithm's own stated reasoning.

**Reproduced with a service probe:** buy one share for 10.00; move it through
258 successive holding accounts on 2026-06-01; replace the original acquisition
cost with 20.00, keeping its date and quantity. All transfers post successfully,
but the buy replacement fails with `invalid disposal parameters: internal
transfer basis did not settle`. This is an acyclic, valid history, not a cycle
that fails to settle. The synthetic depth is uncommon, so this is lower priority
than the cache defect; rollback protects the book.

Use a bound derived from the actual dated dependency graph, or process transfer
events in causal order. If a resource limit remains, give it an explicit product
contract and an accurate refusal. Add a boundary regression. The fixed-point
implementation of #147 is reasonable for the tested chains/cycles, but the
claim that it always behaves like the merged dated stream needs this qualification.

### 4. P3 — the R16 completion criterion references a deleted list

Location: `docs/roadmap.md:146`.

“R16 is complete when focus items 1–8 above are closed” survived the planning
cleanup, but the current focus now has only two numbered items and a separate
“placed, not sequenced” paragraph. This makes the initiative's completion bar
ambiguous: it no longer identifies the set whose closure is required.

Replace this with explicit remaining acceptance boundaries or issue references,
including a clear treatment of evidence-blocked provider mapping, zero-delta
splits and the pre-release lot-opening constraint. Keep the reduced single-source
planning structure; restoring duplicated TODO lists would reintroduce drift.

### 5. P3 — TypeScript support policy disagrees with the upgraded workspace

Locations: `docs/developer-workflow.md:264`, `frontend/package.json`,
`pnpm-lock.yaml` (`openapi-typescript@7.13.0(typescript@6.0.3)`).

The workspace now runs TypeScript 6.0.3. Developer guidance still mandates 5.9
while the generator declares a `^5.x` TypeScript peer; the installed
openapi-typescript 7.13.0 still declares that peer. Generation and frontend
checks passed here, which is useful compatibility evidence but does not change
the published peer range.

Document the deliberate tested exception, select a supported generator version,
or restore the documented version policy. Keep the generated-output and build
checks. Do not close #112 just because this upgrade happened: its body specifically
tracks TypeScript 7 compatibility, which this change does not establish. Clarify
that distinction in the guidance and issue when its scope is next updated.

### 6. P3 — several issue closures lack a final resolution record

[#138](https://github.com/sergeyfarin/rekenraam/issues/138),
[#132](https://github.com/sergeyfarin/rekenraam/issues/132) and
[#151](https://github.com/sergeyfarin/rekenraam/issues/151) are closed as completed,
have zero comments, and retain unchecked acceptance lists without a resolution
section. Their implementation and tests exist locally, so this is an audit-trail
gap rather than a recommendation to reopen them automatically.

[#144](https://github.com/sergeyfarin/rekenraam/issues/144) has progress notes,
but its last note says native split reversal/replacement is still open. The
subsequent `46731387` implementation and its tests support completion; the issue
never received the final disposition. Add concise commit/test references and
explain remaining exclusions when maintaining these records. Several other
issues also retain unchecked boxes, but their resolution comments supply the
missing evidence. Checkboxes alone are not the assessment.

## Decisions and remaining risks

- **Shared writer and previews:** gain snapshots precede journals/effects;
  reconciliation considers every appended posting under the command audit;
  gain acknowledgement is checked inside commit; import source acceptance runs
  last. These boundaries prevent partial financial/source acceptance. Cash-neutral
  corrections are correctly netted separately at each checkpoint's date and
  sequence rather than guarding each inverse journal in isolation.
- **Effective reads:** SQL views centralize effective operations and latest
  revisions while self-check still audits original/superseded evidence. Keeping
  revision history after reversal is correct. The merged lot opening fact and
  declared baseline redesign follow accepted ADR 0013's pre-installation exception;
  this is not a forward upgrade path for an already-used older development book.
- **Cross-position propagation:** the dated causal explanation and tests for
  chains, cycles, specific elections and late rollback support the approach.
  Pooled-lot lineage keeps destination identity stable; `source_lots` preserves
  explicit lineage and intentionally refuses incompatible histories. The
  conservative pooled original date is an operational policy, not tax-exact
  acquisition history. R18 correctly retains jurisdiction-specific projections.
- **Verifier independence:** replay equivalence uses the writers' replay engine.
  It can detect stale persistence, but a calculation bug shared by both paths
  can still agree with itself. Keep the independent quantity/basis conservation,
  journal-link checks and explicit expected-value scenarios; a passing replay
  verifier alone is not proof that a newly added operation's arithmetic is right.
- **Performance:** #140 retains the race detector, tests and package timeout
  while parallelizing isolated fixtures, with measured improvements. Separately,
  gain snapshots still read all effective book disposals before/after each command,
  and a fixed-point replay may revisit whole position histories. The 40-position,
  2,000-trade verifier measurement is useful but does not establish bounds for
  correction/preview latency or large transfer graphs. The broader export/self-check
  benchmark remains open as #108; extend workload evidence before release claims.
- **Provider evidence:** holding unsupported split/cancellation fills in review
  is the right decision. #145 and #136 are evidence-blocked; guessing a split ratio
  or interpreting cancelled order status as execution cancellation would be unsafe.
  #152 is an actual open product gap: Refresh cannot reach older revised fills,
  including the buy revision whose later gain would change. It is already tracked
  with a browser acceptance case and is not a newly discovered finding here.
- **Other accepted gaps:** #146 tracks journal-free/zero-delta splits; #148 tracks
  mandatory lot opening provenance before v0.1.0; #100/#101/#102 retain locale-aware
  entry, owner-local dates and catalog review. New write-off entry remains API-only
  even though write-off correction now has UI. Pricing management (#116), quotes
  and crypto (#117), reproducible projections (#118) and returns (#119) remain
  later roadmap work. No claim of complete R16, tax reporting, or provider coverage
  is warranted.

## Issue-closure audit

All 18 closures below are after the base commit timestamp. Commit references
identify local implementation; comments were read where present.

| Issue | Implementation | Assessment |
|---|---|---|
| [#129](https://github.com/sergeyfarin/rekenraam/issues/129) shared gain disclosure | `e4888938`, `b588f061` | Appropriate bounded mechanism/manual-buy pilot; broader rollout explicitly separated. |
| [#141](https://github.com/sergeyfarin/rekenraam/issues/141) rollout | `41a45708` | Protection implemented; API/browser evidence for imports/source revisions initially deferred to #143. Closure was qualified, rather than literal satisfaction of every original evidence checkbox. |
| [#137](https://github.com/sergeyfarin/rekenraam/issues/137) splits | `dd5f33f2` | Manual entry and provider review/linking delivered, with finding 2's linking race; #144/#145/#146 and cash-in-lieu scope explicitly retained. Do not read this as automatic provider split mapping. |
| [#138](https://github.com/sergeyfarin/rekenraam/issues/138) pooled transfers | `60a85c55` | Exact pooling/conservation tests support completion; missing resolution note. Later lineage refinement is #150. |
| [#132](https://github.com/sergeyfarin/rekenraam/issues/132) backdating | `75635707` | Inbound and disposal replay tests cover four methods, ordering and refusal; missing resolution note. |
| [#139](https://github.com/sergeyfarin/rekenraam/issues/139) replay decision/readers | `c4b258f5` | Good item-by-item disposition; explicitly delegates propagation, provenance constraint and verifier to #147/#148/#149. |
| [#147](https://github.com/sergeyfarin/rekenraam/issues/147) propagation | `d418ea37` | Fixed-point substitution for the requested merged pass recorded in the ADR and closure; valid basic evidence, with finding 3's limit. Changed pooled lineage delegated to #150. |
| [#144](https://github.com/sergeyfarin/rekenraam/issues/144) split correction/adjustment | `b904c26a`, `46731387` | Reversal/replacement, adjustment inversion and export tests support closure; last progress note remains stale. |
| [#151](https://github.com/sergeyfarin/rekenraam/issues/151) journal labels | `46731387` | Shared localized title helper, backend label and holding-register browser case exist; missing resolution note. |
| [#143](https://github.com/sergeyfarin/rekenraam/issues/143) import evidence | `dae255c3`, `864ac5a9` | Dev-only provider stub, API cases and mobile browser cases delivered. Buy-source browser case explicitly remains in #152; closure contains an acceptance exception, not evidence that every case passes. |
| [#130](https://github.com/sergeyfarin/rekenraam/issues/130) dividend corrections | `99be4f05`, `125ded00` | Backend/API/mobile correction delivered; finding 1 is a remaining UI defect. Date/account/currency restrictions explicitly retained. |
| [#131](https://github.com/sergeyfarin/rekenraam/issues/131) trade field correction | `b2cd2a2d` | Detailed allowed-field/identity/refusal contract and named tests support closure. |
| [#133](https://github.com/sergeyfarin/rekenraam/issues/133) write-off correction | `044723e9` | Correction/reversal delivered; closure explicitly distinguishes API-only new entry. |
| [#150](https://github.com/sergeyfarin/rekenraam/issues/150) pooled lineage | `da9d4645` | ADR, research, revised date/depletion evidence and tests support the chosen pooled default. Opt-in source-lot restrictions remain intentional. |
| [#149](https://github.com/sergeyfarin/rekenraam/issues/149) replay self-check | `da9d4645` | Stale projection/revision mutation tests and recorded runtime measurement support closure; unmodeled positions remain excluded pending #148. |
| [#140](https://github.com/sergeyfarin/rekenraam/issues/140) race runtime | `f57bb555` | Measured scheduling change with isolated fixtures and no removed assertions; appropriate closure. |
| [#134](https://github.com/sergeyfarin/rekenraam/issues/134) transfer correction | `e63de363`, `ef92ae21`, `44c15cf0` | Detailed staged and final disposition; all currently creatable transfer directions corrected. Outbound commands explicitly wait for #114's writer. |
| [#135](https://github.com/sergeyfarin/rekenraam/issues/135) reconciliation/register | `b18743b3`, `551555e6`, `69574296`, `c91d027f` | Per-boundary guard, healthy-book self-check fixes, combined netting and register explanation delivered with service/API/mobile evidence. Closure notes code was local then; referenced commits are now present in the local origin/main snapshot. |

Most closures are well scoped. The exceptions in #141/#143 are transparent and
have successor issues, but future closure notes should explicitly distinguish
“original acceptance complete” from “remaining acceptance transferred.” Keeping
#114 open is correct; closing its bounded children does not complete the umbrella.

## Validation performed for this review

- Frontend generation, Paraglide compilation and svelte-check: passed, zero errors
  and warnings; generated tracked files stayed unchanged.
- Frontend unit suite: 477 tests passed across 33 files.
- Integrated static frontend/build and embedded Go app: built by the browser harness.
- Normal Playwright suite: 86 tests passed, including mobile correction, import,
  reconciliation, register, theme accessibility, navigation and CSP journeys.
- Backend formatting, vet and full race gate: passed (`./scripts/test-backend.sh`,
  exit 0, local networking enabled). API 277.250s, application 543.272s,
  database 123.936s; remaining packages passed or reused valid cached results.
- Additional service/interleaving and browser probes reproduced findings 2/3 and 1 respectively.
  Their assertions exposed the failures described above. Temporary probe
  files were removed from the repository; no application code was changed.

Sandbox runs initially failed because HTTP tests could not bind local sockets.
The backend and browser suites passed with local networking enabled; those sandbox
errors are environment limitations, not application regressions. GitHub issues
were read only: no issues were closed, reopened, edited or created by this review.
