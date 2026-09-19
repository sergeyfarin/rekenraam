# Desktop application feasibility review

**Date:** 2026-09-19
**Status:** Analysis and recommendation only. This document does not change the
current product decision that native desktop applications are out of scope.

## Follow-up decision — 2026-09-19

After reviewing the transport and framework tradeoffs, the owner chose to wait
for a stable Wails v3 before reconsidering desktop implementation. This is not a
promise to build a desktop edition when v3 becomes stable; demand and the gates
in this review still apply.

If desktop work is reconsidered, the preferred order is now explicit:

1. reuse the existing `http.Handler` through Wails' in-process asset server if
   the transport conformance spike passes;
2. use loopback HTTP only when a required behavior cannot be made reliable
   through the in-process adapter; and
3. use narrow Wails bindings only for native host capabilities, never as a
   parallel business API.

Tauri and Electron remain technically possible but are not preferred. Tauri
would add Rust, its IPC/capability model, and a second native toolchain to a Go
application. Electron would add Node.js, bundled Chromium, Electron-specific
hardening, and its release/update ecosystem. Both retain the web frontend but
increase dependency and operational surface without improving reuse of the Go
backend. A non-web frontend is a substantially larger divergence: it would
duplicate screens, form behavior, localization, accessibility, client-side
validation, and UI tests, turning most future product slices into two-client
work.

One preparation item is valuable independently of every desktop choice:
extracting application construction and shutdown from `runServe` into a reusable,
Wails-independent runtime package. The owner asked for that work to move earlier
in `docs/roadmap.md` even if desktop ships much later or never. Its web-product
benefits are explicit lifecycle ownership, cleanup of partial startup failures,
testable composition, and a smaller server command; the roadmap scope fence
forbids adding Wails or changing runtime behavior as part of that extraction.

## Executive conclusion

Rekenraam can become a desktop application without rewriting its ledger,
database, API, or Svelte frontend. The current architecture is unusually well
suited to a thin Wails host: the frontend is already a static single-page app,
the backend is already Go, production already embeds the frontend, SQLite is
local-first, and all important frontend/backend interaction already crosses a
versioned HTTP API.

The difficult part is not showing the current UI in a desktop window. A useful
prototype should be achievable in days. The difficult part is releasing and
supporting a trustworthy financial desktop application: per-OS data and secret
storage, authentication inside a WebView, file dialogs and downloads, process
lifecycle, background work while the window is closed, installers, signing,
notarization, updates, migration-safe rollback, and a three-OS test matrix.

The recommended shape is:

1. Keep the self-hosted web application as the canonical product and preserve
   its current HTTP API and same-origin security model.
2. Add Wails as a thin, optional desktop host around the same application
   runtime. Do **not** expose the domain services again as a parallel set of
   Wails JavaScript bindings.
3. First try mounting the existing `http.Handler` inside Wails' in-process asset
   server. This retains the API boundary without opening a TCP listener and
   avoids Windows firewall, endpoint-protection, port, and packaged-app loopback
   friction. Use an ephemeral loopback listener only as a proven fallback when
   Wails' request/response adapter cannot support a required API behavior.
4. Keep everything in this repository. Add a second Go command and desktop
   packaging assets; continue sharing the frontend, migrations, services,
   OpenAPI contract, and tests.
5. Start with one operating system, preferably the one used by the maintainer
   and first testers. Treat support for Windows, macOS, and Linux as three
   distribution projects, not one checkbox.
6. Do a time-boxed technical spike before changing product scope. In particular,
   prove cookies, origin/CSRF checks, uploads, downloads, deep links, shutdown,
   SQLite locking, and a packaged build on the first target OS.

Wails is the best conceptual fit among the common desktop wrappers because it
uses Go and the platform WebView. As of this review, however, Wails v2 is the
stable line and Wails v3 is still beta. Wails v3 has the cleaner window and
asset/packaging APIs for the proposed in-process-host design, so a supported release
should either wait for and pin a stable v3 release or explicitly accept the
cost of integrating v2. A production decision must pin an exact Wails version;
it must not follow `latest`.

## What “desktop edition” means

There are two materially different products that are easy to conflate:

