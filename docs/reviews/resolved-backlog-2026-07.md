# Resolved Backlog Record — 2026-07

This is the historical record removed from the live `docs/backlog.md` during the
2026-07-12 documentation consolidation. The detailed original writeups are
preserved verbatim in `docs/reviews/backlog-pre-consolidation-2026-07-12.md`.
`docs/implemented.md`, tests, and the linked design records are the source for
current behaviour.

## Resolved

- **Security and operations:** S-01 setup ownership claim; S-02 slow-client
  protection; S-03 trusted forwarded-client parsing; S-05 external TLS posture;
  S-08 SQLite/WAL/SHM permissions; T-01 configurable session lifetime; T-18
  expired-session cleanup; T-19 secret-key loss/rotation guidance.
- **Import correctness and contracts:** T-05 pagination consumption; T-06
  atomic generic import commit; T-07 import OpenAPI coverage; T-08 encrypted
  connection secrets; T-11 real Trading 212 key probe; T-12 retained connection
  provenance; T-14 paginated-history continuation; T-15 retry re-staging signal;
  T-16 atomic start guard; T-17 same-timestamp cursor safety; T-21 verified
  provider endpoint/enums; T-22 import entry-kind validation; T-23 complete
  preview pagination; T-26 atomic investment-import commit; T-28 intraday fill
  ordering; T-29 unused routing-setup compensation; T-30/T-33 settlement-account
  eligibility; T-31 provider provenance; T-32 concurrent commit protection.
- **Product validation and UI coverage:** T-20 critical E2E expansion; T-24
  reconciliation guard for investment postings; T-25 pricing-history results
  signal; B-T212-SCHED scheduled refresh.

## Resolved later (recorded here to keep one resolution record)

- **T-35 QIF EU date misparse** and **T-36 decimal-comma amounts 100× off**,
  both fixed 2026-08-06. New `backend/internal/app/import_locale.go` owns the
  locale decisions for file imports: an import profile's `date_layout` and
  `decimal_separator` win when set, and otherwise the QIF adapter resolves the
  date field order across the whole file (one row with a day above 12 settles
  it) and the decimal separator per amount. Staged rows now carry ISO dates and
  canonical amounts; the raw fields keep the file's own spelling. A file whose
  dates are all ambiguous, or whose rows disagree, parses as `MM/DD` with a
  parse warning telling the user to set a profile layout. Covered by
  `import_locale_test.go` and the EU cases in `import_qif_test.go`.

