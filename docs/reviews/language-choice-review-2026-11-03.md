# Backend language choice review — 2026-11-03

**Status:** Analysis only. This document revalidates the current choice; it does
not propose a rewrite and does not change an ADR, requirement or roadmap item.
The governed runtime decision remains in
[Early Architecture Decisions](../early-architecture-decisions.md#runtime-shape)
and the accepted ADRs.

## Question

Rekenraam currently uses Go for the API, application services, repositories,
background workers, migrations, backup/restore and the static frontend host,
with SvelteKit/TypeScript in the browser and SQLite for storage. Is Go still the
right backend language, and what would Python, Rust, TypeScript or another
option add or take away?

This is a deliberate "keep or revisit" review, not a migration plan. It was
prompted by two recurring intuitions:

- Python is easier for AI-assisted coding and has a much larger training
  corpus, but is dynamically typed and usually slower and heavier at runtime.
- Rust is popular for AI-assisted systems work and has excellent runtime
  properties, but builds slowly, complicates concurrency, and can add a large
  dependency graph.

## Answer

**Keep Go.** No measured part of the current product is slow because it is
written in Go. The 2026-11-03 profile found the backend's time dominated by
native calls, statement preparation and per-connection schema loading, not by
Go arithmetic, JSON, HTTP or goroutine scheduling. The cgo bucket includes the
Go-to-C boundary as well as native execution, so it does not isolate SQLite
engine time; no controlled language/binding comparison was run. The defensible
conclusion is **no demonstrated rewrite benefit**, not proof that a rewrite
cannot help. The CI critical path is browser/axe work, not backend execution. A
backend rewrite would spend months re-proving financial behaviour while leaving
the measured bottlenecks in place.

This is not a claim that Go is universally better. Python is a better fit for
small scripts, data exploration and some AI workflows; Rust is a better fit for
a measured, pure-CPU component with strict latency/memory bounds. Neither
advantage is decisive for a single-user, self-hosted, SQLite-backed ledger.

## What would and would not change

| Concern | A backend language switch would change it | Evidence in this repo |
|---|---|---|
| API/service correctness | New implementation risk; same accounting rules | 2,622 passing application leaf cases, 82.8% backend statement coverage; ledger invariants are enforced in services, SQL and tests |
| SQLite/native time | Not isolated by the current profile; a language or binding change is not proven to help or hurt | 86.6% of sampled large-book CPU is `runtime.cgocall`, which includes the Go-to-C boundary and native execution, not pure SQLite engine time |
| Statement preparation / schema load | Only if the data layer or schema changes | 28.7% of test CPU in `sqlite3_prepare_v2`; 16.9% in driver connection open |
| Browser/E2E time | Nothing | E2E browser steps are 705–750 s medians; route/sub-view `Evaluate` (axe/layout) dominates |
| Single-binary deployment | Go is already the simplest fit | One 33 MiB binary embeds the static frontend; one process in Docker |
| Desktop path | Go + Wails is the reviewed path; Rust/Tauri adds a second native toolchain | [Desktop feasibility review](desktop-application-feasibility-2026-09-19.md) |
| AI-assisted coding | Python/TypeScript would be more familiar to models; Go needs more explicit scaffolding | No production measurement; model familiarity is not a runtime or correctness property |
| Build and CI time | Rust would likely make builds slower; TypeScript/Python add runtime/toolchain setup | CI is dominated by browser journeys; backend race step median is 270.5 s |
| Dependency/supply-chain surface | Rust/Python/Node add another ecosystem and native/tooling layer | Go has 8 direct modules and 101 modules in the full graph; the frontend already carries the pnpm/Node surface |

The important distinction is **language cost versus system cost**. The ledger is
slow where SQLite and the schema make it slow, and CI is slow where a browser
runs accessibility/layout matrices. Replacing Go does not remove either cost.

## Criteria

Ratings describe this product, not theoretical language benchmarks.

| Criterion | Go (current) | Rust | Python | TypeScript/Node | Other options |
|---|---|---|---|---|---|
| Static safety | Strong | Strongest (ownership, enums) | Weak unless mypy/pyright is enforced | Adequate; types erased at runtime | C#/Java strong; Elixir dynamic |
| Runtime CPU | Good | Best | Weak for CPU-heavy work | Adequate for I/O, weak for CPU | C#/Java good; C/C++ best but unsafe |
| Memory predictability | Good; GC but small live set | Best; no GC | Higher; interpreter/objects | Higher; V8 heap/GC | JVM heavier; BEAM different trade-off |
| Concurrency model | Simple goroutines/context | Powerful but more complex | GIL for CPU; async/process workarounds | Event loop; worker threads for CPU | Java virtual threads; BEAM processes |
| Single-binary deployment | Strong | Strong | Weak without a bundler | Weak without a bundled runtime | .NET AOT possible; JVM weak |
| Desktop reuse | Strong via Wails | Strong via Tauri, but second toolchain | Requires interpreter bundling | Electron/WebView wrapper, larger footprint | Depends on UI stack |
| Build speed | Fast | Slow | Fast to start, packaging slower | Fast dev, bundling slower | JVM/C# moderate |
| Dependency/supply chain | Small core, 8 direct modules | Often large crate graph; proc macros/build scripts | Large package graph; native wheels | Very large transitive graph | Varies |
| AI/LLM familiarity | Good, below Python/TS | Good and growing | Best | Best | Varies |
| Financial ecosystem fit | Good stdlib + exact custom types | Good but more assembly required | Good libraries, weaker typing/performance | Good web ecosystem, weaker numeric runtime | C#/Java finance ecosystems strong |

A language's from-scratch fit and the cost of moving this repository are
different questions. The ratings above describe the former. The table below
isolates the latter; a high transition cost does not make an option a bad
language in general, and a good from-scratch fit does not make a migration
worthwhile now.

### Transition cost from the current Go app

| Option | Transition from here | Main risks |
|---|---|---|
| Go | None; this is the current backend | None |
| Python | Full backend rewrite | Re-prove HTTP/auth, services, repositories, SQL, workers, backups, exact decimals and the whole test suite |
| Rust | Full backend rewrite, or an isolated FFI component | Same rewrite plus FFI/build/runtime boundaries; a component still needs differential tests and packaging |
| TypeScript/Node | Full backend rewrite | Same rewrite plus Node/native-SQLite packaging, worker-thread and dependency-graph risk |
| Other | Full backend rewrite | The same scope plus its own runtime, tooling and ecosystem risk |

## Go as-is

**What it brings**

- A small, typed server language with fast compilation and predictable
  deployment. The production shape is one binary serving `/api/v1` and the
  embedded SvelteKit build, with one process and one SQLite file.
- Straightforward lifecycle and concurrency for the HTTP server, background
  workers, schedulers and graceful shutdown. `context` cancellation is used
  across database and provider calls.
- A strong standard library for HTTP, JSON, cryptography, logging (`log/slog`)
  and testing, which keeps the dependency graph small: 8 direct modules and 101
  modules including transitive dependencies at this revision.
- Built-in race detection, CPU/heap profiling and test tooling. The current
  application suite runs 2,622 passing leaf cases under `-race`; the backend
  merged statement coverage is 82.8% against a 73% floor.
- A credible desktop path without a second backend: the
  [desktop review](desktop-application-feasibility-2026-09-19.md) prefers
  reusing the existing `http.Handler` through Wails rather than Tauri/Electron or
  a parallel binding API.
- Native SQLite is already isolated behind the repository/driver boundary under
  [ADR 0014](../adrs/0014-native-sqlite-driver.md). That is a deliberate native
  boundary, not an accident that a language switch would remove.

**What it costs**

- No sum types or exhaustive enums; investment kinds, statuses and error codes
  are strings guarded by validation and tests. Generics help containers but do
  not replace domain modelling.
- Verbose error handling and manual row scanning. This is visible in the
  repository/service layer, but it has not produced a measured runtime problem.
- CGO means the binary is not fully libc-independent, the build needs a C
  toolchain, and the native engine is outside Go's race detector. ADR 0014 and
  ADR 0016 accept these constraints explicitly.
- The frontend is a separate SvelteKit/TypeScript build. A single-language
  product would reduce context switching, but it would not change the backend's
  measured SQLite dominance.
- AI assistants produce more idiomatic Python/TypeScript than Go. This is a
  developer-experience cost, mitigated by a small set of conventions, typed
  API generation, focused tests and the compiler. It should not drive a
  production rewrite on its own.

**Transition cost from here:** None; this is the current implementation.

## Python

**What it would bring**

- The best model familiarity and iteration speed for prototypes, scripts,
  data exploration and AI-adjacent tooling. The archive already contains a
  FastAPI implementation, so the domain has been modelled in Python before.
- A large ecosystem for CSV/Excel ingestion, data analysis, plotting and
  provider integrations.
- Simple syntax and dynamic flexibility; `mypy`/`pyright` can add useful static
  checking when enforced.

**What it would take away or add**

- Dynamic typing by default. Financial DTOs, optional fields, decimal scales and
  status unions need discipline and CI type checks; runtime errors move from
  compile time to request time.
- The GIL limits CPU-bound parallelism. Async I/O helps provider and HTTP work,
  but CPU-heavy imports, reports or forecasting need processes or native
  extensions.
- Higher memory use and slower per-operation CPU than Go for the same service
  shape. That matters more in CI and on small self-hosted machines than in a
  prototype.
- Packaging is harder: a Python server normally brings an interpreter,
  virtualenv/container, dependency resolution and native wheels. A desktop
  build must bundle all of that, unlike the current Go binary + Wails path.
- The measured bottlenecks would remain. SQLite via Python still calls SQLite;
  statement preparation and schema loading do not become cheaper. The
  browser-driven CI path is untouched.

**Transition cost from here:** Very high. It would replace the HTTP layer,
application services, repositories, SQL, auth, workers, backup/restore,
exact-decimal handling and the current test suite. The archive review says not
to port Python implementation details directly; the ledger, investment,
correction and reconciliation behaviour added since then would need re-proving.

**Verdict:** excellent for analysis tools and experiments; not a better
production backend for this app without a new, measured reason.

## Rust

**What it would bring**

- The strongest combination of memory safety, predictable latency and low
  runtime overhead. Ownership and exhaustive enums are a good fit for financial
  state machines and parsing.
- Excellent pure-CPU performance and control over allocation, plus a credible
  native/desktop story through Tauri.
- A single binary is achievable, and the ecosystem is strong for parsers,
  compression, cryptography and systems integration.

**What it would take away or add**

- Slower builds and a steeper ownership/async learning curve. CI would likely
  get slower before it got faster; the current CI bottleneck is not backend
  compile time.
- A large crate graph is common once `serde`, `tokio`, HTTP, TLS, CLI and
  database crates are assembled. Proc macros and build scripts expand the audit
  surface; Rust is not automatically a smaller supply chain than Go.
- SQLite remains SQLite. `rusqlite` also binds the C engine; a pure-Rust database
  would be a data/product migration, not a language swap. The 2026-11-03
  large-book profile puts 86.6% of sampled CPU in cgo, but that bucket includes
  the Go-to-C boundary and native execution; it does not prove a Rust rewrite
  would recover or preserve a specific fraction. No controlled
  language/binding comparison was run.
- Tauri adds Rust and a second native toolchain to a Go application. The
  desktop review already compared this and preferred Wails for the current
  shape.

**Transition cost from here:** Very high for a full backend rewrite. It would
reimplement HTTP middleware, authentication, CSRF, session/token handling,
migrations, backup/restore, workers and the exact decimal/ledger rules, then
re-earn the existing test suite. An isolated Rust component is a smaller
transition but adds an FFI boundary, a second runtime/toolchain and
differential-test obligations.

**Verdict:** technically the strongest runtime alternative, but there is no
measured hotspot that justifies it today. Reconsider only for an isolated,
profiled, pure-CPU component after Go/SQL optimization has been exhausted.

## TypeScript / Node

**What it would bring**

- One language across the SvelteKit frontend and backend, shared types and
  generated OpenAPI contracts, and the largest web/AI ecosystem.
- Fast iteration, excellent async I/O and a familiar testing stack.
- Desktop/WebView wrappers such as Electron or Tauri are possible.

**What it would take away or add**

- The event loop is not a good fit for CPU-heavy report, import or investment
  work; worker threads add complexity and still share the V8 heap model.
- Native SQLite is an addon/FFI boundary. Packaging a single binary requires
  bundling Node/Bun/Deno, native modules and the frontend; the current Go
  binary already does the equivalent with fewer moving parts.
- TypeScript's types are erased at runtime. Runtime validation still needs
  schemas or hand-written guards, especially for financial payloads.
- The npm/pnpm transitive graph is already large for the frontend. Making the
  backend Node as well increases the amount of that graph that can affect the
  server process.
- The measured backend and CI costs do not move. Browser time remains browser
  time; SQLite remains SQLite.

**Transition cost from here:** High. It is a full backend rewrite in another
runtime, with the same HTTP/auth/service/repository/SQL/worker/backup/test
scope as Python or Rust, plus Node/native-SQLite packaging and worker-thread
constraints.

**Verdict:** reasonable for an all-TypeScript product, but not a clear upgrade
for this deployment shape and not a fix for any measured bottleneck.

## Other options considered

| Option | Attraction | Why it is not a better fit now |
|---|---|---|
| C# / .NET | Strong typing, async, performance, NativeAOT and good tooling | Adds a runtime/ecosystem and a different desktop/UI path; no measured problem it solves |
| Java / Kotlin | Mature performance, libraries and concurrency (virtual threads) | JVM footprint/startup and packaging are heavier for a small self-hosted binary; no measured benefit |
| Elixir / Erlang | Excellent fault tolerance and process concurrency | The product is single-user and SQLite-local; the BEAM runtime and ecosystem add cost without a matching requirement |
| C / C++ | Maximum control and speed | Manual memory safety is a poor trade for financial correctness and long-lived maintenance |
| Zig | Low-level control with a smaller language | Immature ecosystem/tooling for this server, SQLite and desktop surface; no measured need |
| Dart / Flutter | Strong cross-platform UI | Would duplicate or replace the web UI and does not improve the current backend bottlenecks |

Any other backend option carries the same full-rewrite transition cost as
Python, Rust or TypeScript, plus its own runtime, tooling and ecosystem risk.

## Why Go remains the coherent choice

The architecture is not "Go because Go is fastest". It is Go because the product
needs one small process, strong enough typing, simple concurrency, a mature
standard library, a native SQLite boundary and a reusable desktop path. The
current code already pays the native-boundary cost deliberately (ADR 0014) and
has test/tooling evidence for it (ADR 0016). A language switch would move that
boundary, not remove it.

The frontend is already TypeScript/SvelteKit. Adding Python, Rust or Node to the
backend would create a third ecosystem or duplicate the existing one; Go plus
the static frontend is two ecosystems, with the backend's dependency graph kept
small.

## Revisit triggers

Do not start a language migration for model familiarity or popularity. Reopen
this review only when one of these is true:

1. **A measured pure-CPU component remains dominant after Go and SQL work.**
   Example: a parser, forecast or report stage stays above roughly 20–30% of
   user-visible latency or CI time after query/schema optimization, and a
   profile shows the time in Go code rather than SQLite, I/O or the browser.
2. **A required library or platform capability has no viable Go path.** A
   specific provider, file format, native API or desktop capability would need
   to be named and proven.
3. **The deployment shape changes.** A hosted multi-tenant service, a different
   database or a desktop distribution decision could change the weighting; the
   current single-user, SQLite, self-hosted shape does not.
4. **The team/ownership model changes.** If future maintainers are primarily
   Python or TypeScript developers and cannot sustain the Go code, that is a
   real cost, but it is an organisational decision, not a performance claim.
5. **A security or supply-chain incident changes the dependency policy.** Even
   then, the response should compare concrete dependency graphs and audit
   evidence, not assume Rust is automatically safer.

## If a migration is ever justified

Do not do a big-bang rewrite. Preserve the HTTP/OpenAPI contract and use a
strangler path:

1. Keep SQLite, migrations and the financial schema unchanged.
2. Move one bounded read or pure function behind the existing API, with
   differential tests against the Go implementation on the same fixtures.
3. Prove the profile gain and operational cost on the real workload.
4. Only then consider a second component or a wider service boundary.

The existing conformance assets — OpenAPI, fixtures, ledger self-checks, 2,622
application leaf cases and the E2E journeys — are the safety net for any such
experiment. They are also the reason a rewrite is expensive: they encode
behaviour, not just syntax.

## References

- [CI and test performance profile — 2026-11-03](ci-test-performance-profile-2026-11-03.md)
  and its [evidence](fixtures/ci-test-profile-2026-11-03.json).
- [Early Architecture Decisions](../early-architecture-decisions.md).
- [ADR 0014: Native SQLite Driver](../adrs/0014-native-sqlite-driver.md) and
  [ADR 0016: Native Backend Test Resources](../adrs/0016-native-backend-test-resources.md).
- [Desktop application feasibility review](desktop-application-feasibility-2026-09-19.md).
- [Archive FastAPI backend review](../archive/fastapi-backend-review.md) — history,
  not a current requirement source.