- **Local-data desktop edition:** the desktop process owns its own SQLite file
  and runs the entire Rekenraam backend locally. It works offline except for
  provider-backed features. This review recommends this interpretation.
- **Desktop client for a remote Rekenraam server:** the desktop window is only a
  client for a VPS or home-lab instance. That adds server selection, remote TLS,
  credentials, disconnection states, and possibly local caching. Supporting
  both local and remote books eventually creates identity and conflict questions.

The first desktop milestone should be local-data only. It should not introduce
sync, and it must not open the same SQLite database concurrently with a separate
server process. Moving a backup between server and desktop is data portability;
keeping both copies synchronized is a separate product with much higher risk.

## Why the current architecture is a good starting point

The following existing decisions and implementation details are assets rather
than obstacles:

- The production frontend is a static SvelteKit build. It does not depend on a
  Node production server, server-side `load`, or SvelteKit form actions.
- The Go binary already embeds and serves that frontend through
  `backend/internal/web`.
- Frontend requests use relative `/api/v1` URLs through the generated OpenAPI
  client. There is no deployment-specific backend URL spread across screens.
- HTTP handlers, application services, repositories, background workers, and
  migrations are already distinct layers. Wails does not need to become a new
  business-logic layer.
- SQLite uses the pure-Go `modernc.org/sqlite` driver. Wails itself introduces
  native platform build requirements, but the database layer does not add a
  second CGO dependency.
- The database lock, startup migrations, verified backups, restore command,
  read-only reporting pool, and forward-only migration policy are all reusable.
- The UI is already responsive and keyboard/accessibility work is part of the
  web product, so the desktop edition does not need a second component system.

The main structural obstacle is concentrated in
`backend/cmd/rekenraam/command.go`: application assembly, worker startup, HTTP
listener startup, and shutdown are currently one `runServe` function. That is a
refactoring problem, not an architectural rewrite.

## Recommended runtime architecture

### One application core, two hosts

Extract the reusable startup and shutdown work into a package inside the
existing backend module. Conceptually:

```text
shared application runtime
  ├─ opens and locks SQLite
  ├─ runs migrations
  ├─ constructs repositories and services
  ├─ starts/stops workers and schedulers
  └─ exposes the existing http.Handler
        ├─ web host: configured TCP listener for VPS/Docker/LAN
        └─ desktop host: Wails in-process adapter (loopback fallback)
```

The reusable runtime should return an owned object with explicit `Handler`,
`Close`, and failure/lifecycle behavior. Both commands must use the same
construction path so the desktop edition cannot accidentally omit a worker,
security middleware, migration, or future service.

A likely repository layout is:

```text
backend/
  cmd/rekenraam/             # existing web/server command
  cmd/rekenraam-desktop/     # Wails desktop host
  internal/runtimehost/      # shared assembly and lifecycle
  internal/api/              # unchanged canonical interface
  internal/app/              # unchanged business rules
  internal/db/               # unchanged database and migrations
frontend/                    # one shared SvelteKit application
desktop/                     # icons, manifests, installer/signing metadata
```

The exact directory names can change during a spike. Keeping the desktop
command under the `backend` Go module avoids weakening Go `internal` package
boundaries or creating a second copy of backend dependencies.

### Preserve HTTP instead of adding a parallel binding API

Wails can bind Go methods directly into JavaScript. That is attractive for a
new application, but it is the wrong primary boundary here. Mirroring the
existing API as Wails bindings would create:

- two authorization and validation entry points;
- two error contracts;
- two frontend data-access paths;
- ambiguity about which path gets new features first;
- a larger native capability surface if an XSS bug occurs; and
- desktop-only tests for business behavior already covered through HTTP.

The desktop window should therefore use the same `/api/v1` routes as the web
application. Wails bindings, if any, should be a very small host-capability
interface: select an import file, select an export/backup destination, open an
external link, reveal the data directory, show application version, or request
a clean restart. Those capabilities should sit behind a frontend platform
adapter so ordinary screens remain host-neutral.

### Prefer Wails' in-process HTTP handler, with loopback as a fallback

“Keep the HTTP API” does not have to mean “open a local server port.” Wails can
adapt WebView requests to a Go `http.Handler` through its asset server. The
desktop host can therefore mount the existing Rekenraam handler in-process:

