Items 9–12: Design Suggestions
9 — Session lifetime is hardcoded (30 days)
Add SESSION_LIFETIME_HOURS to config.go (default 720, must be > 0). The AuthService should receive it as a field and pass it to sessionExpiresAt(). This lets operators tighten sessions in higher-security environments without a recompile. Document the default and constraints in conventions.md under the new auth section.

10 — CSRF token doesn't rotate
Acceptable for now. The threat is mitigated by SameSite=Strict + same-origin constraints. True rotation requires either server-side nonce state (doubles session lookups) or a short-lived double-submit cookie separate from the session. Neither is justified until cross-origin requests are possible. Document this as a deliberate design choice in 0002-http-security-policy.md so future contributors don't reopen it without justification.

11 — passwordNeedsRehash silently swallows errors
Thread the *slog.Logger into AuthService and emit a WarnContext log when parsePasswordHash fails inside passwordNeedsRehash. The function must still return false on error (safe-fail — don't rehash what you can't parse), but silently discarding parse errors makes operational debugging hard. The logger is already passed to NewAPIServer; routing it to NewAuthService is a single-line change.

12 — Pre-domain readiness gaps
Three gaps to address before starting domain feature slices:

Money representation limits not yet decided — shopspring/decimal is named in conventions but the wire format (integer + scale + commodity code) hasn't been formalized into a schema convention or an OpenAPI type. An ADR or schema example should lock this down before the first account/transaction migration lands.

OpenAPI code generation command undocumented — openapi.yaml is declared the source of truth, but the command to regenerate frontend types (openapi-fetch + openapi-typescript) is not in developer-workflow.md. Add it so contributors know what to run after schema changes.

Domain lifecycle status taxonomy missing — Account, transaction, and posting records will each need an archived/void/draft/posted status. Defining the allowed states and valid transitions now (as a short ADR or conventions section) prevents incompatible patterns from landing in separate feature slices.






