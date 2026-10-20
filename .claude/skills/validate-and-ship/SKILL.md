---
name: validate-and-ship
description: How to run, debug, validate, review, commit, and document changes in Rekenraam - command matrix, dev environment, docs-update rules, review checklist of this repo's recurring bug classes. Use before committing any change, when running the app, or when deciding which doc to update.
---

# Validate And Ship

## Run the app

```sh
pnpm install                 # once, repo root
pnpm dev                     # backend :16888 + frontend :1888 (proxies /api)
pnpm dev:backend             # native SQLite wrapper, APP_ENV=development
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

Frontend commands must work after a clean install, without generated state from
an earlier dev/build run. `catalogs:check` first runs `svelte-kit sync` to create
SvelteKit's `$app/tsconfig` before Vitest transforms catalog tests (#207). Keep
that bootstrap in the shared gate used by dev/check/build and direct catalog
checks; CI must not need a separate warm-up or skip catalog validation.

| Changed | Run |
|---|---|
| `backend/**` | `./scripts/test-backend.sh` (= `go test -race -p 2 -timeout=5m ./...`), plus `./scripts/go-backend.sh vet ./...`, `gofmt -l .` in `backend/` |
| Backend coverage check (optional, same script) | `COVERAGE=1 ./scripts/test-backend.sh` — non-race coverage pass, prints the merged total; CI enforces a soft floor (`scripts/check-coverage-floor.sh`) |
| `frontend/**` | `./scripts/test-frontend.sh` (openapi:generate + paraglide:compile + svelte-check); `pnpm --dir frontend run test` for unit-tested logic |
| OpenAPI | both scripts above |
| Integrated shape / static serving / embed | `pnpm build` (builds frontend, copies into `backend/internal/web/dist/`, runs embed test, compiles `dist/rekenraam`) |
| After a non-trivial merge | `./scripts/check-post-merge.sh` — migration/ADR number collisions, OpenAPI client drift, duplicate doc paragraphs, duplicate workflows, colliding IDs, tests lost by the merge (*Post-Merge Audit* in `docs/developer-workflow.md`) |
| Changed the bundle export, the self-check, or a whole-book read | `./scripts/measure-large-book.sh` — 250k-posting book, bounded wall time and peak heap (*Large-book measurement* in `docs/developer-workflow.md`); weekly in CI, never on push |
| User journeys | `./scripts/test-e2e.sh` (self-contained: builds app once, one instance + fresh SQLite per worker under `backend/var/e2e/`, ports from 127.0.0.1:16900; `E2E_SKIP_BUILD=1` to reuse the binary) |

Native SQLite requires CGO and a C compiler with development headers. Supported
commands source `scripts/native-sqlite-env.sh` and enable `fts5,netgo,osusergo`.
Use `./scripts/go-backend.sh test ...` for focused checks. Go race detection
covers application and driver-wrapper Go code; it does not instrument the
bundled SQLite C engine.

ADR 0016 sets two concurrent packages and a five-minute per-package deadline
for both race and coverage modes. Normal commands use available Go CPU
resources; the old four-core tuning advice applies only to historical modernc
measurements. CI's integrated build runs independently, and every check remains.

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
   Expiry clocks sampled before pool/SQLite writer waits also go stale (#359):
   acquire write admission before sampling final token/session guards. Keep
   `TestPersonalAccessTokenAuditExpiryWhileWaitingForDatabase`,
   `TestPersonalAccessTokenAuditExpiryWhileWaitingForSQLiteWriter` and
   `TestPersonalAccessTokenAuditManagementSessionExpiresWhileQueued` green.
   Session/device reads check their returned deadline with a fresh clock, without
   taking a writer lock (#362). MFA consumption and throttle decisions acquire
   SQLite write admission before sampling time (#363/#364); the final MFA session
   insert also pins the consumed challenge against disable (#365). Keep
   `TestAuthExpiryAudit*` green. Reservations pin persisted generation identity
   (#366): failure, refund and
   success reset must never affect a replacement/missing generation, including
   same-timestamp recreation. Keep `TestAuthExpiryAuditReservationRolloverProbe`,
   `TestThrottleRollover*`, `TestThrottleReset*` and the migration-preservation
   control green. Count verified guesses through real consumers, not just status
   codes; an older generation may finish after rollover.
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
    The same holds across a **pipeline**: #300 had the adapter apply the
    file's declared separator and commit re-read rejected text with per-value
    detection. A value rejected at staging is recorded on the row and refused
    at commit, never re-parsed (`TestImportAuditRejectedConventionRowsAreFlaggedAndNeverCommitted`).

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
    A sourced split initially without eligible lots is journal-free (#146),
    but must remain in replay and activity-date admission. Revised movements
    anchor to its sourced fact when there is no original event. Named cases:
    `TestBackdatedAcquisitionEntitledToJournalFreeSplitPostsAdjustment`,
    `TestJournalFreeSplitAdjustmentUsesGainAndCheckpointGuards`,
    `TestJournalFreeSplitExemptionDoesNotHideMissingJournals`.
    Operation-addressed correction (#196) covers both journal shapes: never
    promote an adjustment to a primary, fabricate a zero inverse, or drop a
    journal-free successor's source/memo. Keep source identity through cash-in-lieu
    correction and import dedupe. Named cases:
    `TestSplitOperationReplacementCrossesZeroBothWays`,
    `TestJournalFreeSplitReversalBeforeAndAfterEntitlement`,
    `TestJournalFreeSplitReversalExemptionRequiresNonzeroInverseJournal`.
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
    Entry requests must carry that selection through the application and
    disposal command too, including zero-proceeds write-offs with no cash
    currency. Named regression: `TestWriteOffMixedCurrenciesEachMethodConsumesOnlySelectedCurrency`
    (#199); historical entry and native correction retain the same pin.

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
33. **Every disposal decision assumed to dispose units** — a cash-and-stock
    merger's cash decision (#181) has quantity 0, and so do its allocations
    and its `disposal` lot events: it realizes basis, the units leave through
    the exchange. A reader or validator that requires `quantity > 0` (replay
    persistence, the allocation-set and lot-reconciliation self-checks) or
    that replays every decision as an ordinary `disposal` intent breaks it.
    Exclude the merger's decision from the disposal intents (its exchange
    group replays it) and allow zero only for that kind. Named regressions:
    `TestBuyCorrectionBeforeCashStockMergerRevisesRealizedAndCarriedBasis`,
    `TestCashStockMergerUnknownBasisLeavesResultUnresolved`.
34. **Reading `effective_investment_lot_events` as if it were complete** —
    the view only *removes* events that a correction or a replay revision
    superseded; it never adds the revised amounts back. A reader that folds
    it alone forgets every revised cover, sale, split, return of capital and
    transfer link. The named-short quantity did, so a cover revised by a
    backdated opening stopped counting: 15 owed instead of 11, unexplained
    negative units passed as short, and the net-worth series showed a total it
    should withhold. The side guard did too, admitting overlapping long and
    short positions. Every existing self-check passed (#188). A current-state
    reader folds `db.EffectiveLotMovements` (#189), which owns those
    add-backs; `TestEffectiveHistoryReadersUseTheOwner` refuses a new direct
    reader unless it is allow-listed with a reason. A reader that needs only an
    operation's dates and holding takes the original events of effective
    operations, because a revision changes neither. Test a revised case, and
    test the visible warning, not just the quantity. Named regressions:
    `TestNamedShortCountsARevisedCoverOnItsDate`,
    `TestUnexplainedNegativeUnitBesideARevisedCoverIsFlagged`,
    `TestShortOpeningInsideALongClosedByARevisedTransferIsRefused`.
    Replay equivalence cannot see this class, because stored state and replay
    agree. The independent oracle (`requireConservationOracle`, #190) checks
    readers against the posted journal and operation facts. It runs in every
    shared self-check pass helper; call it before a later correction retires
    the revision under test.
35. **An operation kind added without its capabilities** — kinds are
    special-cased by string across generic code, so a new kind can ship
    without replay, a correction, export rows or a passing self-check. Writers
    refuse kinds missing from `db.InvestmentOperationKinds`. The capability
    matrix (#191) fails until the kind has a row naming, for each capability,
    a test, a refusal test or a gap issue, and until a capability scenario
    records it. A new replay-intent family needs its own row too.
36. **Canonical text CHECKs that inspect only a valid prefix** — SQLite text
    `length` and `GLOB` stop at an embedded NUL, so a digit-only CHECK admitted
    `1\0abc` and trailing NULs in every coefficient column (#192). Canonical
    ASCII coefficient CHECKs also require `instr(value, char(0)) = 0`, outside
    any zero allowance; this works across SQLite text encodings. Test installed
    CHECKs through actual insertions, including embedded/trailing NULs, signed
    boundaries and nullable amounts.
    Named regression: `TestExactNumberColumnChecks`.

37. **Validating a different representation from the one readers consume** —
    Go struct JSON decoding matches keys case-insensitively, while immutable
    source JSON and browser readers keep exact keys. `Reference` could override
    a blank `reference` during validation, then history displayed the blank
    canonical value (#198). Read required keys exactly when preserving raw
    JSON. Also align text limits: JSON Schema counts Unicode characters,
    HTML maxlength counts UTF-16 units, and Go len counts UTF-8 bytes. Source
    forms count Unicode characters explicitly rather than depending on a
    native maxlength. Named cases:
    `TestBasisResolutionRequiresSourceDeclarationWithoutWriting`,
    `TestBasisResolutionSourceTextUsesUnicodeCharacterLimit`.
    Compare source metadata without float decoding too: spin-off and merger
    correction treated adjacent integers above 2^53 as unchanged evidence
    (#309). Reuse the exact-number provenance comparator; named consumer:
    `TestCorporateActionCorrectionDistinguishesExactSourceNumbers`.
    Share-exchange replacement must compare that effective evidence too and
    retain it when omitted (#310):
    `TestShareExchangeCorrectionRejectsUnchangedSourceEvidence`. A server
    comparator cannot repair metadata the browser already parsed: a form that
    spreads parsed evidence back into its payload rounds unedited numbers
    (#311). Correction forms send only the fields the owner changed as
    `source_evidence_edits` and the server merges them into the raw recorded
    tokens. Named cases:
    `TestSpinOffReplacementPreservesExactUneditedSourceEvidence`,
    `TestSpinOffCorrectionAPIPreservesExactUneditedSourceEvidence`. Every
    investment correction form follows it (#312): a form that does not edit
    evidence sends none, and an omitted evidence keeps the recorded one (never
    `{}`), so a no-op check compares effective evidence, not "evidence was
    sent". Named cases:
    `TestInternalTransferReplacementPreservesExactUneditedSourceEvidence`,
    `TestSplitReplacementPreservesExactUneditedSourceEvidence`,
    `TestSplitCorrectionAPIPreservesExactUneditedSourceEvidence`.

38. **Treating source corrections as amount corrections** — comparing only an
    exact basis amount refused a genuinely changed declaration (#197). Evidence
    changes at equal basis append their own immutable, attributed revisions,
    without inverse journals, economic replacement or replay. Pin both the
    effective resolution and evidence revision in the form and recheck inside
    the writer. JSON layout/key order or a reason alone is a no-op. Preserve
    additional metadata server-side as raw JSON: browser object parsing can
    round numeric tokens, and rebuilding only typed fields drops metadata.
    History renders the exact saved JSON. Named cases:
    `TestProvenanceCorrectionPreservesDownstreamSalesTransfersAndCheckpoints`,
    `TestProvenanceCorrectionRefusesNoOpsInvalidDeclarationsAndStaleWrites`,
    `TestProvenanceCorrectionPreservesExactAdditionalMetadataOnServer`.

39. **Browser “today” and refreshes rewriting working copies** — financial
    defaults come from session `owner_today` (#101), including report years,
    investment discovery and provider-assignment dates. Search the whole
    frontend for private current-date helpers. Editable initializer effects
    must not track the shared date. Keep dialogs outside loading branches and
    retain their opening snapshot, so a refreshed workspace date cannot remount
    them and discard selected dates/amounts. Check an opposite-midnight browser,
    year rollover and refresh followed by reopening; see
    `owner-local-dates.spec.ts` and `owner-local-date-contract.spec.ts`.
    A valuation's date must also constrain holding events: current lot state
    can include recorded future buys, sales or covers. Omitting a gains date
    must match explicit owner-local today. Named regressions:
    `TestDatedValuationDefaultExcludesFutureInvestmentEvents`,
    `TestInvestmentGainsDefaultTodayKeepsFutureSaleOutOfValuation`.
    The backend has the same trap: grep `UTC().Format(time.DateOnly)` and
    bound/default owner-entered dates through `SettingsService.OwnerTodayAt`
    (#339, `TestOwnerEnteredDatesFollowOwnerDay`).
    Carry the authenticated owner into blank transaction and ledger-read
    defaults too (#343); undated FX source assignments use the owner's day
    at refresh start (`TestBlankTransactionAndLedgerDatesFollowOwnerDay`,
    `TestUndatedFXRefreshSelectsAssignmentsOnOwnerDay`).

40. **Checking a process lock without holding it through replacement** —
    `CheckAvailable` releases its lock immediately; it cannot protect a later
    restore from concurrent startup (#227). A destructive file replacement
    needs ownership throughout the operation, and lock handoffs must preserve
    one lock-file inode: unlinking after unlock can split contenders across
    different files. #227 retains ownership and keeps the file in place.
    `TestTwoProcessRestoreOwnsLockThroughoutReplacement` pauses before/after
    replacement and refuses startup and another restore;
    `TestTwoProcessLockHandoffsKeepOneFileIdentity` stages pre-opened contenders;
    `TestRestoreCommandReleasesLockOnFailure` preserves error-path recovery.

41. **Upload limits that silently return a prefix** — `ParseMultipartForm`'s
    argument is a memory threshold, not a request limit, and `LimitReader` alone
    silently truncates oversized files (#228). Bound the entire request with
    `MaxBytesReader`, including unknown-length bodies and multipart epilogues;
    check file size and read at most limit + 1 before accepting. Remove multipart
    spill files on every path. Prove the exact supported size works and one byte
    over an independently parseable prefix refuses without durable changes.
    Both staging and CSV analysis must show the translated size envelope.
    Named regressions: `TestImportUploadFileSizeBoundary`,
    `TestImportUploadMultipartResourceLimits`, `import-upload-limit.spec.ts`.

42. **Browser journeys that depend on live providers or late handlers** —
    the e2e app runs its background workers, so a real network call can add
    evidence to a worker's book (#226: a live ECB fixing outranked a voided
    fixture in evening CI runs). Every outbound provider goes through a
    development-only stub override (`TRADING212_BASE_URL`,
    `MARKET_DATA_BASE_URL`) refused outside `APP_ENV=development`. A
    `route.fetch()` handler still reading a refetch when the page closes fails
    as "Response has been disposed" (#236): unroute with `ignoreErrors` after
    each test. The once-a-minute recurring scheduler likewise sweeps every
    enabled template, so a spec must not assume nothing materializes between
    saving a template and its next API call (#282): assert the occurrence
    the app recorded, not a `run-now` counter. Named regressions: `TestDefaultRegistryAtSendsEveryProviderToTheOverrideHost`,
    `TestLoadRefusesMarketDataBaseURLOutsideDevelopment`,
    `TestRuntimeRefusesMarketDataBaseURLOutsideDevelopment`.

43. **One response assembled from different effective histories** — a shared
    price `knowledge_cutoff` does not freeze ledger corrections or observation
    retirements (#237). Live gains and statements use `ReadGains` /
    `ReadGainsStatement` to capture all operational, comparison, boundary,
    income, historic-basis and FX dependencies once. Never combine standalone
    stage calls against live repositories. Deterministic stage interleavings
    must retain the same figures and evidence after source corrections and
    withdrawals, with no durable read-side writes. Named regressions:
    `TestLiveGainsCorrectionBetweenOperationalAndFXKeepsSameCurrencyGain`,
    `TestLiveStatementBoundariesDisposalsIncomeAndFXShareEffectiveHistory`,
    `TestLiveGainsAPICorrectionBetweenCompositionStages`.

44. **A complete formula mistaken for a complete return** — period gains can
    correctly sum realized + dividends + unrealized change while excluding
    capital-return excess (#238). Disclose the excluded amount and unknown
    coverage beside the total; retain payment versus effective dates and
    independent FX coverage. Never add the basis reduction again. New saved
    disclosure needs its own document version; old output bytes remain frozen.
    Named regression: `TestGainsStatementCapitalReturnExcessIsSeparateFromLimitedTotal`.

45. **Filesystem paths treated as SQLite URI syntax** — concatenating `file:`
    with a path lets `#`, `?` and percent escapes select a different file
    (#128, #239). Build file URIs with `db.SQLiteFileURL` before adding query
    options, including template and auxiliary test connections. Exercise unnamed
    subtests (`#00`) and punctuation in the temporary root; assert migration,
    copy isolation and reopening the returned URL. Named regressions:
    `TestOpenEscapesFixturePaths`,
    `TestTemplateAndFixturesEscapeTemporaryRoot`,
    `TestFreshDatabaseHelpersEscapeTemporaryRoot`,
    `TestRecoveryAndRestoreHelpersEscapeTemporaryRoot` and
    `TestRestoreDrillMatchesSourceTrialBalance`.

46. **Missing defaults silently discarding known financial attribution** —
    Trading 212's dividend writer used to fall back to ordinary cash when an
    income default was absent, losing a payer already resolved (#242). Route
    known dividends through explicit owner review and the named writer. Missing
    gross/tax stays pending beside unrelated valid rows; cash does not prove
    gross or zero tax. Historical repair must retain the same payment/source
    identity, pin its current version inside the writer, and preserve unchanged
    reconciliation. Test late identity failure, setup cleanup/retry and dedupe
    after native owner correction. Named evidence:
    `TestTrading212DividendOwnerReviewRetainsKnownPayerWithoutDefaults`,
    `TestImportedDividendRepairKeepsTransactionCashSourceAndNativeCorrections`,
    `TestImportedDividendRepairWriterPinsVersionAndSourceEligibility`.

47. **Security state checked before a later independent write** — enrollment
    used to overwrite an activation and activation could accept a code for a
    replaced secret (#261). Pin the verified pending secret at the write and
    refuse demoting an active enrollment. Exercise deterministic interleavings
    through real services, not just a happy-path TOTP vector. Browser cookie
    lifetimes must match persisted session/device policy too (#263); test both
    password and completed-MFA HTTP consumers and existing-device renewal.

48. **A budget checked before a verifier and recorded after it** — the sign-in
    and reauthentication throttles read the counter, ran Argon2, then wrote
    count+1: overlapping failures were lost and 16 parallel guesses all got
    verified against a budget of 5 (#285). Every verifier entry point reserves
    a place atomically first (`reserveAttempt`) and defers `release`. Count
    verifications from the auth log in a parallel burst, not the response
    codes (a `429` can follow a checked guess). Partial identity writes
    belong in one transaction (#287), and a maintenance command that revokes
    sessions must hold the server lock (#286). Named regressions:
    `TestConcurrentPasswordGuessesNeverExceedTheBudget`,
    `TestConcurrentReauthenticationGuessesNeverExceedTheBudget`,
    `TestFailedDeviceApprovalLeavesNoOrphanSession`,
    `TestRecoverOwnerCommandRefusesWhileTheServerHoldsTheDatabase`.

49. **Partial form labels matching data inside another control** — the mobile
    account journey's `getByLabel('Name')` also matched the Parent select when
    its wrapping label contained an account option with "name" in it. Worker
    history made this intermittent. Match the Name field exactly on create
    and edit, and seed a colliding account in the journey itself rather than
    relying on another spec to expose it (`reference-data-management.spec.ts`).
    Likewise, scope payment cards by a fixture's unique source identity rather
    than amount/kind alone (`income-coverage.spec.ts`); repeated specs may
    retain earlier equal-valued payments in the same worker's book.

50. **Continuation deduplicated against itself** — an FX continuation enqueued
    its identical successor before completing, so active deduplication returned
    the running item and completion lost the remaining horizon (#314). Complete
    and enqueue atomically, with lease fencing and rollback; exercise more than
    two chunks (`TestFXCoverageWorkerFinishesEveryContinuationChunk`). #315
    also fixes starvation at missing publication dates: persist scan progress
    separately from observations, retry failed dates independently, count every
    attempt against the budget, and test fresh posting demand while a cursor
    is queued (`TestFXCoverageFreshDemandRevisitsLatePublicationAndEarlierDates`).

51. **Transport errors copying operator secrets into history** — Go HTTP
    errors render the full request URL, including Open Exchange Rates' app_id
    (#316). Redact at the HTTP boundary before persisting/logging; preserve
    classification through unwrapping rather than interpolating the cause.
    Exercise the actual FX refresh history consumer
    (`TestFXRefreshTransportFailureDoesNotPersistOperatorSecret`).

52. **Colour utility naming no token** — Tailwind emits nothing for an
    unknown colour, so `text-destructive`, `text-warning-foreground` or
    `bg-accent-soft` compile silently and leave the element its inherited
    colour: 22 investment errors and negative gains lost their danger colour,
    and the danger/warning confirmation buttons drew foreground text on their
    own fill (1.6:1 dark) (#324). Use only `@theme inline` tokens; text on a
    danger or warning fill is `text-background`. Named check:
    `src/lib/theme-tokens.test.ts` scans every `.svelte`/`.ts` source.

53. **Sweep that audits the sign-in screen** — every Playwright test has its
    own browser context, so a spec that seeds in one test and visits routes in
    another is signed out and silently runs axe on the entry screen. Sign in
    per test and assert the visited pathname before checking
    (`route-sweep.spec.ts` `settle`).

54. **Hand-rolled modal overlay** — a `fixed inset-0` div with `aria-modal`
    hides nothing from the keyboard: focus stayed on the trigger, Tab walked
    the blurred page behind, Escape did nothing and Cancel dropped focus on
    `<body>` (#327). Use `ModalDialog`; a native `<dialog>` alone still lets
    Tab leave for the browser chrome, which the component wraps. A window
    `keydown` Escape handler also fires under a nested dialog — let the
    dialog's `cancel` own Escape. A panel that covers the page only below
    `lg` attaches `phoneModal` (`modal-focus.ts`) instead (#342); a row that
    opens it on Enter must `preventDefault`, or the key activates the panel
    control that just took focus. Named checks: `dialog-focus.spec.ts`,
    `panel-focus.spec.ts`.

55. **Layout and hover states no sweep looked at** — Categories scrolled
    118px sideways at 1280px once its `xl:` toolbar row applied, because the
    route sweep only measured 390/640px (#340). `hover:opacity-90` on a
    warning fill read 4.2:1 (#341): contrast tests covered the resting state
    only. Measure overflow at desktop too, and pin hovered pairs. Interactive
    sub-views (dialogs, forms, panels, import review) are swept by
    `subview-sweep.spec.ts`; add new ones to its list.

56. **Empty data bypassing a native parser negative control** — native text
    parsers may treat embedded NUL as end-of-input even when Go strings and
    SQL bindings retain it. Transaction search rejects NUL before quoting its
    literal FTS phrase (#355). Exercise parser refusals against a **populated**
    index: an empty FTS table returned 200 without reaching the failing parser.
    Named regression: `TestNativeSQLiteAuditSearchRejectsNULBeforeFTS`.

57. **CSP-blocked browser measurement** — a screenshot helper's inline
    stylesheet passed isolated controls but production `style-src 'self'`
    blocked it, leaving no measured glyphs (#361). Run readable and deliberate
    low-contrast controls under the real policy. The rendered-contrast helper
    uses CSSOM property writes and restores the original styles; keep its
    4.5:1 threshold and text-only border/icon controls. Named cases:
    `rendered-contrast.spec.ts` and the real `subview-sweep.spec.ts`.

58. **Generated types mistaken for runtime schema evidence** — OpenAPI 3.1
    ignored `nullable: true`, setup omitted implemented-step enum members,
    posting tags emitted null arrays, and spending disclosed stale role aliases
    (#367–#369). Use explicit null unions, normalize empty response collections,
    and check actual HTTP JSON against resolved schemas. The bounded
    `scripts/audit-api-contract.py` audit retains #350's exact status gap;
    #370's enrichment exception is removed after its repair. Generation alone
    cannot prove requiredness, enum or nullability conformity.

59. **Mutation journals bypassing display enrichment** — investment and
    reconciliation responses converted raw records directly, losing account
    class, names, commodity codes and symbols (#370). Use the shared owner for
    singles and a two-lookup batch for compound results. Compare real HTTP
    mutation postings with canonical GETs, including journal-free/null results.
    Inject display read failures after COMMIT and preserve the saved operation
    exactly once. Internal import receipts need IDs, not display metadata;
    leave their commit outcome independent from optional enrichment reads.

60. **Test harness exception disclosure** — a localhost provider stub returned
    parser input excerpts and implementation diagnostics through `String(err)`
    (#373, CodeQL alert #84). Keep caught exceptions out of HTTP responses,
    including development helpers. `pnpm --dir e2e run test:stubs` exercises
    malformed JSON, account encoding and provider transformation failures;
    both shared E2E wrappers run it before browser journeys.

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

## Provider-independent delivery (ADR 0017)

Review core/manual acceptance separately from verified provider automation.
Missing API data, credentials or live samples block only the adapter mapping.
Check explicit partial attribution, affected report coverage, manual repair and
reimport without duplicate effects or lost owner corrections. A real app-owned
recovery gap stays actionable; do not call it implemented through a policy change.
Use deterministic core fixtures without live provider access; synthetic examples
do not establish unverified wire semantics. Keep the roadmap and issue dependency
claims consistent with this boundary.

## Definition of done for a slice

1. App still runnable end-to-end (`pnpm dev` or `pnpm build`).
2. Narrowest relevant validation green; new behavior has named tests.
3. OpenAPI + generated types in sync (if API touched).
4. Docs updated per the table above.
5. No violation of `ledger-invariants` (if money/ledger touched).
6. Focused conventional commit.


## Observation selection regression (#203)

Valuation consumers use the shared `db` observation-selection owner
(`observation_selection.go`, `live_raw_v1`). Never choose an adjusted or future
quote by sorting all evidence by recording time. Keep same-date manual/override,
exact and approximate priority (including the existing unadjusted
`trade_implied` / `not_applicable` fill marker), the valuation/knowledge boundary and each
consumer's explicit age rule. A live timestamp cutoff does not reconstruct
past retirements or book state. Validate through real holdings and reporting FX
(`TestObservationSelection*`), preserving original per-currency amounts.