```text
WebView fetch('/api/v1/...')
  -> Wails asset-server request adapter (no TCP socket)
  -> existing Rekenraam http.Handler
  -> existing middleware, endpoint and application service
```

This is still a thin Wails host rather than a Wails-binding rewrite. It has
important operational advantages over loopback:

- no listening socket or port allocation;
- no Windows firewall prompt or rule to reason about;
- less friction from endpoint protection and corporate network policy;
- no MSIX/AppContainer loopback exemption question;
- no other local process can probe an HTTP port; and
- no need to distinguish “the desktop instance's local server” from a normal
  self-hosted instance in support material.

It also keeps most of the current application contract intact:

- relative API URLs;
- one handler and middleware chain;
- security headers and API 404 behavior;
- uploads and streaming downloads; and
- API-level integration and end-to-end tests.

The qualification is important: Wails' asset server is an HTTP-shaped adapter,
not a complete network HTTP server. Wails v2 documents method, header, body, and
status support across the three platforms, but no WebSockets, no response-body
streaming on Windows, and no HTTP redirects on macOS/Linux. Its documentation
also warns that this custom-handler development path is incompatible with Vite
5+, while Rekenraam uses a newer Vite. Wails v3 is intended to provide a more
consistent development/production asset path, but is beta at the time of this
review.

The in-process option must therefore pass a transport conformance suite before
selection. Test every HTTP method Rekenraam uses, large multipart imports,
headers, error status codes, cookies, origin/CSRF validation, CSV/QIF/ZIP
downloads, cancellation, and frontend development hot reload on every claimed
platform. In particular, Windows uses a `wails.localhost` origin while Wails has
historically used a custom `wails://` scheme on macOS/Linux. Current `Secure`
cookie and exact-Origin assumptions will probably need an explicit desktop
transport policy even though the endpoint handlers remain shared.

An ephemeral loopback listener remains the fallback because it provides real
browser HTTP semantics. If required, it must bind only to loopback, use an
OS-assigned port, reject unexpected hosts, and stop with the window. The spike
must prove `Secure` cookie behavior in each WebView. Authentication and the
existing setup token remain required because other same-user processes can
reach a loopback port. On Windows, ordinary unpackaged Win32 loopback may work
without user-visible firewall configuration, but packaged/AppContainer rules,
endpoint security, and managed-device policies still make “no listener” the
cleaner supported shape.

The decision order is therefore:

1. in-process Wails asset-server handler if it passes conformance;
2. loopback HTTP if a concrete required behavior cannot be made reliable; and
3. narrowly scoped Wails bindings only for native host capabilities, never as a
   replacement business API.

## Required changes

### 1. Refactor startup and lifecycle

- Move database locking, open/migrate/permission checks, service wiring, worker
  startup, route construction, and cleanup out of the server command.
- Make partial-startup failures close every acquired resource.
- Give workers and schedulers explicit cancellation and bounded shutdown.
- Keep the existing command-line maintenance commands independent of the GUI.
- Add a single-instance policy. The existing database lock prevents two writers,
  but the desktop host should detect this cleanly and focus the existing window
  or show a useful localized error rather than only logging a lock failure.

This refactor benefits the web product too: construction becomes testable and
the two hosts share one composition root.

### 2. Define desktop data, configuration, and log locations

Environment variables and relative `var/dev.sqlite` paths are appropriate for
servers and development, not installed desktop software. The desktop host needs
per-OS defaults based on the OS application-data directory for:

- the active SQLite database and lock file;
- verified backups;
- WebView profile/cache data;
- logs and crash diagnostics; and
- non-secret desktop preferences.

The active database must stay outside the installed application bundle so an
upgrade cannot replace it. The UI should expose “show data folder”, the current
database and backup locations, and the documented restore flow. It should not
encourage arbitrary selection of a live database file; that makes concurrent
opening and accidental network-filesystem use too easy.

Server environment configuration should remain supported unchanged. Desktop
defaults should be passed as an explicit config source, not implemented as
global environment mutation hidden inside Wails startup.

### 3. Give secrets a desktop lifecycle

`REKENRAAM_SECRET_KEY` currently comes from operator configuration and seals MFA
and online-provider credentials. An installed app cannot expect a user to set a
32-byte base64 environment variable before launch.

