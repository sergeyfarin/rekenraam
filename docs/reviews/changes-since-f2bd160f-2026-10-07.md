# Review of changes since f2bd160f (2026-10-07)

## Scope and conclusion

Reviewed `f2bd160fa35aeeeaab16fe2a77526c7805e39e0a..10c6bdc0138dd069a90e785192b3269e17945a1b`:
14 commits, 113 changed files. The base is excluded. GitHub was checked against
the base timestamp, 2026-10-04 20:48:57 UTC: 10 issue closures and 13 newly
created issues fall after it. There are 29 open issues at this review snapshot,
including the roadmap index.

**Work remains in progress.** R16 is current. Cash-in-lieu HTTP correction and
entry, unknown basis, short positions, compound actions and other stated
follow-ups remain open. Their absence is not itself a defect in a bounded
known-basis slice.

The earlier cache, discarded-batch and transfer-depth findings have substantive
fixes and named regressions. The new outbound bridge/replay/correction design
continues the shared transaction, immutable evidence and acknowledgement model.
However, the new return-of-capital family has an input-precision defect, an
inadequate correction workaround, and a historical-evidence diagnostic gap.
These were reproduced with three separate service probes. Its completion claim
needs correction even though the normal browser and frontend suites pass.

## Findings requiring action

### 1. P1 — return-of-capital allocation depends on how the amount is written

Location: `backend/internal/db/investment_capital_return.go:123–149`.

The allocator divides the receipt's raw coefficient and truncates at
`amount.Scale()`. That makes source formatting choose financial precision.
The frontend preserves the entered scale through `parseMoneyMagnitude` in
`frontend/src/lib/investments/capital-return-form.svelte:105–114`; it does not
pad an integer entry to a currency scale.

**Reproduced:** buy two equal one-share lots, each with 10.00 basis, then record
an economically identical 1.00 return of capital:

| Input coefficient / scale | First lot reduction | Second lot reduction |
|---|---:|---:|
| `1` / `0` (entry `1`) | 0.00 | 1.00 |
| `100` / `2` (entry `1.00`) | 0.50 | 0.50 |

Both histories pass the investment foundation, lot-reconciliation and
replay-equivalence checks. A later FIFO sale was also exercised: its disposal
basis is 10.00 for the integer entry and 9.50 for the decimal entry. A FIFO or
specific-lot disposal therefore reports different basis/gain solely because
the receipt was typed differently.
Per-lot basis caps can also make the unresolved excess depend on that spelling.
The cash receipt itself still balances and is the same economic amount.

Use a documented allocation precision derived from the cost currency, with
one deterministic remainder rule and range handling, rather than the request's
scale. The existing disposal allocation machinery and T-103's input-independent
basis contract are relevant precedents. Add equivalent-amount cases at scales
0, 2 and higher, under preview, commit, replay, later disposal and basis caps.
Include a browser case with an integer amount, not only `10.00` on one lot.

The fix needs a disposition for already-recorded effects: preserve originals
and audit any corrected projections. A passing verifier using the same wrong
algorithm is not an independent arithmetic oracle. Address this known-basis
bug separately from #160's unknown-basis expansion.

### 2. P2 — reverse-then-record is not an equivalent return-of-capital correction

Locations: `backend/internal/app/investment_capital_return.go:223–256`,
`frontend/src/lib/transactions/investment-correction-section.svelte:320–333`.

Only reversal is exposed from transaction detail; there is no native replacement
command or replacement action. Issue #163's progress comment explicitly calls
replacement “reverse-then-record”. That preserves two separate cash changes,
but the new record gets a new operation slot rather than the original
correction root's same-day slot.

**Reproduced through the offered service workflow:**

1. Buy two shares for 20.00.
2. Record a 4.00 return of capital on June 1.
3. Sell one share later on June 1: disposal basis is 8.00.
4. Correct the receipt to 6.00 by reversing it, accepting the changed gains,
   and recording a new return with the same effective/payment dates.

