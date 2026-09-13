# Ledger and investments: third release review — 2026-09-13

Reviewed `2904cf7e`, including the other agent's implementation summary and the
new role, chronology, valuation, and transaction-version tests. Also inspected
the upgraded migration-fixture coverage from `1e52eb71`.

## Result

**Two confirmed P1 integrity gaps remain; hold the release.** The latest fixes
close the preceding concrete cases. This pass does not reopen the average-cost
chronology or valuation findings: focused tests pass, and the former fractional
sale now returns exactly 99,999.9999 EUR of market value.

The remaining failures concern the provenance of prepared inputs: one command
reads a draft twice, and account-role validation is not coupled to the eventual
posting write. Both need overlapping operations, but neither needs multiple
app processes, direct SQL tampering, or a Go memory race.

## P1 / T-94 — promotion loses the version from its first read

Locations: `backend/internal/app/transactions_write.go:240`, `:256`, `:193`.

`PostTransaction` reads the draft and constructs its spec, then calls
`UpdateTransaction`. That method reads the transaction again, derives promotion
checkpoint candidates from the second record, and supplies the second version
as `ExpectedVersionID`. The old spec is not bound to its actual source version.

Confirmed deterministic interleaving:

1. Post 10 EUR on January 1 and reconcile to a January 31 statement for 10 EUR.
2. A system producer creates a 5 EUR draft dated January 2. This is permitted:
   drafts are outside the ledger.
3. Promotion captures that draft's January spec (the first `PostTransaction`
   read).
4. Another request edits the draft to February 1.
5. Promotion enters `UpdateTransaction` with its captured January spec.

Observed: it posts January 2 successfully, without an override, and leaves the
January checkpoint active. The promotion candidates describe February, while
the actual new postings describe January. The repository's version check
passes because it receives the newer February version.

The probe invokes the exact second service phase after the intervening edit;
it does not call the repository with invented candidates. This is the gap
*before* the prepare/commit interval already covered by
`TestStaleDraftPromotionIsRefused`. That existing test correctly proves its
case, but cannot catch a version replaced between the two service reads.

Required correction: carry the first read's version through promotion and
refuse a changed draft, or use a single authoritative record to prepare the
promotion. Build candidates for the postings that will actually enter the
ledger. Preserve the new repository version check; the problem is which
version the command passes to it. Add a named service-level interleaving test
where the draft date/account changes between the two reads and assert both
rejection and an untouched checkpoint.

## P1 / T-100 — account roles can change after posting validation

Locations: `backend/internal/app/transactions_validate.go:356`, `:385`, and
`backend/internal/db/transactions_write.go:23`.

The generic posting fence and investment role checks resolve account facts
before `BeginTx`. The eventual write neither validates those facts again nor
checks which account version was used. The new `ExpectedVersionID` protects an
existing *transaction*, not the accounts on which a new transaction depends.

Confirmed through the actual account and transaction services:

1. Create a posting-enabled `other_asset` account with a security as its default
   commodity, effective January 1. Account creation succeeds through
   `AccountService.CreateAccount`; this does not rely on a seeded checking
   account lacking the default commodity required by real account creation.
2. Prepare a balanced ordinary entry for 10 security units in that account.
3. Before it commits, change the still-unused account to `security_holding`,
   effective January 1, through `AccountService.UpdateAccount`. The structural
   edit is allowed because there are no posted records yet.
4. Commit the prepared transaction.

Observed: transaction 1 commits and there are zero investment lots. A fresh
preparation of the identical entry is correctly rejected by the subledger
fence. The pending preparation therefore bypasses a rule enforced for the
account as it exists at commit.

Required correction: resolve/enforce posting eligibility, effective account
roles and commodity constraints inside the financial write, or carry and
validate the precise version dependencies there. Check the reciprocal race
where an account structural edit is prepared while empty and commits after its
first posting. Cover both generic and investment preparation, including
imports. No reciprocal-race failure is claimed here: that is a required sweep,
not a separately reproduced finding.

## What was verified successfully

- The baseline `./scripts/test-backend.sh` passed formatting, vet and race
  tests. Several package results were cached; this is not an uncached full-race
  claim.
- Fresh focused tests across `app`, `db`, `exact`, and `api` passed with
  `-count=1`: stale-version guards, normal reconciled metadata edits, backdated
  investment refusals, legal same-day ordering, invalid roles, valid trades,
  write-off previews, valuation fitting and exact normalization.
- An independent repetition of the earlier fractional-sale example passed and
  asserted the exact market value, not just a non-null field.
- `./scripts/test-frontend.sh`: zero Svelte errors/warnings; 25 files and
  **381 tests passed**.
- `pnpm test:release-preflight`: **7 passed, 0 skipped, 39 seconds**, including
  owner bootstrap, split transfer, reconciliation, QIF import, buy/sell and
  mobile entry. This is fresh evidence for the narrower release preflight;
  the other agent's 58-test full browser run was not repeated.
- No coverage percentage was recomputed in this pass. The prior coverage result
  is not presented as coverage of the latest revision.

The isolated probes initially needed access to Go's build cache outside the
sandbox; they were rerun successfully with that access. Browser preflight also
ran with local-server permissions. These were environment restrictions, not
application findings.

## Reproduction artifact

`fixtures/v0.1-ledger-investment-third-pass-probes.go.txt` contains two intended
failing safety assertions and one passing valuation control, run against an
isolated archive of `2904cf7e`. Account creation/update and draft creation/edit
use real services with an isolated migrated database.

To reproduce, first ensure the destination is absent:

```sh
cp docs/reviews/fixtures/v0.1-ledger-investment-third-pass-probes.go.txt backend/internal/app/release_third_pass_probe_test.go
(cd backend && go test ./internal/app -run '^TestThirdPass' -count=1 -v)
rm backend/internal/app/release_third_pass_probe_test.go
```

Expected on the reviewed revision: two failures, one pass, with setup succeeding
in all three cases. Keep unresolved probes outside the default suite; promote
them to permanent regression tests with the fixes. No production-code changes
or production-data changes are included in this review.