The desktop host should generate the key on first launch and store it in the OS
credential facility where practical (Windows Credential Manager/DPAPI, macOS
Keychain, and an appropriate Linux Secret Service). A documented, permissions-
restricted file fallback may be needed on headless/minimal Linux desktops.

This has a recovery consequence that already exists on the server: the key is
not in the SQLite backup. A desktop backup/restore UX must clearly preserve or
export recovery material, or explain that the ledger restores while MFA and
provider credentials must be reset. Quietly putting the key inside the same
backup would remove the separation it is intended to provide.

### 4. Adapt browser features behind a small platform boundary

Most screens should run unchanged. The following need packaged-WebView tests and
possibly native adapters:

- CSV/QIF/ZIP downloads currently create a Blob and click an anchor with a
  `download` name. WebViews differ in download behavior; desktop should probably
  offer an explicit native save dialog while keeping the HTTP response as the
  source of bytes and filename.
- File import currently uses browser `File`/`FormData`. Keep it if all target
  WebViews work correctly; use a native open dialog only if it improves reliability.
- Report CSV export is generated in the frontend and needs the same save-path
  abstraction as backend-generated exports.
- External URLs must open in the user's browser. Untrusted navigation must not
  replace the application window.
- Keyboard shortcuts, window close behavior, zoom, print, clipboard, dark mode,
  high-DPI scaling, and sleep/resume need explicit policies.
- Recovery instructions currently show a terminal command. A desktop recovery
  workflow must remain operator-controlled and backup-first but cannot assume a
  shell is the normal entry point.

Avoid gratuitous native UI. Native menus, tray behavior, and file associations
can be later enhancements; they are not required to prove the desktop edition.

### 5. Keep authentication and auditability consistent

Do not create a desktop bypass around authentication merely because the database
is local. Login protects an unattended open computer, keeps the same product
mental model, and avoids maintaining authenticated and unauthenticated service
paths. The first release can preserve the existing setup, password, session,
CSRF, MFA, throttling, and recovery rules.

Desktop-originated mutations may eventually use a distinct audit origin such as
`desktop_webview`, but this should be one intentional cross-cutting change, not
inferred separately by handlers. API semantics and permissions must remain the
same.

Any Wails binding has the privilege of the local process. A future XSS issue is
therefore more serious when broad bindings exist. Keep bindings narrowly scoped,
validate every path received from JavaScript, and never bind repositories or
application services wholesale.

### 6. Decide what closing the app means for background work

On a VPS, pricing refreshes, connection imports, backups, recurring generation,
and self-check scheduling run in a long-lived process. In a desktop edition they
stop when the application exits and may be suspended when the computer sleeps.

The first desktop release should state this plainly: scheduled work runs while
Rekenraam is open, and startup performs any safe catch-up already supported by
the scheduler. Tests must cover long downtime, sleep/resume, clock and time-zone
changes, lease recovery, and shutdown during work.

“Keep running in the tray” and “start at login” improve scheduler availability
but also change user expectations, battery use, update behavior, and the meaning
of Close. They should be optional later work, not hidden MVP requirements.

### 7. Add a real desktop release pipeline

Desktop distribution adds work that the current single binary and Docker image
do not have:

- native builds for each supported OS and architecture;
- application icons, metadata, version embedding, and installer manifests;
- Windows installer and Authenticode signing;
- macOS application signing, hardened runtime decisions, notarization, and
  universal or separate Intel/Apple Silicon artifacts;
- Linux package/AppImage decisions plus declared GTK/WebKitGTK runtime support;
- checksums, provenance/SBOM, release notes, and download hosting;
- update-channel and rollback policy; and
- smoke tests on clean virtual machines, not only build agents.

Wails uses the operating system WebView rather than shipping Chromium. That
keeps artifacts smaller, but it creates a WebView support matrix. Windows needs
WebView2; Linux users need compatible GTK/WebKitGTK runtime libraries; macOS uses
the system WebKit. Wails' current documentation explicitly describes these
platform dependencies and separate packaging/signing flows.

Automatic update should not be an MVP requirement. Begin with signed manual
updates and an in-app notification linking to release notes. When an updater is
added, it must download signed artifacts, coordinate clean shutdown, preserve a
verified pre-migration backup, and explain that a binary rollback after a schema
migration may require restoring that backup. The existing forward-only migration
policy remains correct.