The existing sale now has basis **10.00**, and the remaining share has basis
**4.00**: the new return happened after the sale. Correcting the original return
in its original slot would give disposal basis **7.00** and remaining basis
**7.00**. Self-check passes the reverse-then-record history because it accurately
represents a different event ordering. This is a limitation of the claimed
correction workflow, not a failure of replay to reproduce its inputs.

Implement replacement through the shared audited correction writer, preserving
the original root slot and applying receipt/basis changes atomically with gain
and checkpoint review. Keep a same-day dependent-sale regression. Until then,
reopen #163 or track this remaining acceptance explicitly and qualify the
“return-of-capital completion” claim in the roadmap. A manual reversal/new
entry should not be presented as an equivalent correction in this case.

### 3. P2 — self-check skips original and superseded return-of-capital amounts

Location: `backend/internal/db/self_check.go:1301–1320`.

`SelfCheckCapitalReturns` reads original effects only when the operation has
no revision, otherwise only the latest revision. Once replay revises a return,
its original allocation/reduction/excess conservation is no longer audited by
this check; intermediate revision amounts are also omitted. Effective selection
is correct for projection reconciliation, but insufficient for an evidence audit.

**Reproduced using the repo's usual mutation-test technique:** record a 10.00
return against a 7.00-basis lot, then correct the acquisition to 5.00 so a return
revision exists. After confirming the healthy book passes, remove the test
fixture's immutability trigger and change the original allocation to **11.00**,
leaving its 7.00 reduction and 3.00 excess. Investment foundation and replay
equivalence both still report `passed`. The archived original evidence no longer
conserves its own 10.00 receipt, despite the current projection remaining sound.
This is a diagnostic gap; the normal application does not offer that mutation.

Audit every original and revision effect set independently against its receipt
and per-effect conservation, while using the latest effective set separately
for current lot reconciliation. Include original and intermediate-revision
mutation cases. Retain the working stale-current-revision test. This repeats
the superseded-disposal-evidence bug class already recorded in the validation
skill and should receive the same treatment for the new basis-action family.

## Verification of the previous findings

| Previous finding | Current evidence | Disposition |
|---|---|---|
| Gains/positions cache | `invalidateInvestmentReads` is used by all investment forms, reversal dispatch, import commit/source correction and suggestion acceptance; browser preloads Gains and navigates through the correction. | #153 closure justified. The original UI reproduction is now a passing regression. |
| Split link after discard | Writer re-reads parent batch/source/status inside its transaction. The wider sweep also fences staged-row outcome writes and conditional batch transitions. | #154 closure justified; named preparation/discard and mid-import discard tests exist. |
| 256-round propagation limit | Downstream closure is replayed as one merged dated stream using shared reset/apply/finish steps; a 260-hop two-account regression exercises the former failure. | #155 closure justified; removes the limit and repeated whole-position walk. |
| R16/TypeScript/closure documentation | Roadmap names its completion issues; developer guidance explains the tested TS6 exception; #112 still tracks TS7. Resolution comments now exist on #138/#132/#151/#144. | #156 closure justified. All four historical comments were read. |

The richer shared cache helper and the expanded discard sweep are useful fixes
beyond the narrow initial reproductions. A single causal replay pass is a better
match for the ADR than the earlier fixed-point rounds. Preserve the cycle,
pooled original-date, election and late-refusal cases when adding new intents.

## Issue-closure audit

GitHub timeline events were read for all 10 closures. Each names an implementation
commit in this range; these are not merely unexplained manually closed issues.
An unchecked acceptance list or zero comments is not by itself proof of a bad
closure when the closing commit supplies the implementation/test record.

