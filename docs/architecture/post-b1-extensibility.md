# Post-B1 Extensibility Direction

Last updated: 2026-05-06

## B1 Boundary

B1 preserves extension compatibility without shipping plugin or theme execution.
There are no plugin/theme APIs, database tables, manifest schemas, permission
models, registries, WebAssembly dependencies, sidecar protocols, or custom CSS
loading in b1. The b1 implementation keeps the host app modular enough that
post-b1 extension work can be additive.

In short: plugin/theme runtime deferred, extension seams preserved.

Reserved future API namespaces:

- `/api/v1/plugins/*`
- `/api/v1/themes/*`

The only b1 theme-facing data is the existing per-user `theme` string
preference. Unknown values may be stored for future compatibility, but b1 does
not interpret them as installed theme packs.

## Plugin Host Model

Post-b1 plugins should use a two-tier model:

- Sandboxed WebAssembly plugins for constrained extension logic. Evaluate the
  WebAssembly Component Model, WASI capabilities, Wasmtime, and Extism-style
  host functions before choosing the runtime.
- Out-of-process sidecar plugins for arbitrary-language support. Sidecars
  should communicate through a narrow local HTTP or gRPC contract and run with
  Docker/container isolation.

Untrusted plugins must not run as in-process Python packages. Plugins should
never receive direct database credentials or repository access; they should call
typed host capabilities that enforce permissions, book scope, validation, and
audit attribution.

Initial extension candidates:

- import providers
- report providers
- pricing providers
- transaction enrichment rules
- static plugin assets
- manifest-declared navigation, settings panel, and report panel contributions

Frontend plugins should start as manifest-driven surfaces, not arbitrary
frontend code. Prefer navigation declarations, settings/report panel entries,
backend-served static assets, and iframe/server-rendered surfaces before
considering richer execution.

## Permission And Isolation Model

Future plugin manifests should use manifest-declared capabilities such as:

- `book.read`
- `transactions.write`
- `imports.parse`
- `reports.render`
- `pricing.fetch`
- `network.egress`
- `plugin.storage`
- `secrets.read`

Administrators must review and grant requested capabilities before a plugin is
enabled. Runtime enforcement belongs at the host-function or local API boundary,
not inside plugin code by convention.

Admin review is required before enablement.

The runtime must support:

- per-book grants
- network allowlists
- plugin-local storage quotas
- timeout and memory limits
- explicit secret access grants
- disabled/failed-plugin isolation
- audit events that include user, session, device, request id, plugin id,
  plugin version, capability used, and target book when applicable

## Theme Readiness

B1 should make the app themeable through semantic tokens while deferring actual
theme packs. The active token layer should cover:

- background, surface, text, border, input, ring
- primary, accent, muted, destructive, success, warning
- chart colors
- positive, negative, neutral, asset, and liability money colors
- account and transaction status colors

Post-b1 themes should start with built-in CSS token packs. Custom theme manifests
or mounted/admin-managed theme files can follow after deterministic fallback,
validation, CSP guidance, and admin enablement exist. Arbitrary remote CSS should
not be loaded.

## References

- WebAssembly specs and WASI overview: https://webassembly.org/specs/
- Wasmtime and WASI capability direction: https://docs.wasmtime.dev/
- Extism host functions: https://extism.org/docs/concepts/host-functions/
- OWASP secure architecture guidance: https://top10proactive.owasp.org/the-top-10/c4-secure-architecture/
- OWASP browser extension permission guidance: https://cheatsheetseries.owasp.org/cheatsheets/Browser_Extension_Vulnerabilities_Cheat_Sheet.html