## Testing impact

Shared behavior should continue to be proved once:

- Go service/repository/migration tests;
- HTTP handler and OpenAPI contract tests;
- frontend unit/type/accessibility checks; and
- browser end-to-end journeys against the ordinary web host.

Add a smaller desktop-specific suite rather than duplicating every journey:

1. launch a packaged app with a fresh data directory;
2. complete setup and login, including persistence across restart;
3. perform one authenticated mutation to prove Origin, cookie, and CSRF behavior;
4. import a file and save CSV/QIF/ZIP exports;
5. run a backup, verify it, and exercise the supported restore/restart path;
6. prove deep-link navigation, external-link handling, and API 404 separation;
7. close during idle and active background work and verify lease recovery;
8. upgrade a database from the oldest supported released version; and
9. run keyboard, scaling, light/dark, and critical accessibility smoke checks.

At least the host smoke suite must run on Windows, macOS, and each claimed Linux
packaging baseline. Rendering or download success on one engine is not evidence
for the other two.

## Estimated effort and continuing cost

These estimates assume one experienced engineer, the current application shape,
no sync, no app stores, manual updates initially, and reuse of the current UI.
They are planning ranges, not commitments.

| Scope | Estimated effort | Main uncertainty |
|---|---:|---|
| Technical spike on one OS | 3–7 working days | internal-handler conformance, Wails version, cookie/origin behavior, shutdown and download behavior |
| Unsigned internal desktop build on one OS | 1–2 additional weeks | runtime extraction, data paths, secret bootstrap, file UX |
| Supported signed release on the first OS | 2–4 additional weeks | installer, signing, recovery, clean-machine testing, documentation |
| Add a second OS | 1–3 additional weeks | signing/notarization and WebView differences |
| Add Linux as a broad public target | 2–4 additional weeks | distro/WebKitGTK ABI and packaging/support policy |
| Robust automatic updater | 1–3 additional weeks | signatures, failure recovery, migration-aware rollback |

Thus, a convincing prototype is **low-to-moderate difficulty**. A polished,
signed single-OS release is **moderate difficulty**, roughly 4–7 engineer-weeks
including the spike. A responsibly supported three-OS desktop line is
**moderate-to-high operational difficulty**, roughly 7–12 engineer-weeks before
ongoing maintenance. App-store distribution would add policy, entitlement,
sandbox, review, and release work not included here.

After the host is stable, ordinary backend and UI features should usually cost
only about 0–10% more because they remain shared. Features touching files,
secrets, background scheduling, external links, window lifecycle, or updates can
cost 20–50% more because they need platform behavior and testing. Across the
whole project, maintaining signed releases for three operating systems is likely
to add roughly 15–30% to release and maintenance effort even if feature code
stays unified. The cost arrives in CI, certificates, clean-machine tests,
WebView regressions, user support, and security updates rather than duplicated
ledger code.

The largest complexity multiplier would be product divergence. If desktop gains
native-only workflows or bypasses HTTP while web continues independently, the
cost can approach two clients rather than one product. The architecture should
make that divergence deliberately inconvenient.

## Repository decision

Keep the desktop edition in this repository.

That choice is strong here because both editions share:

- the same financial invariants and migrations;
- the same Go services and repositories;
- the same OpenAPI contract and static frontend;
- the same version and database compatibility policy; and
- mostly the same release milestones.

A monorepo makes an atomic change possible when a feature needs a migration,
service, endpoint, generated frontend type, screen, and desktop capability. CI
can prove that a pull request did not break either host. A split repository would
replace code separation with coordination overhead and make version skew easy.

Keep boundaries through packages and CI rather than repositories:

- desktop packaging code may depend on the shared runtime;
- the shared runtime must not depend on Wails;
- domain/application/database packages must not import desktop packages;
- frontend feature code uses HTTP and a tiny host-capability interface; and
- desktop release jobs can be conditional on relevant path changes while a
  scheduled matrix catches dependency and WebView drift.

Consider splitting only if the desktop program becomes a genuinely independent
remote client with a separate team and release cadence, or if app-store/native
functionality grows large enough that it no longer shares the application
runtime. Neither condition exists now.

