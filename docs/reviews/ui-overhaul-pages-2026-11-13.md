# Import views and Ledger Overview — 2026-11-13

Delivery for #447 and #448, based on upstream `ad3a2968`, in an isolated
worktree. The accepted [navigation design](../design/home-and-navigation.md)
governs scope. No migrations or financial writers change.

## Behavior

Import has URL-selected Files, Connections, Rules and profiles, and Dividends.
The Files working copy survives view changes; history and `batch_id` restore
all staged-row pages. Both old dividend paths retain batch/query/fragment state
when redirecting. Selection generations reject a response arriving after the
owner clears or changes their selected batch. Existing payee resolution,
matching, commit, partial-commit and correction commands remain the owners.

An active write keeps its batch identity through preview, acknowledgement and
commit. The browser case holds the real preview request, attempts a batch
change, switches to Connections and verifies that a second import is disabled,
then requires the only commit to target the original batch. Starting a new
batch clears old working-copy reviews and retires old reads and polling.

The attention read reuses the dividend queue's SQL membership for its row
count. Actionable batches, including historical dividend repairs, still count
once in the header; the row count is never added to that total.

Ledger Overview replaces the Accounts tab. The account index consumes every
cursor and groups ordinary accounts by institution/country. Its composed
read includes current account metadata, source labels, exact financial
quantities and explicit valuation coverage. Financial inputs, including
unsettled investment cash, come from one snapshot per response. A brokerage
owns its cash children and holdings in this projection, without moving any
ledger records. Its page shows cash, links to cash registers, positions and
instrument links; instruments link back. Holding registers remain addressable
and investment commands retain their holding selectors. Existing income,
equity and system-register links retain their reference metadata when outside
the account-index valuation scope.

Institution add/edit/archive/restore reuses existing commands and their
in-use refusal. Configured settlement connections and committed file/profile
imports supply source labels; a label is evidence of that configuration or
import, never a provider-coverage promise.

## Financial controls

The service fixture holds €1,000 directly in a brokerage and €500 in a cash
child, then spends €200 on two OTHER units. A second holding contains ten TEST
units. Both holdings are hidden from the account index. Without usable prices,
the brokerage value is unavailable. Pricing TEST at €60 and OTHER at €120 gives
€1,300 cash + €600 + €240 = €2,140, exactly the Net worth report for the same
scope/date. The detail and overview responses agree. A 120-account fixture
exercises continuation and identity uniqueness; the existing 2,100-account
browser fixture still reaches and prefills its final account.

Net worth's rate table, seven-day window, unsettled recognition and
unclassified-negative-position rules are reused. No separate frontend
financial calculation, guessed price, quantity rounding or ledger mutation is
introduced.

## Validation

- Focused service/API checks cover brokerage composition, missing prices,
  Net worth equality, pagination, search exclusion, HTTP errors and the
  dividend count before and after its real review command.
- The frontend gate checks catalogs, regenerates OpenAPI/Paraglide, runs
  Svelte diagnostics and 1,190 unit tests.
- Integrated `pnpm build` validates static embedding and builds the one Go
  binary used by browser checks.
- The targeted browser run covers Import view history/reload, legacy dividend
  links, late response rejection, CSV mapping/rules/commit, dividend review,
  gain review, short linking, old route/filter history, large catalogs,
  account management, institution archive/restore and the phone brokerage →
  instrument → brokerage → entry journey.

The first browser run exposed a query reading editor state before its
initialization; state now precedes query construction. Existing connection
journeys now open Connections explicitly. Redirect assertions retain the
original parameter order before the new view selector. An overlapping full
backend/frontend/browser run exceeded the backend's five-minute app deadline
and the frontend token scan's five-second deadline. Validation is rerun in
isolation without changing either limit. The isolated backend gate passes:
app 208.397 s, API 124.956 s, command package 56.365 s; vet and formatting pass
through the same wrapper. The isolated frontend gate passes all 1,190 tests.
The 250,000-posting resource gate passes (standalone run):

| Read | Wall | Peak additional heap |
|---|---:|---:|
| Bundle export | 10.797 s | 16 MiB |
| Self-check | 9.909 s | 40 MiB |
| Attention | 2.270 s | 3 MiB |
| Home | 3.627 s | 118 MiB |
| Ledger Overview first page | 3.076 s | 93 MiB |
| Investment entry accounts | 0.038 s | 5 MiB |

Ledger's first-page ceilings are 10 seconds / 256 MiB. This is a bounded
synthetic read measurement, not a full UI latency or race-suite speedup claim.
The final-build targeted run passes 22 existing cases. The first broad audit
finds a Russian profile-selector width defect, fixed with shrinkable fieldsets
and constrained controls. Two audit cases encounter an OS temporary-file quota
failure; their reruns move browser scratch files onto the larger filesystem.
All nine rerun audits pass: the full route phone/language sweep in every locale
and the eight focused subview locale/theme/width configurations. The initial
broad run also passes the other five route sweeps, including data errors,
masking, keyboard help, document titles and both-theme layout checks.
The final Import/Ledger/route run passes all eight cases, including the held
commit-preview request and batch-selection guard. Its test fixture registers
its provider key and uses the actual accessible control names. A focused
owned-book-only invocation cannot create the existing shared test-state root;
the final run includes the route suite that initializes it. No runtime limits
or assertions are relaxed.

## Retained boundaries

Pins, the object switcher and object hierarchy remain #442, with #449's owner
book evidence. Entered asset values remain #454; this delivery reuses current
Net worth capabilities. New CSV mappings still require analysed file headers.
Translation catalog parity does not replace #102's native terminology review.
No push, PR, publication, live-provider verification or owner-book access is
part of this delivery.
