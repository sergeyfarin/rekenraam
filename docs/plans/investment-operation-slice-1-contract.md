# Investment operation slice 1: economics and compatibility contract

Status: accepted design contract, 2026-09-26. This slice specifies the next
schema/writer work; it does not enable new transaction types or change the
current journal. ADR 0012 and ADR 0013 govern. The delivery order and later
operation families remain in [the refactor plan](investment-operation-refactor-plan.md).

## Scope and signs

The current writer is the compatibility reference for buy, sell, cash
dividend, reinvested dividend, and zero-proceeds write-off. Each journal
balances **per commodity**. Positive postings are debits; negative postings
are credits. Security quantity `q` is positive in the examples. Cash facts
`G`, `C`, and `N` use the owner's sign: receipts positive and payments
negative. `T` means the `commodity_trading` system account, `H` the security
holding, `A` the actual cash account, `I` dividend income, and `X` an
expense/income or withholding account chosen for its role. A zero cell means
there is **no posting**, not a posted zero.

Source facts and operational calculations are different records. The broker's
gross, individual charges, net cash, currency and dates are immutable source
facts. Cost basis, disposal proceeds and allocations are elected or derived
operational results with snapshot provenance. `gross_unknown` is explicit
for today's one-cash-amount trades; do not recover a fictional gross or fee
from that amount.
This separation matches the distinct trade price, gross amount, commission,
and net amount fields in [IBKR's feed specification](https://www.interactivebrokers.com/campus/ibkr-reporting/employeetrack/).

## Current commands and future posting matrices

| Operation | Security postings | Cash-currency postings | Lot/result |
| --- | --- | --- | --- |
| Buy | `H +q`, `T −q` | `A −B`, `T +B` | Open long lot at basis `B` |
| Sell | `H −q`, `T +q` | `A +P`, `T −P` | Consume long lots; gain `P − disposed basis` |
| Cash dividend | None | `A +D`, `I −D`; if withheld, `X +W`, `A −W` | No lot change; cash received `D − W` |
| Reinvested dividend | `H +q`, `T −q` | `T +D`, `I −D` | Open long lot at basis `D`; no cash movement |
| Write-off | `H −q`, `T +q` | None | Consume long lots at zero proceeds; loss `−disposed basis` |

`B`, `P`, `D`, and `W` are positive amounts in one currency. These rows
describe the code as it stands: the buy stores `CashAmountValue` as lot cost;
the sell's cash postings supply proceeds; dividend withholding uses the cash
commodity and can post to an asset, liability, or expense account; and the
reinvestment command has no withholding component. A later withholding or
fee on reinvestment needs an explicit extension of the operation, not a
silent change to `D`. Write-offs have no cash legs. The security clearing
leg alone balances their security commodity, while the earlier acquisition
cost remains in currency clearing and corresponds to the loss on closure.

For exact trade economics, let `G_c` be signed gross consideration in
currency `c`; each signed charge `C_i,c` is negative for a fee/tax and
positive for a rebate; and `N_c = G_c + Σ C_i,c` is the sourced net cash in
that currency. A trade with one primary cash commodity has one gross amount;
separate fee currencies have their own net cash components. Validate the
identity independently in every currency with exact scaled arithmetic.
Unknown gross permits only a sourced net component, with no invented charge
breakdown. The account posting for each `N_c` is `A_c +N_c`.

For same-currency charges, split known charges by their immutable treatment:
`C_clear` goes through clearing, and `C_separate` goes to a named
expense/income account. Set `K = G + Σ C_clear`, so `N = K + Σ C_separate`.
The cash-currency journal is:

| Account | Posting | Meaning |
| --- | --- | --- |
| Settlement cash `A` | `+N` | Actual cash movement |
| `commodity_trading` | `−K` | Operational opening cost or disposal proceeds |
| Each charge account `X_i` | `−C_i` for `C_i ∈ C_separate` | Expense debit for a fee; credit for a rebate |

These postings sum to zero exactly. For an ordinary buy, `K < 0` and opening
basis is `−K`. A buy whose rebates would make basis negative is rejected
for named review; no negative long-lot basis is stored. For a sale,
operational proceeds are signed `K`: charges can exceed a small gross sale
and produce a net payment, which must remain a loss rather than be clamped
to zero. A charge recorded in `X_i` is excluded from basis/proceeds and
reported separately; it must not also enter `K`. A charge can be part of
the one net settlement
posting rather than having its own journal line; its value's **clearing
treatment** is what matters. A zero-proceeds write-off uses `K = 0` and no
cash-currency posting.

For example, buy gross `−100.00` with commission `−2.00`, then sell gross
`+120.00` with commission `−2.00`, each for the full position:

| Treatment | Buy currency postings (`A`, `T`, `X`) | Buy basis | Sell currency postings (`A`, `T`, `X`) | Sale proceeds | Closed `T` balance | Trading gain; separate expense |
| --- | --- | ---: | --- | ---: | ---: | --- |
| Clearing included | `−102, +102, 0` | 102 | `+118, −118, 0` | 118 | `−16` | `+16; 0` |
| Separately expensed | `−102, +100, +2` | 100 | `+118, −120, +2` | 120 | `−20` | `+20; 4` |

Both give an after-expense result of `+16`. The debit-positive clearing
balance is the negative of the realized trading gain on a fully closed
position **only** when the basis and proceeds components included there
are complete in the same currency. ADR 0012's unresolved-basis and FX
exceptions remain in force.

## Charge policy and missing information

Each known charge component snapshots `clearing_included` or
`separately_expensed`, its charge kind, selected account if expensed,
resolution tier, policy ID/version, effective date, and source evidence.
Resolution order is `transaction → account → global → fallback`, using the
same tier names as disposal decisions. Account and global defaults are
versioned and dated; later edits never reinterpret a committed component.
An import uses exactly the same resolver as manual entry. A correction can
explicitly replace an election and its postings; an unchanged election
remains pinned even if defaults have since changed.

| Component | Fallback without an account/global/transaction choice | First-slice behavior |
| --- | --- | --- |
| Ordinary same-currency trade commission | `clearing_included` | Preserves today's commission-inclusive net-cash cost/proceeds |
| Explicit transaction tax, other fee, rebate | None | Manual command returns a named missing-treatment error; import row stays in review |
| Dividend withholding | Not a trade-charge election | Use the specified withholding account and actual cash currency |
| Fee in another currency | `separately_expensed` only when an explicit account is mapped | Otherwise named manual error/import review; never silently convert into trade basis |

The fee-currency default is intentionally narrow. For a foreign-currency
fee `C_f`, post actual cash `A_f +C_f` and the mapped fee account
`X_f −C_f` in that currency. The primary trade's `G_t` still balances
through `A_t +G_t` and `T_t −G_t`. Do not sum `C_f` into `N_t` or convert it
with an inferred rate. A future capitalized foreign-currency charge needs
an explicit value bridge, named FX source/rate/date, and a complete
per-currency posting and reconciliation specification before acceptance.
Until then the manual command rejects that election and an import stays in
review. FX conversion for display is a reporting matter; it does not alter
the recorded trade or its cost currency.

For example, buying for `−100.00` EUR with a `−2.00` USD fee separately
expensed posts EUR cash `−100.00` and EUR trading `+100.00`, plus USD cash
`−2.00` and USD expense `+2.00`. The two commodities balance independently;
the opening lot basis is `100.00` EUR. No unobserved EUR/USD conversion is
inserted into the journal.

## Exact allocation and date rules

- Store money/quantity as canonical decimal TEXT coefficient and explicit
  scale; use exact scaled arithmetic. No floating point and no SQLite `SUM`
  over coefficient columns. The current service's int64 admission ceiling
  remains until deliberately widened, while the next schema uses TEXT.
- Source trade date, settlement date, and charge payment dates separately.
  Post each actual cash movement on its financial date; do not date cash to
  a trade, record, or ex date merely to make a lot effect line up.
- A long disposal records one gross/net proceeds decision per position, side,
  and cost currency. Allocate proceeds to consumed lots in their exact
  quantity ratio at a position-wide scale no shallower than any recorded
  amount and no deeper than the cost commodity's as-of-date maximum scale.
  The current int64 service may choose a shallower scale within that range
  when a deeper coefficient would exceed its admission limit; otherwise it
  returns a named range error. Never choose a scale from the user's text.
  Truncate each non-final
  allocation toward zero and assign the exact remaining amount to the final
  allocation in stable lot order. Snapshot both decision and allocations.
  The sum must equal the decision's proceeds, including after mixed-scale
  input and a partial lot disposal. The existing cost-basis allocation uses
  the same position-wide scale and retains a partial-disposal remainder in
  the surviving lot; a final disposal consumes that remainder.
- If a multi-currency position has lots in different cost commodities,
  require an explicit cost-currency selection or separately named decisions.
  Do not combine their coefficients or infer FX from sale cash.
- If sale cash and selected lot cost use different currencies, keep the
  balanced source cash journal but mark operational proceeds/gain unresolved
  until a sourced FX conversion rule and value bridge are specified. No
  report may subtract one currency's basis coefficient from another's
  proceeds coefficient.

Worked mixed-scale case: buy `1.250` shares for gross `−100.005` EUR and a
clearing-included `−0.015` EUR commission, so net cash is `−100.020` and
opening basis is `100.020`. Sell `0.500` shares for gross `+60.001` EUR
with a `−0.011` EUR clearing-included commission, so net cash/proceeds are
`59.990`. At a six-place EUR allocation scale, `0.500 / 1.250` consumes
`40.008000` of basis; realized gain is `19.982000`, and the remaining
`0.750` shares retain `60.012000`. A spelling such as `100.02` for the
same opening amount must produce the same allocation. A non-terminating
ratio case must conserve its exact remainder as specified above.

## Migration and export gate for slice 2a

Slice 1 does not edit `0001_initial_schema.sql`. The fresh and seeded
candidate-database test creates the current HEAD schema twice, loads the
existing frozen v0.1 seed into one, and exports both. It pins the existence
and relationships of the investment files and verifies exact seeded basis,
allocation, and decision provenance. Empty files still have headers. Slice
2a updates that test alongside the declared baseline rewrite, checksum,
fresh/seeded fixtures, and bundle schema version. New operation, component,
date, link, lot-fact, projection/revision and price-provenance data need
lossless export rows; a projection-only `lots.csv` cannot replace immutable
facts. Restore and self-check must read the same effective revision. No
candidate rewrite is accepted merely because a fresh database boots: the
seeded export and ledger state must survive the change.

The reference export remains one full-book CSV bundle with the balanced
`ledger.csv`, `lots.csv`, `investment-operations.csv`,
`disposal-decisions.csv`, `disposal-allocations.csv`, `prices.csv`, and
manifest checksums. Investment QIF round-tripping is outside this slice.
