---
name: validate-and-ship
description: How to run, debug, validate, review, commit, and document changes in Rekenraam - command matrix, dev environment, docs-update rules, review checklist of this repo's recurring bug classes. Use before committing any change, when running the app, or when deciding which doc to update.
---

# Validate And Ship

## Run the app

```sh
pnpm install                 # once, repo root
pnpm dev                     # backend :16888 + frontend :1888 (proxies /api)
pnpm dev:backend             # go run, APP_ENV=development
pnpm dev:frontend
```
Open http://localhost:1888. Dev SQLite lives at `backend/var/dev.sqlite`
(git-ignored). Key env vars: `HTTP_ADDR`, `DATABASE_URL` (`file:...`),
`APP_ENV` (`development`|`production`, defaults production),
`REKENRAAM_SECRET_KEY` (base64 32 bytes; required for import connections),
`TRUST_PROXY_HEADERS` + `TRUSTED_PROXY_CIDRS`, `OPEN_EXCHANGE_RATES_APP_ID`.
Owner password reset: `recover-owner` command (see
`docs/developer-workflow.md` § Local Owner Recovery) — it backs up and revokes
sessions; never edit the users table.

## Validation matrix (narrowest first — this is the contract; CI runs the same scripts)

| Changed | Run |
|---|---|
| `backend/**` | `./scripts/test-backend.sh` (= `go test -race -p 1 -timeout=15m ./...`), plus `go vet ./...`, `gofmt -l .` in `backend/` |
| Backend coverage check (optional, same script) | `COVERAGE=1 ./scripts/test-backend.sh` — non-race coverage pass, prints the merged total; CI enforces a soft floor (`scripts/check-coverage-floor.sh`) |
| `frontend/**` | `./scripts/test-frontend.sh` (openapi:generate + paraglide:compile + svelte-check); `pnpm --dir frontend run test` for unit-tested logic |
| OpenAPI | both scripts above |
| Integrated shape / static serving / embed | `pnpm build` (builds frontend, copies into `backend/internal/web/dist/`, runs embed test, compiles `dist/rekenraam`) |
| User journeys | `./scripts/test-e2e.sh` (self-contained: builds app, boots on 127.0.0.1:16889, fresh `backend/var/e2e.sqlite`) |

Never invent parallel validation commands; if a command must change, change
the script and CI together and update `docs/developer-workflow.md`.

## Review checklist — this repo's recurring bug classes

Every one of these shipped as a real bug here at least once. Check them on any
non-trivial diff (yours or reviewed):

1. **Silent limit clamping** — internal full-set reads using a paginated repo
   method whose limit gets clamped (import commit processed only 200 of 201
   rows). Internal reads use explicit `ListAll*` methods.
2. **PATCH omission overwrites** — optional update fields must be pointer
   types end-to-end; a plain `bool` made every rename silently disable
   auto-refresh.
3. **TOCTOU guard races** — check + insert in separate statements/transactions.
4. **Split-transaction crash holes** — a row and its idempotency marker written
   in different transactions (T-06, T-26 — both closed; the pattern recurs).
5. **Cursor boundaries** — `<=` vs `<`, resume token vs incremental boundary
   (see `background-work`).
6. **Unconsumed pagination** — frontend fetching page one and ignoring
   `next_cursor` (T-05).
7. **Reconciliation guard bypass** — any new mutation path over postings must
   be guarded (see `ledger-invariants`).
8. **Error-envelope drift** — new error codes not added to the OpenAPI enum;
   raw Go errors leaking to clients.
9. **i18n bypass** — hard-coded English in UI or in `lib/api/`.
10. **Logging financial content** — forbidden at every level.
11. **Builder-output tests that never reach the real consumer** — a test that
    checks a spec-building function's *return value* in isolation (e.g.
    `buildTransactionSpec`) is not the same as proving it survives the
    consumer's real validation. `EntryKind: "main"` sat in
    `buildTransactionSpec` since the import feature's first commit — invalid
    per `entryKinds`, so every `CommitImportBatch` call failed the instant it
    reached `TransactionService.CreateTransaction` for real — undetected
    because no test drove a staged row through the *actual* commit path
    against a real account (T-22). When testing a function that produces
    input for another service, add at least one test that calls the
    consumer for real, not just asserts on the producer's output shape.