| Issue | Actual closing commit | Assessment |
|---|---|---|
| [#153](https://github.com/sergeyfarin/rekenraam/issues/153) cache | `8cad2875` | Meets its scoped acceptance; shared helper and actual browser regression. |
| [#154](https://github.com/sergeyfarin/rekenraam/issues/154) discard race | `8cad2875` | Meets acceptance; transactional status/source recheck and broader row-write protection. |
| [#155](https://github.com/sergeyfarin/rekenraam/issues/155) deep propagation | `8cad2875` | Meets acceptance; causal pass, depth regression and documented measurements. |
| [#156](https://github.com/sergeyfarin/rekenraam/issues/156) documentation | `8cad2875` | Explicit R16 bar, TS policy and historical resolution notes delivered. |
| [#157](https://github.com/sergeyfarin/rekenraam/issues/157) outbound entry | `6dd876a5` | Six-locale entry, explicit-lot/pool preview, bridge label and phone journey exist. |
| [#158](https://github.com/sergeyfarin/rekenraam/issues/158) outbound replay/bridge | `ad51c0e8` | Link revisions, dated bridge delta, backdating, preview/rollback and self-check evidence support closure. Changed pooled source lineage remains an explicit refusal. |
| [#159](https://github.com/sergeyfarin/rekenraam/issues/159) outbound correction | `e90d3d47` | Net bridge is pinned/rechecked and inverted; source replays; replacement, mobile and rollback cases exist. The specialized replacement route is `replace-transfer-out`, rather than the ticket's generic route spelling. |
| [#161](https://github.com/sergeyfarin/rekenraam/issues/161) return of capital | `b8d72262` | Qualified first-slice closure: replay, entitlement and UI were transferred to #163, unknown basis to #160. Its original body still promises the broader feature. Finding 1 needs a separate correctness fix. |
| [#162](https://github.com/sergeyfarin/rekenraam/issues/162) cash in lieu | `658f9e45` | Qualified API-slice closure: the body includes mobile entry, still open under #165; unknown basis belongs to #160. Disposal/date/split-fence cases support the shipped slice. |
| [#163](https://github.com/sergeyfarin/rekenraam/issues/163) complete return of capital | `0442a11e` | Not complete as written: no native correction from detail, and explicit entitlement is whole-lot IDs rather than the promised lot/quantity set. Partial-lot entitlement is explicitly unsupported in the active contract, but no successor issue tracks that narrowed acceptance. Findings 1–3 affect this family. |

For #161/#162, keep the bounded delivery, but amend their final disposition to
name acceptance moved to the still-open/completing children. For #163, either
complete the missing behavior or explicitly re-scope the issue and track the
excluded work. A progress note describing a workaround does not demonstrate
that the originally promised correction is equivalent; finding 2 disproves it
for same-day history. The last #163 comment also predates its final explicit
entitlement commit and lacks a final scope/validation disposition.

## New issues and current open work

Thirteen issues were created after the base:

- **#153–#156:** prior review findings; now closed by `8cad2875`.
- **#157–#159:** outbound entry, replay/bridge and correction; now closed.
- **#160:** unknown-basis transfer/resolution; still open, deliberately last
  within these #114 actions.
- **#161–#163:** return of capital, cash in lieu and return-of-capital completion;
  closed with the qualifications above.
- **#164:** race-gate timeout headroom; still open. It records an app run of
  825 seconds against the 900-second package timeout and requests measurement
  and restored headroom without dropping assertions. This review's app race
  run passed in 594.108 seconds (about 34% headroom here); one successful run
  does not establish the variance or fulfill its profiling acceptance.
- **#165:** cash-in-lieu correction and entry; correctly remains open.
  `c85aba82` adds service-layer reversal/replacement and preserves the split
  link, but HTTP endpoints, chain flag, entry/correction UI and their browser
  evidence are not yet shipped. Its progress comment states those limits.

The 29 currently open issues include the #114 umbrella, #103 short sale/cover,
#115 compound actions, #146 zero-delta splits, #148 mandatory lot opening
provenance, #152 older source revisions reachable from Refresh, and the existing
locale/date/catalog work #100/#101/#102. Evidence-blocked provider mapping
#145/#136 also remains open. Do not treat those declared limits as newly found
failures in the manual known-basis commands.

## Suggestions

1. Fix input-independent return-of-capital allocation and its regression first.
   Use multi-lot and later-disposal scenarios, not only receipt-total conservation.
2. Restore a truthful completion bar for return-of-capital correction and
   entitlement. Add an atomic replacement preserving the root slot; explicitly
   track partial-lot entitlement if it remains excluded.
3. Extend basis-action self-check to all immutable evidence, independently of
   effective projection checks. The new probes show why running the same replay
   engine twice cannot prove its arithmetic or archived history correct.
4. Continue the current bounded cash-in-lieu work in #165 and known/unknown
   contract in #160 after the above trust work. Keep provider mapping gated by
   actual evidence rather than inferring entitlement from fill names.
5. Follow through on #164 with package/test timing evidence. Preserve race and
   conservation coverage; prefer reducing repeated heavy fixture/self-check work
   over removing financial assertions.
6. Clean up the feature ledger's status labels: new outbound and return-of-capital
   rows have UI but still use 🟡, whose legend says “backend only (no UI)”. Use the
   declared partial/shipped status appropriate to their stated limits. The
   slice-5 plan's opening “remaining commands are unimplemented” sentence also
   predates the commands described later in that same document.

## Validation performed

- Frontend OpenAPI/translation generation and type checking passed, with no
  tracked generated-output changes; 478 unit tests passed.
- Integrated build and normal Playwright suite passed: 89 tests, including the
  earlier cached-gain reproduction and new outbound/return-of-capital phone flows.
- Full backend formatting, vet and race gate: passed on the settled-build
  rerun (`./scripts/test-backend.sh`, exit 0), including embedded-web checks.
  The first run passed the API (269.734s), application (594.108s) and database
  (131.821s) race suites, but its final embedded-web compilation used asset
  filenames discovered before the concurrent browser build replaced them.
  That validation-ordering error was corrected by rerunning the gate after
  the build settled; it is not an application defect.
- Three additional service probes failed their intended financial/diagnostic
  expectations exactly as described in findings 1–3. Healthy fixtures were
  checked before the deliberate historical-evidence mutation. Probe sources and
  logs were retained temporarily under `/tmp`; the repository probe file was removed.
- GitHub issue metadata, comments and closing timeline events were read only.
  No issue was closed, reopened, edited or created during this review.

This report records findings and suggestions; it does not change application
behavior or adopt a new roadmap order.


## Resolution follow-up (2026-10-07)

The three findings above are tracked together in
[T-148 #163](https://github.com/sergeyfarin/rekenraam/issues/163), which was
reopened to complete the return-of-capital family. Commit `6c232a44` fixes all
three and completes the missing fixed-quantity entitlement and native correction
acceptance. The original review snapshot and reproductions remain unchanged.

| Finding | Resolution | Named regression evidence |
|---|---|---|
| 1. Input-spelling-dependent allocation (P1) | Allocate at the position's cost-currency precision with int64 backoff, retaining normalized receipt precision and a deterministic remainder. | `TestCapitalReturnScaleIndependence`, `TestCapitalReturnEntitlementAllocationScaleIncludesUnentitledPositionBasis`, `TestCapitalReturnLargeReceiptRetainsExactNormalizedCoefficient` |
| 2. Reverse-then-record changes the original same-day slot (P2) | Native replacement atomically posts the inverse and corrected receipt and replays the basis action at the correction root's slot, with checkpoint and gain guards. | `TestCapitalReturnReplacementKeepsSameDaySlot`, `TestCapitalReturnReplacementLateFailureRollsBackJournalFactsAndReplay`, `TestCapitalReturnEntitlementCorrectionPreservesUnchangedCashCheckpoint`; mobile closed-position correction in `investments-capital-return.spec.ts` |
| 3. Historical self-check skips original and superseded effects (P2) | Audit each original and each revision set independently, including missing sets; effective selection remains separate for projection checks. | `TestCapitalReturnOriginalDamageAfterRevision`, `TestCapitalReturnSelfCheckAuditsSupersededAndMissingRevisionEffects` |

The completed slice passed the full backend formatting/vet/race gate, frontend
checks with 478 unit tests and no type warnings/errors, and the full 90-case
browser acceptance suite. Both return-of-capital mobile journeys passed again
after the final position-wide precision fix. A further feature-scoped acceptance
run was performed before delivery and issue closure.

The baseline change is explicitly declared `BREAKING DEV DATABASE` in the
commit, ADR 0013 and developer workflow. Unknown-basis handling (#160),
cash-in-lieu completion (#165), and race-gate headroom work (#164) remain open;
this resolution does not claim that R16 or the wider project is complete.