## Wails version and alternatives

As of 2026-09-19, the Wails project describes v2 as stable and v3 as beta.
Current v2 documentation lists Windows, macOS, and Linux support, WebView2 on
Windows, and GTK/WebKitGTK dependencies on Linux. Wails v3 documentation exposes
a direct per-window URL and stronger packaging/signing tasks, which align well
with the proposed desktop host, but beta status is an avoidable release
risk for a finance application.

Recommended version policy:

1. Use a spike to test the exact pinned Wails v2 stable release and the exact
   current v3 beta/RC on the first target OS.
2. Prefer stable v3 once generally available and once its migration/release
   notes and required platform versions are acceptable.
3. If shipping before then, choose pinned v2 and budget for a later v3 migration.
4. Never let the build install `@latest`; record the CLI/runtime version and its
   checksums in the release toolchain.

Electron would make browser behavior more uniform but adds a bundled Chromium/
Node runtime and a separate ecosystem to a Go project. Tauri is capable and has
a mature distribution story, but introduces Rust for a shell whose backend is
already Go. A plain browser-launching Go binary is the cheapest near-term
“desktop” experience but does not provide an installed application window,
native menus/dialogs, or conventional packaging. Wails remains the most coherent
choice if a real desktop edition becomes a product priority.

## Recommended decision sequence

Do not commit to all-platform desktop support yet. Use these gates:

### Gate 1 — demand

Confirm that prospective users need an installed, local-data application rather
than easier self-host installation or PWA polish. Desktop distribution creates a
second deployment promise even when it shares all feature code.

### Gate 2 — one-OS spike

Time-box a spike and require all of the following before accepting an ADR:

- shared startup extraction without behavior drift;
- packaged launch on a clean machine;
- setup, login, restart, one mutation, upload, and download working;
- database in the correct OS data directory with single-instance handling;
- generated/stored secret key with a documented recovery consequence;
- clean shutdown and restart-safe background work; and
- a measured artifact/install/runtime dependency story.

Discard the spike or keep it off the production path until these pass. A window
that only renders the dashboard is not sufficient evidence.

### Gate 3 — architecture decision

If the spike succeeds and demand justifies the cost, add an ADR that supersedes
the current “native desktop out of scope” requirement. The ADR should lock:

- local-data versus remote-client scope;
- internal asset-handler versus HTTP loopback transport, selected by the
  conformance results;
- authentication/setup behavior;
- desktop data and secret locations;
- initial OS/architecture support;
- signing and update policy;
- background-work semantics while closed; and
- the single-repository rule.

Then update `docs/product-requirements.md`, `docs/early-architecture-decisions.md`,
`docs/developer-workflow.md`, and release documentation in the implementation
slice. Until that decision is accepted, this review is evidence, not a roadmap
commitment.

## Sources and evidence

Repository evidence reviewed:

- `docs/product-requirements.md`
- `docs/conventions.md`
- `docs/early-architecture-decisions.md`
- ADR 0002 (HTTP security), ADR 0003 (setup/auth), ADR 0004 (SQLite and backup),
  ADR 0005 (server container), ADR 0010 (durable background work), and ADR 0011
  (export/read-only database behavior)
- `backend/cmd/rekenraam/command.go`
- `backend/internal/api`, `backend/internal/app`, `backend/internal/db`, and
  `backend/internal/web`
- `frontend/svelte.config.js`, `frontend/vite.config.ts`, and
  `frontend/src/lib/api`

External primary sources checked on 2026-09-19:

- [Wails repository version status](https://github.com/wailsapp/wails)
- [Wails v2 installation and platform requirements](https://v2.wails.io/docs/gettingstarted/installation/)
- [Wails v2 application development and HTTP asset handler](https://wails.io/docs/guides/application-development/)
- [Wails v2 asset-server HTTP feature matrix](https://wails.io/docs/reference/options/#assetserver)
- [Wails v2 Linux runtime dependencies](https://wails.io/docs/guides/linux-distro-support/)
- [Wails v3 window URL option](https://v3.wails.io/features/windows/options/)
- [Wails v3 Windows packaging and signing](https://v3.wails.io/guides/build/windows/)
- [Wails v3 macOS signing and notarization](https://v3.wails.io/guides/build/macos/)
