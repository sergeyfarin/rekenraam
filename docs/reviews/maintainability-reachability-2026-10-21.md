# Maintainability: reachability and abstraction consumers — 2026-10-21

**Status: fifth slice of [#284](https://github.com/sergeyfarin/rekenraam/issues/284),
bounded dead-code and named-interface/options review delivered; overall audit
partial.** This slice records Go production/test reachability, frontend
file/export findings with explicit entry points, and consumers of named Go
interfaces/options. Every reported function/symbol and selected type has a
catalog disposition. No runtime code is deleted or refactored, no new defect is
established, and no additional independent issue is warranted by this evidence.

Source remains `13d15eabd68ed2d3b6177512c9c8fe56c528bbf0`; working HEAD was
`809613f3655c035b334b9083220eebbde5179afd`. The intervening slices changed audit
artifacts/docs only. This extends the [layer review](maintainability-layers-2026-10-21.md)
and [invariant matrix](maintainability-invariants-2026-10-21.md), with work order
still owned by the roadmap.

## Tools, roots and retained evidence

| Evidence | Method / coverage | Limit |
|---|---|---|
| Go [production](fixtures/maintainability-deadcode-production-2026-10-21.json) and [test-root](fixtures/maintainability-deadcode-tests-2026-10-21.json) reports | `golang.org/x/tools/cmd/deadcode@v0.51.0`, rapid type analysis, `./...`, with/without `-test`; module filter | Linux/amd64, CGO and `fts5,netgo,osusergo` only. `-test` adds test executables as roots; it does **not run tests**. Reachability is not correctness or a binary-size measurement |
| [Go disposition catalog](fixtures/maintainability-deadcode-triage-2026-10-21.csv) | All 63 distinct function findings across both views | No automatic removal permission; nonfunction declarations and other platforms need separate inspection |
| Knip [default](fixtures/maintainability-knip-default-2026-10-21.json), [explicit-context](fixtures/maintainability-knip-context-2026-10-21.json), [production](fixtures/maintainability-knip-production-2026-10-21.json) and [in-file-reference](fixtures/maintainability-knip-local-2026-10-21.json) reports | Knip 6.42.1; files, exports, types and unresolved imports; SvelteKit/Svelte plugins; route, test, build, HTML-script and shell-launched roots | An unused **export** can have a live implementation used in its own file. Svelte's plugin preserves imports/template dynamic imports, rather than analyzing all component-local execution. No all-locals or bundle-size claim |
| [Knip configuration](fixtures/maintainability-knip-config.json), [symbol catalog](fixtures/maintainability-knip-triage-2026-10-21.csv) | 228 distinct symbol findings across explicit normal/production views; all classified | Config must be copied into the disposable repository root. Generated frontend files are excluded from findings; frozen dependencies/generated resolution context are supplied |
| [Type-resolved Go index](fixtures/maintainability-go-abstractions-2026-10-21.csv), [tool](fixtures/maintainability-go-abstractions.go), [25-type catalog](fixtures/maintainability-abstraction-triage-2026-10-21.csv) | `go/packages`/`go/types` across normal/test package variants; 20 named interfaces and five named `Option(s)` structs; 546 rows | References/member-reference owners, not dynamic call counts. Generic type parameters are excluded. Anonymous interfaces, arbitrary callback options and all frontend abstraction costs are not exhaustively classified |
| [Valuation live path](fixtures/maintainability-valuation-reachability-2026-10-21.json) | One shortest production path to `db.valuePositions`, through saved-run composition and dated valuation | An example path, not the only consumer or an executed HTTP journey |
| [Controls](fixtures/maintainability-deadcode-controls.py), [results](fixtures/maintainability-deadcode-controls-2026-10-21.json), [provenance](fixtures/maintainability-deadcode-provenance-2026-10-21.json), [tool manifests](fixtures/maintainability-deadcode-tools/package.json) | Dynamic Go interface/generic reachability, test-root difference, named type selection, live route/import and deliberately unused TS/Svelte files/exports/type | Positive/negative scanner controls, not new app tests or CI gates |

The Go configuration loads **16 module packages and 309 runtime Go files**.
The baseline has 311 runtime Go files; `backup_space_other.go` and
`lockfile_other.go` are excluded by their non-Unix build constraints. No Windows
or other build configuration is certified. Go analysis tools were built with
Go 1.27.0 against the pinned tools graph; package loading uses the module's
Go 1.27.2 toolchain. The retained [Go module](fixtures/maintainability-deadcode-tools/go.mod)
and [checksums](fixtures/maintainability-deadcode-tools/go.sum), plus the isolated
[pnpm lock](fixtures/maintainability-deadcode-tools/pnpm-lock.yaml), keep audit
dependencies out of the application's manifests.

## Go reachability dispositions

| View / disposition | Functions | Meaning |
|---|---:|---|
| Unreachable from production roots | 61 | Includes test support and standalone lower-level wrappers |
| Of those, reachable with test roots | 55 | Removing them requires changing test source/coverage, not just deleting allegedly dead production code |
| Unreachable runtime functions in both views | 6 | Source-hygiene inventory below; not a measured change-cost/performance candidate |
| Additional unreachable test helpers | 2 | `gap` in `investment_capability_matrix_test.go` and `mustParseTestTime` in `investments_service_test.go`; retain for the upcoming test-maintainability disposition |

All paths below are under `backend/internal/`.

| Runtime function unreachable in both views | Inspection / disposition |
|---|---|
| `api/imports.go:rawJSONString` | Obsolete JSON fallback leaf; no references found. Inventory as possible later hygiene, not an import behavior defect |
| `app/exports.go:ExportService.SetNowForTest` | Unused clock setter; export service and its runtime clock remain live. Do not infer export coverage/behavior is absent |
| `app/investment_sale_replacement.go:mergeInvestmentCorrectionImpacts` | Old union-of-checkpoint helper; no references. Actual correction admission uses shared simulation/netted writer boundaries. Do not reintroduce this union as a new policy owner |
| `db/imports.go:ImportRepository.CountStagedRowsByCommitStatus` | Unused grouped-count query; this does not prove missing import progress because current batch/status read models have different callers |
| `db/investments.go:requirePositionBasisRangeTx` | Unused long-only wrapper. **Underlying `requirePositionBasisRangeQueryTx` is live** with explicit side/known-subtotal contracts. Removing a stale wrapper must never be presented as removing the admission guard |
| `db/transactions_read.go:DecodeRegisterCursor` | Unused forwarding alias; current paths use the shared `DecodeTransactionCursor`. No register pagination defect is established |

The 55 production-unreachable/test-reachable functions include clock/stage hooks,
test database setup and lower-level domain entry points. Examples are the
standalone current `PositionsWithGains`, `ListUnrealizedGains` and
`AlternativeGainsProjection` wrappers, preview convenience methods and direct
lot-disposal fixtures. They must not be deleted solely because the production
scan reports them. Whether to retire particular test-only entry points belongs
with the broader fixture/oracle review, preserving meaningful characterization.

The valuation path refines the earlier source-owner inventory: three named
`valuePositions` source consumers do not mean three independently active
production page readers. Current gains use a dated boundary resolved to today;
some standalone convenience wrappers are now exercised only by tests. The shared
valuation owner remains live. A retained production path is `CreateGainsRun` →
`GainsStatement` → `DatedPositionsWithGains` → `valuePositions`; do not create a
replacement engine or weaken snapshot reads based on a wrapper's unused status.

The legacy pricing schema probe from slice 1 is **not disposed by deadcode**:
`ListPriceObservations` still calls `tableHasColumn` and can dispatch to
`listLegacyPriceObservations`. Function reachability does not determine whether
that old schema is supported or exercised. Its per-call schema query remains an
unranked support/fixture investigation, not proven dead-branch removal.

## Frontend findings and false positives

| Knip view | Files | Exports | Types | Unresolved |
|---|---:|---:|---:|---:|
| Default discovery | 5 | 139 | 74 | 0 |
| Explicit entry points, normal roots | 0 | 139 | 74 | 0 |
| Explicit production frontend roots | 0 | 150 | 75 | 0 |
| Normal roots, ignore exports referenced inside their own file | 0 | 41 | 35 | 0 |

All five default file findings are **retained live entry points**:

- `frontend/static/theme-init.js` is loaded by `frontend/src/app.html`.
- `e2e/proxy-audit.config.ts` is selected by `deploy/docker/proxy-audit/run.sh`;
  its test directory owns the two proxy-audit specs.
- `e2e/proxy-audit/totp-code.ts` is executed directly by that shell harness.

These roots are declared in the audit configuration, not hidden by file ignores.
The production view excludes development/build project files; it does not label
the Vite CSP plugin dead merely because runtime pages do not import build code.

The normal view's **213 export/type findings** split into **137 with references
inside their own file** (98 exports, 39 types), and **76 without such references**
(41 exports, 35 types). For example, `getAuthSession` is used by the exported
query-options owner in its own module. Knip reports its unused external export
visibility; deleting the function would break the live query. Theme predicates,
query keys and many other API wrappers have the same distinction.

The remaining 41 export names are mostly unused convenience API clients/query
builders, plus a lot-status label. They include account/payee/tag lifecycle
wrappers, old ledger/pricing query wrappers and automation-rule helpers. The
catalog retains every exact name and location. An unused frontend wrapper does
**not** imply that its API endpoint, backend guard or user workflow is unused;
screens can use other clients/read models. The 35 type findings are source
surface, not executable financial code. They are later hygiene inventory with no
accepted deletion or measured runtime benefit.

Normal and production views have **228 distinct symbols** in their union:
137 in-file visibility findings, 41 unreferenced exports, 35 unreferenced types
and 15 production-only surface findings. The latter include exports imported by
unit tests but otherwise local to their module; they are not automatically
test-only implementations. Three E2E-support visibility findings occur only in
the normal view, which is why subtracting the totals alone gives the wrong
production-only count. No unresolved import or unreferenced runtime source file
remains in the explicit-context sweep. This is not proof about every Svelte
local variable, dynamic dispatch or generated bundle.

## Named abstraction consumer dispositions

The type index keeps declaration-file identity: `app.DividendProvider` and
`marketdata.DividendProvider` are separate interfaces despite sharing a name.
There are 380 type-reference and 141 member-reference sites, deduplicated across
test recompilations. Member-reference **owners** below are enclosing source
functions, not request counts or a whole-program caller graph.

| Declaration group | Disposition |
|---|---|
| `app.SourceAdapter`, `app.ConnectionProber` | Retain format/provider admission boundaries. Adapter detection/parse has multiple consumers/formats; credential probing is used on create/update and has provider/test implementations |
| `app.InstrumentSearchProvider`, `app.DividendProvider`, `app.CorporateActionProvider` | Zero indexed references, including tests. Old declarations remain as hygiene inventory; active registry capabilities use the `marketdata` contracts. No new abstraction or missing-provider-feature claim follows |
| `db.queryer`, `rowQueryer`, `rowScanner`, `execContexter`, `saleOperationReader`, `transferOperationReader` | Multiple member-reference owners; preserve DB/transaction compatibility and shared read/write leaves. Do not duplicate typed persistence APIs to eliminate a small interface |
| `db.backupPageCopier` | One member-reference owner, `copyBackupPages`; retains production copier and deterministic step/deadline/failure controls. No measured simplification benefit |
| `db.shortLinkScanner`, `importStagedRowScanner`, `queryRowContexter`, `splitOperationReader` | One member-reference owner each; row/rows and DB/transaction variants share a leaf. Their narrow interfaces prevent tying a helper to one concrete reader. Single consumer alone does not establish #284 criterion 4 |
| `marketdata.Provider`, `FXRateProvider`, `PriceProvider`, `DividendProvider` | One indexed member-reference owner each, with registry/capability/type-assertion wiring. FX has four provider implementations, prices three, dividends one admitted provider plus test controls. Retain named capability contracts; adapter count alone does not show change cost |
| `api.HandlerOptions`, `app.qifOptions`, `app.cleanTransactionOptions` | Multiple member-reference owners; proxy/setup admission, explicit import-locale decisions and internal transaction-producer policy remain distinct configuration contracts |
| `db.OnlineBackupOptions` | One member-reference owner; three type-reference sites carry page/deadline controls. Preserve the explicit backup resource contract |
| `app.ForecastLearningOption` | Category-choice data carrier with one formatter owner, **not configuration or a behavioral abstraction**. The suffix match is a classification false positive, not a design problem |

Thus all **25 named declarations** are dispositioned: **21 retained boundaries,
one data carrier and three unreferenced declarations**. Nine interfaces have
exactly one indexed runtime member-reference owner; inspection found no measured
change/test-construction cost justifying their removal. This rejects a blanket
“one caller means unnecessary interface” refactor. It does not certify every
anonymous interface, callback/flag, frontend type or future consumer.

## Reproduction and validation

Use a fresh Git archive of the source SHA, excluding archive experiments. Link
the original frozen root/frontend/E2E dependencies into that temporary checkout;
copy the frontend's generated Paraglide/SvelteKit resolution context from the
identical source build. Do not scan a mixed working tree. Copy the retained
Knip config to its root as `knip.audit.json`; passing a config located in another
repository can change workspace discovery.

Copy the retained tool manifests/locks to an isolated `/tmp` directory, install
Knip with `pnpm install --frozen-lockfile --ignore-scripts`, and build the Go
tools from the retained Go module:

```sh
go build -o /tmp/rekenraam-284-tools/deadcode-pinned golang.org/x/tools/cmd/deadcode
go build -o /tmp/rekenraam-284-tools/go-abstractions-pinned \
  /path/to/rekenraam/docs/reviews/fixtures/maintainability-go-abstractions.go
```

From the archived `backend/`, source `../scripts/native-sqlite-env.sh`, then run:

```sh
GOCACHE=/tmp/rekenraam-go-build-cache GOMEMLIMIT=2GiB \
  /tmp/rekenraam-284-tools/deadcode-pinned -json \
  -filter='^rekenraam/backend/' ./... > /tmp/go-production.json
GOCACHE=/tmp/rekenraam-go-build-cache GOMEMLIMIT=2GiB \
  /tmp/rekenraam-284-tools/deadcode-pinned -test -json \
  -filter='^rekenraam/backend/' ./... > /tmp/go-tests.json
GOCACHE=/tmp/rekenraam-go-build-cache \
  /tmp/rekenraam-284-tools/go-abstractions-pinned -root "$PWD" > /tmp/go-types.csv
```

From the archived repository root, run the pinned Knip CLI with
`--config knip.audit.json --reporter json --include files,exports,types,unresolved`.
Repeat with `--production --workspace frontend`. For the third view, copy the
config and add top-level `"ignoreExportsUsedInFile": true`, then run normal
roots. **Knip exits 1 when it reports these findings**; this is not an app test
failure. Check the JSON and unresolved count rather than suppressing tool errors.

The control script's `prepare` phase takes a fresh temporary directory and
`--frontend-deps /path/to/frontend/node_modules`. Run the same scanners on its
`backend/` and `web/`: save `go-prod.json`, `go-tests.json`, `go-types.csv`,
`web.json` and `web-local.json` at the control root. Use module filter `^controls$`
and the supplied web configs, then invoke `verify ROOT --output RESULT.json`.
Expected controls distinguish live dynamic interface/generic calls, a test-only
function, a genuinely unreachable function, unused TS/Svelte files, a local-only
export and an unused type. Generic `T` is not a named interface candidate.

**Validation passed:** all six accepted production/test/type-index/Knip views
repeat byte for byte; all deliberate controls; 63 function, 228 symbol and 25
type dispositions against raw reports; source coordinates; 562 original runtime
input hashes; all 115 relative links/anchors across the five touched Markdown
documents; helper formatting/syntax; `./scripts/check-post-merge.sh HEAD` and
`git diff --check`. The accepted Knip debug trace uses archived runtime sources,
with no original-worktree runtime source references.
Application suites are not rerun for these artifact-only changes; loading test
executables is not recorded as executing their tests. No dependency, runtime,
API/schema, CI gate, release or owner-acceptance change is claimed.

## Remaining acceptance

#284 remains open for broader fixture/oracle and test-duplication work, any
additional sites/cost evidence that review reveals, then final ranked
investment/reporting dispositions and owner review. These reachability findings
do not independently satisfy the audit's change-cost justification test for a
refactor. No additional issue was opened: P3 #379 measurement and #380 forecast
calendar admission retain their separate scope. R13 specification may proceed;
implementation still needs its calculation contract and #284's final relevant
dispositions. The standalone test-root wrappers are not permission to remove
valuation or transaction-admission boundaries.