12. **Creation dates masquerading as financial facts** — stamping a record's
    `effective_from`/`opened_on` with "today" makes every earlier posting
    fail, so installing the app now and importing years of history breaks.
    Shipped three times: commodities (T-42), user-created categories (T-43),
    import-created holding accounts (T-44), each fixed by opening the record
    at the genesis date `0001-01-01`. Ask of any new dated container: is this
    date a real financial fact (account `opened_on` — keep it, it should
    reject earlier postings) or app bookkeeping (everything above — genesis)?

13. **A duplicated helper is only as fixed as its least-visited copy** — the
    decimal-comma 100x error has now shipped **three** times from the same
    two-line pattern, `input.replace(/,/g, '')` before parsing, which reads
    `1,50` as 150. Fixed on the import side (T-36), then in the transaction
    editor and reconcile form (T-45), and it was *still live* in all three
    investment forms four months later (T-47) because the survey that scoped
    T-45 treated those forms as a later slice. The fix each time was correct;
    the **sweep** was what failed. So: when fixing a helper that exists in more
    than one place, grep the whole tree for the *pattern* before declaring it
    done, not just the copies the current ticket names — and count what you
    find, because the T-47 survey said two copies and there were seven.

14. **Consolidation that silently widens what is accepted** — retiring a
    private helper onto a shared one is a behaviour change unless proven
    otherwise. The investment forms' parsers rejected a leading `-` only as a
    *side effect* of running `/^\d+$/` over the concatenated coefficient;
    `parseDecimalAmount` handles signs properly, so a like-for-like swap would
    have started accepting negative share quantities (T-47). Ask of any such
    swap: what did the old code reject *incidentally* that the new code
    accepts? Then make the rejection explicit and named, and test it where the
    behaviour changed — per call site, not once on the shared module.

    Corollary: if the call sites are `.svelte` files, that test is impossible
    in place. This project has **no component-test harness** (no
    `testing-library`, no `jsdom`; vitest runs plain `.ts` only), so the
    validation has to be extracted to a module first. That is a feature, not an
    obstacle — it is the same reason G-02 existed.

15. **Producer drafts mistaken for posted ledger changes** — R9 generation
    initially inherited a create/edit reconciliation guard that checked dates
    without checking draft status (T-78). A test that creates the draft before
    reconciling cannot catch this: create and edit a producer draft after a
    checkpoint exists, prove no invalidation, then prove posting still requires
    its override. Draft-edit preview is not a promotion preview. Also test
    discard through the existing DELETE route: producer occurrence identity,
    its audit, and deletion must commit or roll back together (T-77).

16. **Animating a control's disabled fade** — `disabled:opacity-60` is safe on
    its own: the contrast rules exempt an inactive control, and axe skips
    disabled elements for `color-contrast` outright. Pairing it with a bare
    `transition` is not, because `transition` covers opacity: clearing
    `disabled` removes the exemption one to three frames before the 150ms ramp
    off 0.6 finishes, and an axe run that lands in that gap measures an
    operable control at 4.37:1 (T-93, an intermittent failure of `[acceptance]
    every report view is accessible`). Use `transition-colors` on any control
    whose disabled state clears *on its own* — a query settling rather than a
    click — so opacity stays a step function. Synchronising the test instead
    only moves the race, and costs the check its view of the loading state.

17. **A sound current projection hiding damaged disposal snapshots** — original
    allocation quantity, basis and proceeds mutations passed self-check because
    current lots were compared with lot events, while replayed allocations only
    contributed quantity and basis to that projection. Proceeds corruption and
    damage to superseded revisions could pass silently. Check every original and
    revision allocation set independently against its snapshot totals, including
    missing allocations, before using effective evidence for a projection.
    Negative proceeds are valid; nonpositive allocated quantity and negative
    basis are not. Named regression: `TestSelfCheckDetectsDisposalAllocationConservationDamage`.

18. **Offsetting disposal errors hiding behind sound group totals** — combined
    proceeds and per-lot conservation can both pass after two decisions and
    their allocation proceeds are changed in opposite directions. Immutable
    decision-to-clearing portions must conserve each decision and each pinned
    posting independently. Check provenance even for equal-valued journals.
    Named regression: `TestSelfCheckDetectsOffsettingDisposalProceedsDamage`.

