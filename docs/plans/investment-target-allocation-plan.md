# Target allocation and drift (#411)

Status: **proposed, awaiting owner acceptance; runtime implementation has not
started** (2026-11-06). Governs [#411](https://github.com/sergeyfarin/rekenraam/issues/411).
The owner explicitly requested this independent feature in an isolated worktree
and a pull request. The [roadmap](../roadmap.md) remains the source of overall
work order; this authorization does not reorder the other agent's work.

This extends the [accepted actual-allocation contract](investment-returns-plan.md#allocation-at-the-closing-day-slice-4).
ADRs [0012](../adrs/0012-investment-ledger-subledger-and-reporting-boundaries.md)
and [0017](../adrs/0017-provider-independent-features-and-incomplete-data.md)
continue to govern valuation, incomplete data and the read-only ledger boundary.

## Proposed owner decisions

### Portfolio and grouping

- One current target set per book, portfolio selection and grouping dimension.
  Choose one of the existing dimensions: account, instrument type, or quote
  currency. These are independent complete targets; they are never combined.
- Instrument type means the existing stable types (stock, ETF, fund, bond,
  option, future, other, crypto), plus cash. It does not look through ETFs or
  funds, infer geography/sectors, or provide custom asset-class buckets.
- Portfolio selection follows returns: either the default investment scope or
  explicit selected account subtrees. Sort and deduplicate explicit roots and
  remove a root already covered by another root. Parent and child selections
  cannot double-count an exposure. Default and explicit scopes remain distinct.
- Identity uses the selection mode and canonical root account IDs, not the
  reporting currency, valuation date, display name or current descendant list.
  A new eligible account joins the default scope; a new descendant joins an
  explicitly selected subtree. This does not silently create a new target set.
- Actual account groups remain the outermost in-scope ancestor. Instrument
  classification and quote currency are those effective on the closing day,
  exactly as in the existing allocation. Group keys are stable account IDs,
  type codes or currency commodity IDs, never translated names.

### Exact targets, cash and shorts

- Targets are long-only, from 0% through 100% each, with at most four decimal
  places in percentage units. Store integer units: 1 unit = 0.0001 percentage
  points, and every saved set sums to exactly 1,000,000 units (100%). Reject
  negatives, excess precision, duplicate groups, mixed dimensions and totals
  other than 100%; never round input into acceptance. Zero targets are allowed.
- Forms parse and prefill percentages in the active locale with the shared
  exact decimal helpers. API values are integers within this small exact
  range; no monetary coefficient crosses JSON as a number.
- Cash is included in the same denominator as securities. In the type view it
  is cash; in the currency view it is its own currency. No implicit cash
  exclusion, residual target, or automatic normalization is performed.
- Keep actual long and short rows separate, including negative cash. Targets
  apply only to the long row; short rows have an explicit zero target. Actual
  long weights may exceed 100% and actual short weights are negative. This
  version cannot express a desired leveraged or short target portfolio.
- A classified actual group omitted from the target set has an explicit zero
  target. A targeted group with no actual balance is a known zero only when
  the complete portfolio is valued. Such rows remain visible.
- Unclassified instrument types and absent quote currencies remain separate
  unknown groups. They cannot be target keys. Their known market values stay
  visible; their drift is unavailable with an unknown-group explanation.
  Other classified groups may still show drift when the denominator is complete.

### Comparison and unavailable figures

- The portfolio/date/reporting-currency/staleness selection is the actual
  allocation's own. Reuse its same consistent database image, posted balances,
  trade-date recognition, income receivables, rate table and classification.
  There is no second pricing engine or separate denominator policy.
- For complete prices and a positive closing portfolio value, actual share is
  the exact group value divided by that exact closing value. Signed drift is
  actual share minus target share, displayed in percentage points; positive
  means over target and negative means under target.
- Calculate from exact rational values, then round once to six decimal places
  in ratio units, half away from zero, matching the existing weight precision.
  Do not subtract separately rounded actual and target displays. Rounded row
  displays need not add to exactly 100% or zero drift.
- If any required price or FX leg is missing, show the existing known-only
  actual breakdown and unknown units, but all target-comparison actual shares
  and drifts are unavailable. Target percentages remain visible. Never compare
  a 100% target portfolio against a subtotal of only its priced holdings.
- Zero or negative closing totals preserve values and targets but have no
  comparison shares or drift. An unclassified short makes the comparison
  unavailable under the existing allocation reason. Missing classification
  never becomes a guessed type, currency or known-zero holding.
- Valuation age notices follow the existing actual read. The comparison
  echoes its date, currency, selected/resolved scope, coverage and target
  revision so the UI cannot mix snapshots.

### Persistence, lifecycle and history

- Persist settings beside the ledger with book identity, stable group keys,
  creator, UTC time and immutable revisions. Create/change is an explicit
  audited command; use an expected revision to refuse stale concurrent edits
  atomically. Refetch never silently replaces unsaved form fields or their
  revision token. An empty working copy does not erase saved targets.
- Editing creates a new complete revision. This first version has one current
  revision and retains previous revisions for audit; it has no effective-date
  schedule or historical target selection. A historical actual date is explicitly
  compared against today's current targets, never called the targets in force
  on that historical date. No automatic historical target backfill occurs.
- Renaming an account or instrument does not change target identity. Archived
  or closed accounts and inactive instruments retain their references; they
  are not silently removed or redistributed. A previously saved group that
  no longer resolves stays visible with its ID and an explanation. New targets
  validate against eligible current group identities, including retained
  historical accounts/currencies supported by the existing returns selection.
- A grouping change opens that grouping's independent saved set or a new form;
  it never converts or overwrites another dimension. A changed portfolio
  selection opens its own set. Warn before discarding an unsaved form.
- Native SQLite backup/restore preserves all target revisions and audit rows.
  Under the existing portable-ledger export policy (ADR 0011), target settings
  are omitted from ledger CSV/QIF/bundle; add that omission explicitly to the
  bundle manifest/documentation and verify it. Backup/restore remains the full
  installation round trip. No bundle importer is introduced.

### UI and immutable runs

- Add manual target create/edit/reopen and actual/target/signed-drift rows to
  the live returns allocation workflow, with the grouping and scope explicit.
  Show exact total and remaining/excess percentage while editing, validate on
  both client and server, and preserve input after validation/network failures.
- Use one backend-composed read for the shareable comparison component, with
  no request per row. The comparison obtains targets and actuals from one
  input image; it does not join separately fetched revisions and values in
  browser code. Reads never write financial or application settings.
- Use translated text, semantic theme tokens, labelled controls, visible signs
  and words, keyboard focus management, mobile layout, and loading, no-target,
  error, unavailable and success states. Support the maintained en/nl/ru
  catalogs and explicit fallback for partial es/fr/de catalogs.
- Existing returns calculations, request fingerprints, saved output bytes,
  immutable-run integrity checks and reopening behavior remain unchanged.
  The live comparison is a separate read model and is not included in saved
  returns runs. A saved-run view never overlays today's targets on its
  immutable allocation. The live UI explains that saving a returns run does
  not save targets or drift; target revision history remains separate.
- No rebalancing advice, trade recommendations, orders, ledger postings,
  cost-basis changes or tax optimization. Advisory rebalancing stays in #412.

## Independent worked cases

All values below are exact EUR values at the chosen closing date. Percentage
point drift is actual percentage minus target percentage.

| Case | Facts and targets | Expected comparison |
|---|---|---|
| T1 complete/cash | Stocks 600, bonds 300, cash 100; targets 50%, 40%, 10% | Total 1,000; actual 60%, 30%, 10%; drift +10, -10, 0 pp |
| T2 missing group | Stocks 900, cash 100, no bonds; targets 60%, 30%, 10% | Bonds appear as actual 0%, drift -30 pp; stocks +30 pp; cash 0 pp |
| T3 missing price | Known cash 100, 10 unpriced stock units; targets stock 90%, cash 10% | Existing allocation says cash 100% of known-only 100; comparison actual shares/drifts all unavailable, targets 90%/10% stay visible |
| T4 short cash | Stocks +1,100, cash -100; targets stock-long 90%, cash-long 10% | Total 1,000; stock-long actual 110%, drift +20 pp; cash-long actual 0%, drift -10 pp; cash-short actual -10%, target 0%, drift -10 pp |
| T5 non-positive | Stocks +100, cash -100 (then cash -150) | Total 0 (then -50); values and targets stay, comparison shares/drifts unavailable |
| T6 unknown class | Classified stock 600, priced unclassified security 400; stock target 100% | Total 1,000; stock actual 60%, drift -40 pp; unclassified value 400/actual 40% visible, its target/drift unavailable; nothing assigned to stock |
| T7 exact input | 33.3333%, 33.3333%, 33.3334% | Units 333333 + 333333 + 333334 = 1000000 accepted; three 33.3333% values rejected (99.9999%); 33.33333% rejected for precision |
| T8 rounding | Group A 1, B 1,999,999; targets 0.0001% and 99.9999% | Total 2,000,000; A exact actual ratio 0.0000005 rounds to 0.000001; exact drift 0.0000005 - 0.000001 = -0.0000005 rounds to -0.000001 (-0.0001 pp), not zero from subtracting rounded shares |
| T9 portfolio identity | Selected parent P and descendant H, then P alone | One canonical root P and one target set; all actual balances counted once |
| T10 currency | USD stock 100 at EUR 0.9/USD, EUR cash 10; targets USD 80%, EUR 20% | Total EUR 100; USD actual 90%, drift +10 pp; EUR actual 10%, drift -10 pp; missing USD/EUR makes drift unavailable |
| T11 lifecycle | Account renamed, then closed with nonzero historical exposure | Same target ID/percentage retained; name/status updates; historical dated actual rules unchanged; no redistribution |
| T12 current vs historical | Revision 1 stock 60%/cash 40%, then revision 2 70%/30%; historical actual 65%/35% | Comparison labels current revision 2, drift -5/+5 pp; revision 1 retained; existing saved returns bytes unchanged |

## Planned validation

Named service/API tests own the literal cases above, validation of duplicate
keys and stale writes, no read-side writes, and targets/actuals captured from
one input image. Existing allocation and saved-run compatibility/integrity
regressions must remain green. Exercise target backup/restore and the explicit
export omission. A phone browser journey creates, changes and reopens targets,
checks exact locale input and signed drift, partial-price and empty states,
stale edit preservation, keyboard/accessibility and both themes. Run the
relevant shared backend/frontend gates and integrated build before the PR.

## Acceptance record

Pending. Record the owner's answer and any amended choices here before runtime
coding. Issue #411 explicitly requires owner acceptance of this contract and
its independent examples before implementation.