- **T-41 scaled-integer arithmetic consolidated into `exact`**, done 2026-08-06.
  The two near-identical private helpers (`scaledAmount` in `app`,
  `scaledInteger` in `db`) and the duplicate `pow10`/`pow10DB` tables are gone,
  replaced by `exact.ScaledInt` and `exact.Pow10` in
  `backend/internal/exact/scaled.go`. `ScaledInt` carries
  add/sub/align/compare/truncate plus overflow-checked `Int64()` (returns
  `exact.ErrInt64Range`) and `Coefficient()`. Migrated call sites: ledger
  balances and register running balances, reconciliation totals, `PreviewSell`
  gain, and the `db` investment aggregations — `PositionsWithGains` and
  `ListRealizedGains`, whose hand-rolled basis→proceeds scale alignment is now
  `TruncatedTo`. The lot-selection loop in `disposeLotsTx` keeps its explicit
  `exact.Pow10` alignment because its exactness check (`QuoRem` remainder at the
  lot's own scale) has no `ScaledInt` equivalent. Behaviour is unchanged; tests
  moved to `internal/exact/scaled_test.go` and were extended to cover `Cmp`,
  `TruncatedTo`, `SubScaled`, operand immutability, and int64 overflow. The
  ledger-invariants skill now mandates the helper.

- **T-39 background work no longer retries forever**, fixed 2026-08-06. FX
  coverage work now gives up after `maxFXCoverageAttempts` (8) failed attempts
  and moves to the existing `failed` status instead of retrying against the 6h
  backoff cap indefinitely — matching what the Trading 212 fetch worker already
  did. Because a bounded cap is only safe if the work is recoverable, three
  things landed together: `BackgroundWorkRepository.ListBackgroundWorkByStatus`
  surfaces given-up items through `PricingService.FailedBackgroundWork`, which
  the source-health and currency-settings page responses now carry as
  `failed_background_work`; `RequeueBackgroundWork` moves a failed item back to
  pending with `attempts` reset, refusing the transition with
  `ErrBackgroundWorkAlreadyActive` when an equivalent item is already live (the
  active-unique index allows one); and
  `POST /api/v1/pricing/background-work/{work_id}/retry` plus a re-enqueue
  button on the currency settings page make that reachable from the UI. Covered
  by `TestProcessFXCoverageWork_GivesUpAfterMaxAttempts` and the two
  `TestRetryBackgroundWork_*` cases.

- **T-40 `triangulation_max_hops` is now honored**, fixed 2026-08-06. The
  policy value used to be a bare on/off check (`<= 0` disabled derivation) with
  the derived path hard-coded to exactly one intermediate currency, so a
  configured 2 silently behaved as 1. `fetchAndStoreDerivedFX` now resolves a
  chain of arbitrary length: `findFXPath` deepens one hop at a time and returns
  the first chain it can complete, so the **shortest** available route always
  wins — each extra leg compounds rounding and widens the vintage spread across
  source observations. Every leg fetch is memoized, so deepening re-walks
  shallower depths without extra provider calls, and the one-hop search order is
  unchanged from before. Storage generalized with it: `scaledFXProduct` takes N
  observations and stays a single exact `big.Int` quotient (no intermediate leg
  product is ever rounded), the derivation metadata records every leg and an
  N-factor formula, `via_currency_code` became `via_currency_codes`, and the
  chain is dated by its stalest leg. The derived observation's dedupe key keeps
  its single-hop spelling, so pre-existing derived rows still match. Covered by
  `TestRefreshFXTarget_DerivesMultiHopChainWhenPolicyAllowsMoreHops`,
  `TestRefreshFXTarget_PrefersShorterChainOverDeeperOneFoundFirst` (verified to
  fail against a plain depth-first search), and
  `TestRefreshFXTarget_ZeroHopsDisablesTriangulationEntirely`.

- **T-37 price observations can now be voided**, fixed 2026-08-06 (audit P3,
  R16 slice 1). `price_observations.voided_at` was written by nothing;
  `POST /api/v1/pricing/prices/{price_id}/void` now retires an observation
  with a **required** reason, so a poisoned rate stops feeding valuations
  while the row itself survives — prices are corrected by superseding or
  voiding, never edited or deleted. Three decisions are worth recording.
  (1) **The void cascades.** A triangulated rate is a separate observation
  carrying the same poisoned number under a new id, so
  `VoidPriceObservation` walks `derivation_json`'s `legs[].observation_id`
  (SQLite `json_each`, breadth-first, bounded by `maxVoidCascadeDepth`) and
  retires every rate derived from the voided one, tagging their reason with
  the originating id. Voiding only the leg would have left the bad number
  live. (2) **Not idempotent** — a second void would overwrite the first
  reason, so it returns `ErrPriceObservationAlreadyVoided` → 409. (3) **No
  reconciliation guard is involved**: observations carry no postings, so a
  void cannot change a reconciled balance; it changes reported market values
  only. Migration `0002` adds `voided_audit_event_id` so the one
  `audit_events` row per void is referenced from every row it retired, and a
  partial index over voided rows. `GET /pricing/prices` gained
  `include_voided=true` so what was retired stays inspectable. Covered by the
  five `TestVoidPrice_*` cases in `app/pricing_test.go` — including
  `..._CascadesToRatesTriangulatedFromTheVoidedLeg` and
  `..._StopsTheObservationBeingUsedForValuation` — plus `TestVoidPrice_HTTP`.
  The R11 pricing UI still owns the operator-facing surface.

- **T-38 zero-proceeds disposals are now possible**, fixed 2026-08-06 (audit
  P4, R16 slice 1). `POST /api/v1/investments/write-off` and its `/preview`
  sibling record a total loss — fund closure, worthless delisting, liquidated
  issuer — over the existing lot-disposal engine. Before this, the shares
  stayed in open lots forever and the loss never reached realized gains.

  The backlog framed this as "either `CashAmountValue == 0` support on Sell
  or a dedicated write-off endpoint". **A dedicated endpoint was chosen**: a
  zero on a sale is indistinguishable from a typo, and making the most
  destructive investment operation reachable by clearing one field is a trap.
  `InvestmentWriteOffInput` therefore has no cash account, cash commodity or
  cash amount at all, and carries a **required reason** instead.

  The open "which account does the loss book against" decision resolved to
  **none, deliberately**. The postings are the sale's two commodity legs with
  the cash legs absent (holding credit, `commodity_trading` debit), so the
  transaction balances per commodity at genuinely zero proceeds and the
  existing `realizedGainCashProceeds` join reads proceeds 0 against the
  disposed basis with no change. Booking the basis to an expense account
  would have been worse than cosmetic: that query counts *any* positive
  non-trading posting in the cost commodity as proceeds, so an expense debit
  would have reported the realized gain as **zero** instead of a full loss.
  It would also have pre-empted open decision **I-04** (whether realized
  gains/losses become ledger postings at all) for one transaction kind only.
  When I-04 lands, a write-off needs no special case — it is already an
  ordinary disposal with no cash leg.

  Two smaller decisions: no trade-implied price is recorded (a zero price
  violates the positive-price trigger, and a near-zero stand-in would poison
  the instrument's history), and `validateWriteOffInput` is deliberately
  separate from `validateTradeInput` rather than a relaxation of it. Covered
  by five `TestWriteOff_*` service cases — including
  `..._RealizesTheWholeBasisAsALoss`, which asserts the loss reaches
  `ListRealizedGains`, and `..._DoesNotRecordAZeroTradeImpliedPrice` — plus
  `TestWriteOffInvestment_PreviewAndCommitCloseThePositionAtALoss` at the
  HTTP layer. No UI yet; entry belongs with the R16 investment-lifecycle
  surface.

- **S-07 authentication events are now visible**, done 2026-08-06. There was
  no record at all of who signed in, who failed, or from where — an operator
  could not tell a brute-force run from a forgotten password, and had nothing
  to reconstruct an incident from. New `authentication_events` table
  (migration `0003`) records `login_succeeded`, `login_failed`,
  `login_blocked` and `logout` with the **proxy-aware** client IP (the one
  `loginClientIP` resolves through the trusted-proxy allowlist, not the raw
  peer address), attempted username, failure reason, request id, and the
  session id where one exists.

  Privacy posture is deliberate and tested: no password material, no session
  token, not even the token hash — the session id is enough to correlate with
  `auth_sessions`. `TestAuthenticationEvents_NeverStorePasswordOrSessionToken
  Material` asserts it against the real rows. Rows are pruned to a 90-day
  window by `pruneAuthenticationEvents`, riding the existing daily
  session-cleanup tick rather than adding a second timer; this is an
  incident-response log, not a permanent sign-in archive.

  Two consumption paths, because "durable **or** operator-consumable" is
  weaker than the operator needs: `GET /api/v1/auth/events` (owner session
  required — the log names IPs and attempted usernames) returns the recent
  list plus `failed_last_24h`, the number that makes a run visible without
  scanning; and every event is mirrored to structured `slog`, failures at
  `WARN`, so a log shipper can alert without querying SQLite.

  `recordAuthEvent` deliberately swallows its write error after logging it:
  failing a *successful* login because its audit row could not be written
  would turn a logging fault into a lockout. `login_blocked` is a separate
  event type from `login_failed` because a throttled attempt never reaches
  password verification and is a different operational fact. `CreateSession`
  now returns the new session id, and `Logout` takes a `LogoutInput` so it
  can record the client IP and resolve the user before revoking. Documented
  for operators in `docs/deployment-security.md` (§ Monitoring
  authentication). No UI yet.

- **S-04 the login throttle is now lockout-safe**, done 2026-08-06. The
  throttle blocked on a username scope, and a single-owner app publishes its
  owner's username by construction — so the throttle was a remote lockout
  switch. Any internet attacker could fail five logins every fifteen minutes
  and keep the real owner out of their own finances indefinitely. The
  protection *was* the denial of service.

  Fixed with approved devices (`login_trusted_devices`, migration `0004`). A
  device that completes a successful login — or first-run owner setup, so a
  fresh install is never briefly vulnerable before earning one — receives a
  random token as an HttpOnly, SameSite=Strict cookie, hashed at rest exactly
  like a session token. `loginThrottleScopesForAttempt` then puts that
  device's attempts on a `trusted_device` scope keyed by device id, instead
  of the shared `username` and `client_ip` scopes. An attacker filling the
  username scope no longer touches the owner's budget.

  The security properties that make this safe, each with a named test:

  * **The cookie is not a credential.** It grants no access; a login still
    needs the password, and presenting it alone authenticates nothing
    (`TestSetupAndLoginIssueTrustedDeviceCookie_HTTP` asserts the session
    endpoint stays unauthenticated with only the device cookie).
  * **The bypass is opt-in by proof, not by claim.** An absent, garbage,
    expired or revoked token changes nothing —
    `TestLogin_UnapprovedDeviceKeepsTheOriginalUsernameAndIPThrottle` and
    `TestLogin_ExpiredOrRevokedDeviceGrantsNoBypass`.
  * **Devices are user-bound**, so an approval for one account can never lend
    its budget to guesses against another
    (`TestLogin_DeviceApprovedForAnotherUserGrantsNoBypass`).
  * **A stolen cookie buys no extra guesses.** The device scope keeps the
    same 5-in-15 budget; the bypass isolates the blast radius rather than
    removing the limit
    (`TestLogin_ApprovedDeviceStillHasItsOwnFailureBudget`).

  Approval lapses after 180 days unused and slides forward on each successful
  login; expired and revoked rows are pruned on the same daily tick as
  sessions and authentication events. `GET`/`DELETE
  /api/v1/auth/trusted-devices` let the owner review and revoke, with the
  current device flagged so it is not revoked by accident. The IP scope is
  still never cleared on success, preserving the original NAT reasoning.
  Documented in `docs/deployment-security.md`. No UI yet. **MFA (S-06)
  remains the one open public-deployment gate.**

- **T-42 a commodity's enable date no longer blocks earlier history**, fixed
  2026-08-06. Posting validation resolves the commodity version *as of the
  entry date*, and a commodity's first `commodity_versions` row was effective
  from the day it was created — so every transaction dated before setup was
  rejected. That collided head-on with the announcement's centerpiece: install
  today, enable EUR today, import several years of history, and every row
  fails.

  **Decision: option (a), backdate the first version.** `commodity_versions`
  is write-once in practice (one `INSERT` path, no archive or update service
  method), so the as-of lookup never actually chose between versions — its
  only observable effect was to reject earlier dates. `db.CommodityGenesisDate`
  ("0001-01-01") is now stamped on every commodity's first version, mirroring
  `app.systemAccountDate`, which already makes the same call for system
  accounts and seeded categories. A genuine later change (archive, rename,
  scale) is a *new* version with a real effective date, which as-of resolution
  still honours.

  Applied at the single writer rather than passed in by callers, deliberately.
  Three production paths reach `createCurrency` — including account creation,
  which was passing the *account's* date as the currency's — and the Trading
  212 import had already had to work around the old default twice. A default
  that every producer of historical data must remember to override is the bug,
  so it is now impossible to get wrong.

  The redundant half of that import workaround is gone:
  `ResolveOrCreateInstrumentForImport` no longer takes an `effectiveFrom`. The
  holding-account backdating stayed at the time — `opened_on` = the trade's
  date was judged correct behaviour rather than a workaround; T-44 below
  overturned that a day later. The commodity-side fallback added earlier the
  same day (`CommodityExists` → "posting date is before the commodity was
  enabled") is also gone: with genesis dating it became unreachable, and
  unreachable checks are what caused this investigation in the first place.
  The account-side fallback stays, because accounts legitimately open later.

  Two test fixtures were seeding commodities and the `commodity_trading`
  system account at a 2026 date via raw SQL, which is not what production
  does; both now use the genesis date, so back-dated import journeys are
  reachable in tests at all. Covered by
  `TestCreateTransaction_HistoryPredatingCurrencySetupIsAccepted` and
  `TestCommitImportBatch_BackdatedFirstTradeNeedsNoInstrumentBackdating`, both
  confirmed to fail against the old default.

  Both follow-ups it filed — T-43 and T-44 — were fixed 2026-08-07; see below.

- **T-43 user-created categories no longer open today**, fixed 2026-08-07.
  Seeded categories were stamped `0001-01-01` but a category the user created
  got today, so it could not take an imported transaction from last year.

  **Decision: categories join system accounts and seeded categories on the
  genesis date**, and creation no longer accepts a date at all. A category is a
  classification bucket, not something you open — `app.categoryGenesisDate` is
  now stamped on every category's `opened_on` and first-version
  `effective_from`, seeded or user-created. `opened_on` was removed from
  `CreateCategoryRequest` and `UpdateCategoryRequest`, and `effective_from`
  from create (a first version has no other honest date); an *edit* still
  carries a real `effective_from`, because a genuine change does happen on a
  date. The category editor's "Opened on" field is gone with it. Asset and
  liability accounts are deliberately untouched: there `opened_on` is a real
  financial fact. Covered by
  `TestUserCreatedCategoryOpensAtGenesisSoItTakesEarlierHistory`, confirmed to
  fail against the old default, plus rejection cases in
  `TestCategoryValidationRejectsParentMismatchCycleAndAccountFields`.
  Documented in `docs/design/categories-design.md`.

- **T-44 a later import carrying an earlier trade no longer fails on the
  holding account**, fixed 2026-08-07. The importer stamped a new holding
  account's `opened_on` with the date of whichever fill it saw first and never
  revisited it, so a backfill or later sync carrying an *earlier* trade failed
  `posting date is before account opened date` with no repair available.

  **Decision: import-created holding accounts open at the genesis date** —
  design note, and the two rejected options (widening `opened_on` backwards;
  rejecting the row with an actionable message), in
  `docs/design/holding-account-opened-date.md`. Same reasoning as T-42 and
  T-43: the date is app bookkeeping about a container the importer
  materializes, not a financial fact, and the lock on structural fields stays
  intact because nothing is ever changed. **Hand-created holding accounts are
  deliberately unchanged** and still take a user-supplied `opened_on`. Covered
  by `TestCommitImportBatch_LaterImportCarryingAnEarlierTradeStillCommits`,
  confirmed to fail against the old behaviour.

- **S-06 multi-factor authentication**, shipped 2026-08-07 — the last public
  deployment gate. **Decision: TOTP (RFC 6238) plus ten single-use recovery
  codes**, chosen over WebAuthn because this app is single-owner and routinely
  reached at a LAN address or a private hostname, where WebAuthn's origin
  binding is awkward and the recovery story for an owner who loses their only
  authenticator is worse. An authenticator app works everywhere the app does.

  The algorithm is a truncated HMAC, so `internal/totp/` implements it in ~150
  dependency-free lines against the RFC 6238 test vectors rather than adding a
  library to the authentication path. Migration 0005 adds `user_mfa_totp`,
  `user_mfa_recovery_codes`, and `login_mfa_challenges`.

  Shape of the login change: once MFA is active a verified password no longer
  produces a session, a device approval, or a throttle reset. It produces a
  five-minute single-use challenge — a durable row, carried by an HttpOnly
  cookie so no token is handled by page scripts — which `POST
  /api/v1/auth/login/mfa` exchanges for a session on a valid code. Both paths
  issue sessions through one `completeLogin`, so the second factor cannot
  quietly skip a step the password path does.

  The details that make it real rather than decorative, each with a named test:
  wrong codes spend the same 5-in-15 throttle budget as wrong passwords; a code
  cannot be replayed inside its own 30-second step (atomic counter advance in
  the `UPDATE`'s `WHERE`); a challenge is single-use; a *pending* enrolment
  never gates a login, so an abandoned setup cannot lock the owner out;
  enrolling, disabling, and regenerating recovery codes each re-confirm the
  password, so a stolen session cannot change what protects the account; and
  the shared secret is sealed with `REKENRAAM_SECRET_KEY` (absent ⇒ enrolment
  refused, never stored in the clear).

  One deliberate asymmetry: recovery codes are SHA-256 hashed rather than
  sealed, and the verification path falls through to them when the secret
  cannot be opened. Losing the key must not lock the owner out of their own
  finances — that is exactly the situation recovery codes exist for.

  UI shipped with it: Settings → Security for enrol/activate/disable/regenerate
  (secret and codes shown once), and the code step in the install-gate login.
  `docs/deployment-security.md` now states the operator requirement — the gate
  is no longer "MFA does not exist" but "the owner account must be enrolled".

- **G-07 no coverage signal in CI**, closed 2026-08-07 — the last open item of
  `docs/plans/backend-test-coverage-plan.md` (Workstream 7) and of the
  2026-07-07 test-coverage review's CI gaps. `scripts/test-backend.sh` gained a
  `COVERAGE=1` mode; the default run stays `-race`, because a combined run is
  slow and muddies both signals. A separate `backend-coverage` CI job runs it,
  uploads `coverage.out`, echoes the merged total into the job summary, and
  calls `scripts/check-coverage-floor.sh`, which fails below `COVERAGE_FLOOR`
  (default **73.0%**, roughly two points under the 2026-08-07 merged total of
  **75.2%**). **Decision: a floor, not a target** — it is a tripwire against
  erosion, raised deliberately when the level rises and never lowered to make a
  change pass. The job is intentionally outside `app-build`'s `needs`, so a
  breach is loud without blocking the release build.

- **T-45 decimal-comma amounts 100× off in the browser**, found and fixed
  2026-08-08 while extracting G-02's shared money module. The frontend twin of
  T-36, which had only ever been fixed on the file-import side. Both frontend
  amount parsers — the private `parseDisplayAmount` in
  `transactions/transaction-editor.svelte` and `parseStatementBalance` in
  `reconcile/statement-balance.ts` — opened with
  `input.trim().replace(/,/g, '')`, stripping *every* comma. That is correct
  for the `en` thousands separator (`1,234.56` → 1234.56) and silently wrong
  for decimal-comma input: typing `1,50` posted **150.00**, with no warning,
  straight into the ledger.

  Fixed in the new `frontend/src/lib/money/amount.ts` by validating the shape
  before stripping: the integer part must be either plain digits or correctly
  grouped in threes (`/^-?(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$/`). Well-formed
  grouping still parses; `1,50`, `12,3`, `1,2345` and friends now return `null`
  and the form rejects them. **Deliberately a rejection, not a guess** — `1,234`
  is genuinely ambiguous between the two conventions, and inferring per value is
  exactly how T-36 happened. Resolving the separator from the active locale is
  tracked as G-08. Proven by the `decimal-comma input` cases in
  `frontend/src/lib/money/amount.test.ts`.

- **T-46 an unparseable split leg posted as zero**, found and fixed 2026-08-08,
  same extraction. `buildSplitJournalEntries` in
  `transactions/transaction-editor.svelte` mapped each filled-in leg through the
  parser and wrote `quantity_value: value ?? '0'`, so a leg the parser rejected
  became a **zero posting** rather than an error. The client-side balance hint
  hid it rather than catching it: `computeSplitImbalance` *skipped* unparseable
  legs, so the form showed "balanced" while submitting a leg the user had filled
  in as 0.00. Reachable through T-45 above — `1,50` in a split leg parsed fine
  and was wrong; anything the regex now rejects would have posted zero.

  Fixed by refusing to build the payload at all when a filled-in leg fails to
  parse, which is what the simple tier already did. `commodityImbalance` still
  skips unparseable legs and now says so in its contract: a leg it ignores is
  not a leg the backend ignores, so callers must reject separately.

- **P5 drafts vs FX coverage** (2026-07-19 audit), closed 2026-08-07 as a
  **documentation** fix, not a code change. The audit was right that the docs
  and the code disagreed and wrong about which was at fault: posted-only
  coverage is the intended behavior (a promoted draft gets coverage at
  promotion, and nothing today produces drafts at all). The
  `ledger-invariants` skill had already been corrected; `docs/conventions.md`
  still carried the old "a draft may trigger FX coverage" wording and now
  states the posted-only rule, leaving room for a future draft producer that
  needs converted amounts on its own review surface. This also closes item 2 of
  `roadmap-review-2026-07-19.md` §2, the last unapplied documentation-accuracy
  fix from that review.

## Deliberate non-work

- T-02 single runtime book ID, T-03 CSRF-token rotation, and T-04 CSP
  `unsafe-inline` are recorded design choices with their rationale in the prior
  backlog history and governing security/architecture documents.
- T-13 file-import-wrapper service coverage and B-T212-INVST linking an existing
  manually tracked holding account are deferred design follow-ups, not current
  defects. Revisit only when their respective product work is scheduled.

## Moved product decisions

I-03 (multi-method analytical gains) and I-04 (posting realized gains) are not
technical debt. They are deferred accounting/reporting product decisions and now
live in the open decisions section of `docs/roadmap.md`.