19. **Unknown basis converted to a numeric zero** — preserve NULL coefficient
    and scale with explicit knowledge, through read APIs and CSV. One unknown
    lot makes the position's basis/gain unavailable; quantity and independently
    priced market value stay available. Never use known-only numeric fields
    without checking knowledge. Keep quantity self-check active and refuse
    unresolved inputs before known-basis pooling/disposal/range arithmetic.
    Named regression: `TestUnknownProjectedBasisDoesNotBecomeZeroGain`.

20. **Source identity lost on an imported correction descendant** — import
    audit origin marks the replacement as imported, while committed identity
    effects stay on the original fill. A direct-only identity lookup incorrectly
    fences later sale corrections and hides effective specific-lot elections.
    Read committed source provenance through immutable correction ancestry,
    matching the writer's lineage guard; never copy or rebind original effects.
    Named regression: `TestCorrectTrading212SalePreservesSpecificElectionWithoutGuessingQuantityChanges`.

21. **Order side mistaken for provider event type** — BUY/SELL does not
    establish an ordinary execution. Preserve the provider fill taxonomy and
    require explicit TRADE before native import, generic cash fallback or
    source correction. Unknown/missing types fail closed. Order lifecycle
    status cannot cancel an execution and must not create economic revisions.
    Named regressions: `TestImportUnsupportedFillCannotPost`,
    `TestUnsupportedSaleFillCannotUseCashFallback`,
    `TestUnsupportedFillCannotCorrectAcceptedBuyOrSale`.

