# Roadmap & product-direction review (2026-07-19)

> **Status: resolved 2026-08-05.** All seven §3/§4 proposals were accepted by
> the owner and are recorded in `docs/roadmap.md` ("Decisions adopted
> 2026-08-05") with their scope fences. The §2 documentation-accuracy fixes
> are tracked separately. This document stays as the dated rationale; the
> roadmap is the source of truth for what was decided.

Point-in-time review of `roadmap.md`, `product-requirements.md`,
`competitor-comparison.md`, `competitive-analysis-2026-07.md`, `backlog.md`,
and `implemented.md`, cross-checked against the actual code state established
by the two 2026-07 backend audits
(`backend-ledger-investments-audit-2026-07-13.md`,
`backend-comprehensive-audit-2026-07-19.md`). Everything below is a
**proposal** — the roadmap's owner decides; nothing here edits the roadmap
itself.

---

## 1. Overall verdict

The strategy holds up. The 2026-07-02 direction (daily-driver core →
expat/multi-currency niche), the trade-execution rejection, the
distribution-before-hosting posture, and the R2 → R3 → R5 backbone all
survive contact with both the market docs and — importantly — the code
audits. The niche claim ("no product combines correct multi-currency
double-entry + lot-level investments + self-hosted web") remains true and is
now *better* substantiated: the audits confirmed the ledger core is genuinely
correct and regression-tested, which is the whole moat.

The issues found are: (a) three places where the docs overstate or contradict
what the code does, (b) audit findings that materially interact with roadmap
sequencing and aren't reflected yet, and (c) two of the competitive
analysis's own recommendations that the roadmap neither adopted nor
explicitly rejected.

## 2. Documentation accuracy fixes (do these regardless of prioritization)

1. **`competitor-comparison.md` overstates corporate actions.** The OSS
   matrix row "Dividends / corp. actions" marks Rekenraam ✅. The 2026-07-19
   audit confirmed: dividends ✅, corporate actions ❌ — splits, mergers,
   spin-offs, and delistings have *no implementation*, not even manual entry
   (schema stubs only; a worthless security can't be written off at all,
   audit P4). GnuCash and Portfolio Performance genuinely handle splits, so
   this cell currently hides a real parity gap in the product's flagship
   area. Change to ✅ dividends / ⬜ corp. actions, and keep it split
   permanently — they are different features.
2. **Lifecycle taxonomy vs FX coverage.** The transaction-lifecycle taxonomy
   (conventions + ledger-invariants skill) says durable drafts "may trigger
   background FX coverage." `implemented.md` says the opposite ("Drafts/
   previews do not trigger downloads") and the code implements
   `implemented.md` (the trigger requires `status='posted'`). The code's
   behavior is fine; fix the taxonomy wording so the next auditor doesn't
   flag it again (this one did — audit P5).
3. **Slice-numbering ghosts.** R4 (shipped QIF), R7 vs R7a, R12 (referenced
   in the competitive analysis, defined nowhere), and the R6/R7 "Deliberately
   later" bullets make the sequence hard to cite. One renumbering pass, or an
   index table mapping every R-number to status, would stop the drift.

## 3. Re-prioritization proposals

### 3a. Add "EU import correctness" to the pre-announcement gates (new, urgent-by-context)

Audit P1/P2: the QIF importer parses `MM/DD` before `DD/MM` with the profile
override stubbed out, and strips commas so `"1,50"` imports as `150`. The
announcement plan's centerpiece is the migration story ("migrate from MS
Money/Quicken in 10 minutes"), the QIF parser explicitly targets MS Money
exports, and the chosen niche is *European* expats. As it stands, the exact
demo the launch depends on silently corrupts dates and amounts for the exact
audience the product targets. This is small work (the profile plumbing
exists) with launch-critical blast radius: a self-host community's first
impression of a *correctness-branded* finance app importing March 4 as
April 3 is unrecoverable. Proposal: fix now or fold into R5's profile work,
**and** list it as a hard pre-announcement gate next to the secrets scrub.

### 3b. Extend R3 from "portable" to "portable and protected" (backup automation + self-check)

R3's rationale is trust ("a newcomer can migrate, inspect, and export").
Two audit observations belong in that same trust story at near-zero cost:
verified `VACUUM INTO` backup already exists but is reachable only from CLI
recovery — schedule it (the background-work queue is already there) with
retention and a visible last-backup status; and add a trial-balance
self-check (per-commodity posting sums ≡ 0, lots ↔ holdings reconciliation,
`integrity_check`) surfaced in the UI. "Your data is exportable, backed up
nightly, and provably balanced" is a stronger trust sentence than exports
alone, and no self-hosted competitor says the third clause.

### 3c. Pull a minimal rules engine *into* R5 rather than after it

The competitive analysis already concluded rules are Firefly's stickiest
feature and "the retention feature" for the import-heavy persona — but the
roadmap still parks rules in "Deliberately later." The contradiction matters
at R5 specifically: shipping CSV import *without* rules means every imported
statement needs manual per-row categorization, which is precisely the churn
that makes people abandon import-driven tools in week two. A deliberately
minimal v1 — ordered rules of "payee/description contains X → set
category/payee/tags," run inside the staged preview where the user already
reviews rows — reuses the pipeline, needs no new UI paradigm, and defers the
full engine (regex, amount predicates, rule audit) to the later slice.
Alternative if R5 scope pressure wins: keep rules out but *record the
decision* in the roadmap, because right now the analysis recommends it and
the roadmap silently doesn't.

### 3d. Resolve the R10-forecasting promotion question explicitly

Same pattern: the competitive analysis (§3.3, §5.4) recommends promoting
multi-currency cashflow forecasting — "the niche-defining feature nobody
ships" — into the R8/R9 planning work, but the roadmap (reviewed five days
later) leaves R10 third in the planning loop without recording why. Either
ordering is defensible (budgets have broader daily-driver pull; forecasting
has sharper differentiation). The gap is that the roadmap's own governing
analysis disagrees with it and no decision is written down. My lean, for
what it's worth: R9 (recurring) before R8 (budgets), then R10 immediately —
recurring transactions are forecasting's data source, so R9→R10 is one
coherent arc and the draft-producer machinery the audits confirmed ready
gets exercised once, not twice.

### 3e. Name a small "investment lifecycle completeness" slice

Three audit findings plus one design item form a natural, mostly-backend
slice that directly protects the moat claim: **manual split/reverse-split
entry** (the lot-mutation design is the missing half of backlog T-34, and
splits are the one corporate action every multi-year portfolio hits — a
Quicken Premier migrant's first AAPL split shouldn't require deleting and
re-entering lots), **zero-proceeds write-off** (audit P4), **price
observation voiding** (audit P3), and **return-of-capital as basis
reduction**. None needs a provider feed; all four are prerequisites for the
matrix cell in §2.1 to honestly become ✅. Slot after R5, before or alongside
the planning loop.

### 3f. Personal-access tokens before the public announcement

The competitive analysis names ecosystem as the weakness and the typed
OpenAPI surface as the foundation to grow one — but the only auth is a
browser session cookie + CSRF header, so no third-party tool, script, or
community mobile client can actually call the API (2026-07-19 audit, §4).
Firefly's ecosystem exists because of its PATs. The moment of maximum
community-developer attention is the announcement; shipping tokens after it
wastes the spike. Small, well-understood work over the existing
session/hashing infrastructure.

### 3g. Unchanged and reaffirmed

R2 first (every review leads with dashboards) — yes, and it's the right
place to spend UI polish. Security gates as a parallel track — yes. Trade
execution stays rejected; the trade-planner alternative stays "later" — yes.
Second provider (IBKR Flex Query) stays behind R5 — yes.

## 4. Additional user groups — analysis

Framework: a group is attractive when the audited codebase already serves
80% of its needs, it overlaps the existing persona's communities, and it
doesn't re-open a rejected direction.

### Strong fit, cheap to serve

**1. Crypto-holding expats / multi-currency crypto users.** The audit found
the lot engine is commodity-kind-agnostic and already handles scale-24
crypto commodities end to end via API; what's missing is only an instrument
type, a UI entry point, and one BYO-key price adapter (e.g. CoinGecko —
fits the adapter rule). That's a small slice for a real segment nobody
self-hosted serves with *actual cost-basis accounting* (Ghostfolio: no cost
basis; Rotki: the specialist, but heavy, desktop-bound, and portfolio-only).
Crypto is also disproportionately an expat/cross-border instrument, so this
widens the existing persona rather than adding a new one. **Guardrail:**
scope to manually/CSV-entered and priced holdings with lots; explicitly do
NOT chase exchange integrations or DeFi — that's Rotki's tarpit and a
coverage promise the adapter rule forbids.

**2. Plain-text-accounting adjacents (Beancount/hledger users who want a
UI, and the people who consult them).** Zero feature cost — this is a
positioning group. They already believe in exact precision, double-entry,
auditability, and data ownership; Rekenraam is "their values, with a UI."
Two cheap actions: design the R3 export shape so a ledger/beancount-format
export is trivially derivable (even if shipped later), and mention the
correctness architecture (append-only versions, exact decimals, trial
balance) prominently in announcement material. This group writes the HN/
r/selfhosted comments that decide a launch; their approval is marketing
that can't be bought.

**3. Net-worth trackers (Kubera-ish, FIRE community).** Mostly served by
what's already built or planned: manual asset accounts, R2 net-worth
series, multi-currency. The schema already anticipates non-market assets
(`asset_valuation` reconciliation source kind). One small feature — a
guided "revalue asset" flow writing manual price observations for a
house/car/collectible — plus R2's charts makes the app a credible Kubera
alternative with ownership. Low cost, and FIRE forums are dense with
exactly the self-hosting, spreadsheet-refugee persona.

### Plausible later, not now

**4. DACH retail investors (Portfolio Performance's audience).** Large,
self-host-friendly, investment-literate — but the entry bar is R13 returns
analytics (TWR/MWR is table stakes there), a German UI (currently an open
decision — this group is the strongest evidence for German as the first
non-English language, with Dutch second given the NL expat overlap and the
product's own name), and eventually jurisdiction-aware gains. Right group,
wrong year; revisit at R13.

**5. Couples/households (second user).** Real demand (it's a Monarch
selling point) and the most-requested thing self-hosted finance apps hear.
But the audits show single-user is load-bearing (owner-only auth,
`BookID=1`), so even a read-only second login touches the auth core. Keep
deferred exactly as the roadmap does; when it comes, "read-only household
viewer" is the cheap first step, not multi-owner.

### Stay rejected

**6. Small business / freelancers.** The 2026-07-02 rejection reasoning
(VAT/e-invoicing regulatory treadmill; businesses buy compliance + support)
is still correct, and the audits add a practical argument: the ledger's
correctness guarantees are calibrated to personal finance; invoicing, AR/AP,
and tax filings would each demand their own invariant layers. The
freelancer-lite variant ("just tag VAT on categories") is the top of that
same slippery slope — decline it deliberately.

**7. Trade automation users.** Nothing in this review changes the
execution rejection; the audit's API-readiness findings (no PATs, no
idempotency keys) would each be hard blockers anyway.

## 5. Suggested roadmap shape (consolidated proposal)

1. **R2** reports (unchanged) — plus the §2 doc-accuracy fixes now.
2. **R3** exports **+ scheduled backups + trial-balance self-check** (§3b).
3. **R3a** accessibility (unchanged).
4. **P1/P2 QIF locale fixes** — now or as the opening task of R5; added to
   pre-announcement gates either way (§3a).
5. **R5** CSV import + profiles **+ minimal rules v1** (§3c).
6. **Investment lifecycle completeness** slice: manual splits, write-off,
   price void, return-of-capital (§3e). Alongside: crypto instrument type +
   one price adapter (§4.1) if the widened persona is wanted.
7. **PATs** before announcement (§3f); announcement assets per the existing
   plan, courting the plain-text-accounting audience explicitly (§4.2).
8. Planning loop with the R9 → R10 → R8 question resolved explicitly (§3d).

Net effect: the backbone is untouched; the additions are small,
trust-and-correctness-shaped, and every one closes a gap that either the
launch story or the moat claim currently rests on without support.