Read [](file:///home/coder/Repos/rekenraam/docs/product-requirements.md#1-1), lines 1 to 80

Read [](file:///home/coder/Repos/rekenraam/docs/product-requirements.md#80-80), lines 80 to 120

Read [](file:///home/coder/Repos/rekenraam/docs/conventions.md#1-1), lines 1 to 80

Read [](file:///home/coder/Repos/rekenraam/docs/early-architecture-decisions.md#1-1), lines 1 to 100

Read [](file:///home/coder/Repos/rekenraam/docs/early-architecture-decisions.md#100-100), lines 100 to 180

Read [](file:///home/coder/Repos/rekenraam/docs/early-architecture-decisions.md#180-180), lines 180 to 249

Read [](file:///home/coder/Repos/rekenraam/docs/phase-1-implementation-plan.md#1-1), lines 1 to 80

Read [](file:///home/coder/Repos/rekenraam/docs/phase-1-implementation-plan.md#80-80), lines 80 to 200

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "transaction\|posting\|journal\|split" backend/migrations/0001_initial_schema.sql | head -30

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "system_role\|system_account\|opening_balance\|import_imbalance\|transfer_clearing\|commodity_trading\|retained_earnings" backend/migrations/0001_initial_schema.sql | head -30

Ran terminal command:  cd /home/coder/Repos/rekenraam && sed -n '265,295p' backend/migrations/0001_initial_schema.sql

Ran terminal command:  cd /home/coder/Repos/rekenraam && sed -n '295,380p' backend/migrations/0001_initial_schema.sql

Now I have a thorough picture of the existing schema and architecture. Let me compile the full review. 

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "account_class\|account_kind\|normal_balance\|debit\|credit\|sign" backend/migrations/0001_initial_schema.sql | head -20

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "quantity_scale\|max_quantity_scale\|standard_scale" backend/migrations/0001_initial_schema.sql | head -15

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "transfer_clearing\|commodity_trading\|unassigned" backend/internal/app/*.go | head -20

Ran terminal command:  cd /home/coder/Repos/rekenraam && grep -n "system_role\|seeded\|opening_balance\|retained_earnings\|import_imbalance" backend/internal/app/accounts.go | head -20

Now I have all the context I need. Here is the full review.

---

## Review: Proposed Transaction / Journal-Entry / Posting Schema

Overall verdict: **the design is well-suited for this codebase and consistent with the existing architecture.** The sign convention, immutability model, multi-journal-entry pattern for delayed transfers, and FX trading account are all correct for double-entry personal finance. Below is a detailed analysis by section.

---

### Core Mental Model — Alignment With Existing Codebase

The five-table model (`transactions → transaction_versions → journal_entries → posting_lines → posting_versions`) fits the existing versioning pattern used for accounts, commodities, and institutions. All of those already follow the `identity table + append-only versions table` approach, so this is consistent.

The `posting_lines` identity table mirrors how `accounts` provides a stable ID for something whose content lives in `account_versions`. This is the right call for UI diff and history views.

One naming note: the codebase currently calls the concept "splits" in both the conventions doc and the implementation plan ("Transactions contain postings or splits"). If you use `posting_lines` and `posting_versions` in the schema, update conventions.md to prefer "posting" as the canonical term — it is clearer and avoids split/posting ambiguity later.

---

### Sign Convention

The proposed convention (`positive = debit, negative = credit`) is consistent with the standard bookkeeping rule and matches the mental model for all 20 examples correctly.

One gap: this needs to be formally written into conventions.md and early-architecture-decisions.md before implementation, because it is a non-reversible design decision. The current architecture doc says "use exact decimal storage" and specifies `quantity_value` as a signed integer but never commits to the debit-positive sign rule. **Lock this before any transaction migration lands.**

The examples are all correct. Spot-checking:
- Credit card purchase: Expenses +65, Liabilities -65. ✓ (liability balance increases because it goes more negative)
- Salary: Income -4200, Assets/Expenses sum to +4200. ✓
- FX exchange: each commodity balances independently. ✓
- Investment buy with commodity trading: USD and VTI each sum to zero. ✓

---

### `transactions` Table

Clean. No mutable financial data on the identity row — this is the right call, consistent with `accounts.id` and `institutions.id`. The set of columns is minimal and correct.

One question: should `transactions` also carry `book_id`? Every other identity table does (`accounts.book_id`, `commodities.book_id`, etc.) and the conventions doc says "Preserve `book_id` in core schema even while runtime stays single-book." Add it to be consistent.

---

### `transaction_versions` Table

**Good overall. Several items to confirm:**

**`transaction_kind`** — the proposed values are `ordinary | transfer | income | investment | adjustment | opening_balance`. Consider:
- `transfer` and `income` and `investment` are redundant with what the posting accounts already express structurally. In pure double-entry, every transaction is "ordinary" and you infer transfer/investment semantics from the accounts. Adding them as a kind creates a second truth. If you add them, add a DB CHECK that enforces consistency (e.g. a "transfer" transaction must include two asset or liability accounts). Otherwise they become a UI hint with no enforced meaning.
- A simpler starting set is `ordinary | opening_balance | adjustment`. `transfer` and `investment` can be UI tags derived from the posting accounts or added as controlled kinds once the constraints are defined.

**`payee_id nullable later`** — fine as a future FK to a payees table, but mark it explicitly `TEXT` or leave it out of Phase 1 schema entirely rather than adding a nullable integer with no referent. A stale FK column with no referent table causes confusion in migrations.

**`correction_of_transaction_id`** — correct concept; this is the "corrective entry" workflow referenced in the architecture doc. But there should be a DB CHECK: a transaction cannot be its own correction, and possibly a constraint that `status = 'voided'` implies `void_reason IS NOT NULL`. Add those.

**`supersedes_version_id`** — useful for linking versions within one transaction history. Add a DB-level constraint that `supersedes_version_id` must reference a version of the same transaction (not a version of a different transaction). This can be enforced via trigger or by app logic — document which.

**`status: draft | posted | voided`** — aligns with product-requirements.md which says "Physical delete is allowed only for never-posted drafts." The conventions doc says "draft versus posted lifecycle" must be decided before transactions. This satisfies that gate.

---

### `journal_entries` Table

The multi-journal-entry model for delayed transfers (examples 5 vs 6) is the right structural decision. Most apps collapse this into two transactions, which forces artificial linking. Having two journal entries under one `transaction_version` is cleaner and keeps the UI "this is one transfer" while the ledger stays correct.

**`entry_kind`** values: `ordinary | transfer_leg | exchange | investment | opening_balance | adjustment` — same concern as `transaction_kind`. If `entry_kind` is purely informational, say so. If it has enforced semantics (e.g. `exchange` entries must have exactly two commodity groups), document and enforce it. Without constraints, these become soft labels that drift.

**Missing: `book_id` denormalization** — for query performance (balance reports, account registers), it may be useful to have `book_id` here as well. Alternatively the join path `journal_entries → transaction_versions → transactions → book_id` is only three hops. Acceptable for now but worth noting.

---

### `posting_lines` Table

The `line_key` column (stable UUID/string) is a good idea for import deduplication and UI diffing. Clarify: is `line_key` set by the client/import, by the server, or both? If client-settable, it needs a uniqueness constraint per book. If server-generated, no issue but the client can't use it as an idempotency key.

`posting_lines` should also carry `book_id` for the same reason other identity tables do.

---

### `posting_versions` Table

**This is the most important table. The proposed structure is sound.** A few items:

**`quantity_value` + `quantity_scale` + `commodity_id`** — consistent with the existing commodity model in the schema (`commodities.standard_scale`, `commodities.max_quantity_scale`, `commodity_versions.standard_scale`). Good.

**`posting_line_id nullable`** — why nullable? The only reason to have a posting without a line identity would be system-generated postings (e.g. automated rounding adjustments or system corrections). If that is the use case, document it. Otherwise make it NOT NULL — every posting should have traceable UI lineage.

**`reconciliation_status: uncleared | cleared | reconciled`** — consistent with conventions.md which says "Reconciliation status belongs to postings or account-specific ledger state." Good. But the existing architecture says reconciled postings "should not be silently changed by a later edit." Add a DB trigger or application-layer guard: if a `posting_version` row's linked account has any `reconciled` posting in that version, a new corrective version must be used rather than superseding in place.

**`action`** — nullable, which is fine. But the proposed values (`buy | sell | dividend | fee | tax | transfer | cashback`) overlap significantly with `transaction_kind` and `entry_kind`. This is a third label for intent. Rationalize across these three: decide which is the canonical "what is this posting doing" axis and which are structural/routing labels.

**Missing: `recorded_at` / audit attribution** — `posting_versions` carries no `changed_by_user_id`, `change_reason`, or audit event reference. These are on `transaction_versions`, which is fine if the rule is "a new posting version is always inside a new transaction version," but that should be stated explicitly and enforced. If a corrective posting creates a new `transaction_version`, the attribution flows from there. If not, postings become attributionless.

---

### Balancing Constraints

The listed constraints are correct:

```
journal_entry has at least 2 postings
sum postings by commodity_id = 0
posting account allows_postings = true
entry_date >= account.opened_on
entry_date <= account.closed_on if closed_on exists
archived accounts rejected
default_commodity_id must match posting commodity when set
```

Two additions:

1. **Draft postings should allow incomplete/unbalanced state.** The constraint "at least 2 postings, sum = 0" should only apply when `transaction_versions.status = 'posted'`. This is mentioned in "Drafts may be incomplete" but should be reflected in the DB constraint or trigger conditions, not just documentation.

2. **The `default_commodity_id must match` constraint** — the existing `account_versions` table has `default_commodity_id` as a FK to `commodities`. The matching rule should say: if an account has a `default_commodity_id`, postings to that account must use that commodity. This is enforced at the app layer. Consider a trigger here since it is a financial invariant that should survive application bugs (per the architecture doc: "use SQLite constraints and triggers when they protect financial invariants").

---

### System Accounts — Gaps vs. Current Schema

The current schema has these system roles:
```
opening_balance
import_imbalance
retained_earnings
unassigned_income
unassigned_expense
```

The proposal adds:
```
transfer_clearing
commodity_trading
```

And proposes optional later:
```
rounding_adjustment
orphaned_gains
```

**`transfer_clearing`** and **`commodity_trading`** are needed from the first transaction. The FX example (example 7) and delayed transfer (example 6) both require them. These should be added to the `system_role` CHECK constraint in the migration and to the seeding logic in the same slice that introduces transactions.

The proposal says to defer `rounding_adjustment` — agreed, do not add it until a real rounding case appears.

**Account class alignment for new system roles:**
- `transfer_clearing` → `asset` (it's a temporary holding account, analogous to a clearing bank account)
- `commodity_trading` → should this be equity or a custom class? Common practice: use equity or a dedicated "trading gains" income account. The proposal doesn't specify. For multi-commodity balancing, `commodity_trading` acts as the counter-party for each leg. Equity is the conventional choice for a "suspense/clearing" commodity account. Specify this before migration.

---

### FX / Multi-Commodity Examples

All the FX examples (7, 8, 9, 10, 12, 13) use the `System:Commodity Trading` account correctly: each commodity balances independently, and the commodity trading account absorbs the asymmetric legs. This is the standard approach and is correct.

One item: in the investment sell example (10), the USD computation is:
```
478 + 2 - 80 - 400 = 0
```
This is correct. But where does the 80 USD realized gain figure come from in the posting itself? It requires knowing the lot cost basis. This implies the lot-matching computation happens before posting, and the `Income:Realized Gains` amount is a computed value. That computation must live in the app service, not be free-form user input, or you will get incorrect lot accounting. The architecture doc defers "investment lot accounting" — but if users can post investment sells without lot matching enforced, gains will be wrong. At minimum, document that investment transactions of kind `investment` require lot assignment and that the app must compute realized gain before posting, not allow arbitrary user input for the gain leg.

---

### What Is Missing From the Proposal

1. **`transaction_tags` / `posting_tags` junction table.** Tags are already implemented. The phase-1 plan explicitly deferred these: "Do not add `transaction_tags` or `posting_tags` until transaction and posting tables exist." Now they can be defined. The proposal doesn't mention them — add them as part of the transaction migration.

2. **Void workflow.** The proposal says `status: draft | posted | voided` but doesn't define the voiding schema mechanics. Per the architecture doc, void is a lifecycle event requiring audit attribution. Define: does voiding create a new `transaction_version` with `status = voided`, or is it a status update on the existing version? The former preserves immutability; the latter is simpler. The existing account/institution pattern uses a status column on the current version rather than a new lifecycle row. Clarify which approach transactions use.

3. **Draft auto-cleanup.** Physical delete is allowed for never-posted drafts. The schema needs either a DB policy (e.g. drafts older than N days are removed) or a documented API endpoint for draft discard. Otherwise draft rows accumulate silently.

4. **`entry_date` timezone.** The conventions doc says "Use calendar dates for financial facts." `entry_date TEXT NOT NULL CHECK (entry_date GLOB '????-??-??')` is the correct SQLite pattern for this (same as `effective_from`, `opened_on`, `closed_on`). Verify this constraint is in the migration, not just implied.

5. **Version ordering constraint.** `transaction_versions.version_seq` should have a DB CHECK `version_seq > 0` and a UNIQUE constraint on `(transaction_id, version_seq)`, matching the pattern for `account_versions` and `commodity_versions`.

--- 

### Summary

| Area | Assessment |
|---|---|
| Core mental model | Correct, fits existing versioning pattern |
| Sign convention | Correct; must be formally locked in conventions.md before migration |
| `transactions` table | Missing `book_id` |
| `transaction_versions` | `transaction_kind` values need either constraints or scope reduction; `payee_id` should wait for a payees table |
| `journal_entries` | `entry_kind` needs enforcement semantics defined |
| `posting_lines` | Missing `book_id`; `line_key` source (client vs server) unspecified |
| `posting_versions` | `posting_line_id` should be NOT NULL unless system-only exception is documented; needs attribution path clarified |
| Balancing constraints | Correct; need draft exemption noted in triggers/guards |
| System accounts | `transfer_clearing` and `commodity_trading` roles need account class specified; should be seeded in same migration as transactions |
| FX/investment examples | All examples are arithmetically correct; investment gain computation must be app-enforced, not free-form |
| Missing | `transaction_tags`, `posting_tags`, void workflow mechanics, draft cleanup, `entry_date` format constraint, `version_seq` uniqueness |

The proposal is production-quality thinking. Address the `book_id` omissions, nail down `transaction_kind`/`entry_kind`/`action` semantics (consolidate or constrain), define the void workflow, and add the two new system roles to the existing CHECK constraint before writing the migration.