22. **A preview reporting checkpoints for an impossible replay** —
    buy replacement and plain-buy previews validated journal shape without
    testing dependent lots. Audit every replaying entry path, including admitted
    reinvestment and imports, rather than only correction commands. Run the
    proposed domain write in a rolled-back path before reporting impact, with
    the same ordering, lot lineage and elections
    as commit. Never accept staged source evidence or expose temporary IDs.
    Snapshot durable rows after failure and repeated successful previews; the
    actual write still rechecks dependencies and reconciliation. Named regression:
    `TestBuyReplacementPreviewRejectsDependentDisposalWithoutWriting` and
    `TestBuyPreviewMatchesCommitWhenTransferBasisPropagates` and
    `TestReinvestmentPreviewPropagatesTransferBasisWithoutWriting` (since T-132
    a changed transfer basis propagates rather than refuses). Return
    the writer’s actual invalidated checkpoint set, including later checkpoints;
    a second latest-boundary calculation can underreport it. Pin multiple
    checkpoints in `TestBuyPreviewReplaysAndRollsBackReconciledHistory`.
    Shared reconciliation resolution must report all checkpoints the writer
    invalidates, once each, rather than only the latest checkpoint per candidate.
    Fix the common selector when generic transactions and other investment
    previews share it (T-127 #142). Named preview/commit pairs:
    `TestCreatePreviewReportsEveryCheckpointCommitInvalidates` and
    `TestSalePreviewReportsEveryCheckpointCommitInvalidates`. Test each
    checkpoint against its own `(date, sequence)` boundary through
    `activeCheckpointRefsAtOrAfter`; judging only the latest checkpoint let a
    same-day reorder across an earlier boundary through without override
    (T-120 #135, `TestPostingMoveAcrossEarlierSameDayCheckpointRequiresOverride`).
    A multi-journal investment command is guarded once on its combined delta
    read from every posting under its audit event; never re-add a per-journal
    guard or an up-front per-journal rejection, which refuses an inverse its
    replacement cancels (`TestQuantityOnlyBuyCorrectionPreservesCashCheckpointAndGuardsHolding`).
    Gain disclosure now also runs through these writer previews (T-114 #129 / T-126 #141).

23. **Replay silently restating committed gains** — a backdated or corrective
    command can change an earlier sale's basis/gain without touching any
    reconciled balance, so the reconciliation guard never fires. Any new
    replaying command must set `db.GainImpactPolicy` on its first journal, return
    `gain_impact` from its preview, and accept `gain_impact_acknowledgement` on
    commit; the writer recomputes and binds the set in-transaction. Test the
    changed, empty, stale and late-rollback cases. A preview must run the
    command's actual writer — reversal previews that planned only the inverse
    journal missed both impossible replays and gain changes (T-126). Comparing only inside replay
    persistence misses reversed/superseded disposals. Named regressions:
    `TestBuyGainImpactDisclosesFIFORevisionAndRequiresExactAcknowledgement`,
    `TestBuyGainImpactRejectsAcknowledgementOfAnEarlierChangeSet`,
    `TestGainImpactSnapshotDisclosesRemovedAndReplacedDisposals`.

24. **A new replay intent kind that moves quantity without pinning its
    journal** — replay rebuilds lot state from intents, but a posted journal
    leg does not move with it. A split replayed under a corrected history can
    multiply a different number of shares than its posted `H +d`; holdings and
    lots then disagree. Each journal-bearing intent must either refuse with
    itself named or post the difference as an adjustment journal linked to its
    operation and revision under the command's audit and checkpoint guard
    (T-129 does this for splits, ADR 0013 refinement); its per-lot replay
    output must be revisioned, and self-check must reconcile the operation's
    journals with its effective effects, not the originals. Once such an
    operation can be reversed or replaced, every reader of its latest
    revision must also filter to effective operations, and the self-check
    must count the successor's inverse journal (T-129). Named regressions:
    `TestQuantityCorrectionBeforeSplitPostsAdjustmentJournal`,
    `TestSplitReversalInvertsPrimaryPlusAdjustmentDelta`,
    `TestReversalsBeforeSplitPostAdjustmentJournals`,
    `TestEarlierAcquisitionBasisCorrectionReplaysThroughSplit`.
25. **A replay branch that omits a side effect its writer performs** —
    replay re-derives a position from intents, so anything the commit path
    writes besides lot effects (the method-family lock, a link's original
    date, a revision row) must be re-derived by the matching replay branch, or
    the first replay of that position silently drops it. The selected-lots
    transfer branch cleared the source's individual-lot lock this way until
    the `investment_replay_equivalence` self-check (T-134) found it. New
    commands or intent kinds: assert the replay check passes after a replay
    of the touched positions (the shared self-check pass helpers do).
    Named regression: `TestReplayKeepsSelectedLotTransferMethodLock`.
26. **Revision rows read without asking whether their operation is still
    effective** — `latest_*_revisions` views select the newest revision per
    decision or link, not per *effective* operation. Once an operation can be
    reversed, its revisions stay as evidence and must not describe current
    state: self-check counted a reversed transfer's link revision as a live
    lot event until T-119 joined `effective_investment_operations`. Any new
    reader of a latest revision for current state joins it too. Named
    regression: `TestReverseRevisedInternalTransferKeepsRevisionAsEvidence`.

27. **A self-check that reports damage on healthy books** — `checkpoint_integrity`
    summed only the postings a session cleared, ignoring the starting balance
    it carried from the previous checkpoint, so every second reconciliation
    failed; and it called a snapshot posting superseded whenever its
    transaction gained a version, so an unguarded description edit failed it
    too. A diagnostic is only trusted if it passes on the ordinary workflows:
    test each new check over chained and non-financially-edited data, and keep
    a damaged-data case beside it. Judge staleness by financial facts and
    position, never by version identity. Named regressions:
    `TestCheckpointIntegrityPassesAcrossChainedReconciliations`,
    `TestCheckpointIntegrityIgnoresNonFinancialEdits`,
    `TestCheckpointIntegrityStillFlagsChangedReconciledFacts`.

28. **Contrast measured in one palette only** — axe checks the theme a browser
    case renders, and the R3a contrast pass re-derived only the light default
    palette, so the warning and danger badges, accent badges over row hover,
    and four alternative accent palettes sat below 4.5:1 unnoticed. Any token
    change runs `src/lib/theme-contrast.test.ts`, which computes every theme ×
    base × accent pairing from `app.css`; a new text-on-tint pairing gets a
    line there. Named check: `theme-contrast.test.ts` (30 combinations).

29. **Position identity dropping cost currency** — one holding and instrument
    can contain several separately denominated positions. Svelte row keys,
    selection highlighting and lot-detail filters must include cost currency;
    a two-field key crashes the whole overview after a currency-changing
    correction. Browser regression: the currency-changing case in
    `investments-cash-in-lieu.spec.ts` visits the overview afterward and opens
    both positions, checking that each shows only its own lots.

30. **Position identity dropping side** — `investment_lots`, decisions and the
    method-family lock carry `position_side`. Before #173 every lot was long,
    so the disposal engine selected lots by account, instrument and currency
    only; once short lots exist, any such query lets a long sale consume a
    short lot (or a cover a long one). Every lot selection, method-family
    lock, basis-range guard and current-position read filters or keys by
    side; synthetic self-check events take the side of their lot. Named
    regressions: `TestShortCoverRefusesOverCoverAndOtherSideLots`,
    `TestPositionSideConflictRefusesOverlappingSides`.
31. **Transfer-link quantity read as the destination's quantity, or its
    instrument read as the destination's** — a share exchange (#177) is an
    `investment_transfer_facts` row of kind `exchange`: its link quantity is
    the *old* units, its destination lot holds them times the ratio, and the
    destination instrument is `destination_commodity_id`. A reader that opens
    or audits a destination from `link.quantity_value` or `f.commodity_id`
    (the closure edge, the self-check's revised-link events) is right for a
    transfer and wrong for an exchange. Read the destination lot's own
    quantity and `COALESCE(f.destination_commodity_id, f.commodity_id)`.
    Named regression: `TestShareExchangeUpstreamBuyReplacementRevisesTheNewLot`.
32. **A transfer link's source side assumed to be a `transfer_out` that moves
    units** — a spin-off (#180) is a transfer fact of kind `spin_off` whose
    parent lot keeps its units: its source effect is a quantity-0
    `basis_reduction`. A reader that finds a link's source event by
    `event_kind = 'transfer_out'`, or folds a revised link as `−link quantity`
    out of the source (replay intents, `effective_investment_lot_events`, the
    self-check's revised-link events), silently drops or double-moves a
    spin-off. Key the source event on the transfer kind. Named regressions:
    `TestSpinOffUpstreamBuyReplacementRevisesBothSides`,
    `TestSpinOffCarriesUnknownBasisAndResolvesThroughReplay`.

Fix workflow for any bug: failing named test first, then the fix, then the
full relevant suite.

## Docs to update in the same change (the docs ARE the product memory)

| What changed | Update |
|---|---|
| Feature shipped / status changed | `docs/implemented.md` (feature ledger); remove the item from the roadmap's current focus |
| Tech debt found or paid | GitHub Issue (`#number`) with evidence, bounded scope, measurable acceptance, dependencies and validation; retain existing local aliases in `docs/backlog.md`, never allocate new ones |
| Durable product behavior/scope | `docs/product-requirements.md` |
| Repo-wide rule/convention | `docs/conventions.md` |
| Long-lived tradeoff decision | new ADR in `docs/adrs/` (ADRs supersede everything once accepted) |
| Commands/layout/workflow | `README.md` + `docs/developer-workflow.md` (keep both consistent) |

Precedence when docs conflict: product-requirements → conventions →
early-architecture-decisions → ADRs govern all → developer-workflow.
`AGENTS.md` and skills are guidance, not product sources of truth.
`.archive/` is historical reference only — never port from it directly.

Tracking is event-driven: update the feature issue when work starts, scope or
blockers change, and delivery completes. The roadmap alone owns overall order;
priority is urgency and actual dependencies name their reason. Future initiative
issues may stay brief, but implementation starts only with actionable acceptance.
GitHub owns delivery checklists; plans/ADRs own durable scope and behavior.

Before closing, record shipped behavior, commit links, validation (and any
pending CI), retained limits and linked follow-ups. Close a bounded umbrella
when its agreed delivery is met; an unmet original acceptance item stays open.
An independent gap gets its own issue. Retired/duplicate work names its actual
disposition and successor. See `docs/conventions.md` → Work Tracking.

## Commits

Conventional Commits, smallest honest scope:
`feat(backend): ...`, `fix(api): ...`, `docs(requirements): ...`,
`test(backend): ...`. Don't mix unrelated refactors.

**Commit straight to `main`; do not open a branch.** Pre-release with one
contributor, so there is no PR review to reach and branches were where the
2026-08 merge damage came from. The commit message is the only review artifact:
if a change alters product rules, conventions, or ADRs, say so in its body.
Branch only when the work genuinely needs isolation (a throwaway spike, or
something you want CI to see first). This flips at the `v0.1.0` release — see
*Branches And PRs* in `docs/developer-workflow.md`, which governs.

## Definition of done for a slice

1. App still runnable end-to-end (`pnpm dev` or `pnpm build`).
2. Narrowest relevant validation green; new behavior has named tests.
3. OpenAPI + generated types in sync (if API touched).
4. Docs updated per the table above.
5. No violation of `ledger-invariants` (if money/ledger touched).
6. Focused conventional commit.
