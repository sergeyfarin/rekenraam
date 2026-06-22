Backend Review — rekenraam
I reviewed the Go backend across the three layers (api → app → db), the financial primitives, auth, and the background pricing worker. go vet is clean, all tests pass (api suite ~22s), no TODO/FIXME markers. This is a high-quality, carefully-written codebase — what follows is mostly polish, not firefighting.

Overall assessment
The fundamentals are genuinely strong:

Auth (app/auth.go) — argon2id, constant-time compare, dummy-hash timing-attack mitigation, dual-scope (username + IP) throttling, transparent rehash-on-login. Textbook.
CSRF/origin (api/auth.go) — constant-time origin + double-submit CSRF token, careful proxy-header trust gating.
Financial math — the exact.Coefficient big-int decimal type and scaledAmount aggregation are lossless and overflow-guarded; double-entry balancing (transactions_validate.go:413) is per-commodity and scale-aware.
DB hygiene — consistent BeginTx + deferred-rollback-unless-committed pattern, readBookForUpdate row locking, audit-event threading, WAL pragmas verified at startup.
No N+1 — enrichPostings batches account/commodity lookups deliberately.
Findings (roughly highest to lowest value)
1. MaxOpenConns(1) + background FX worker share one connection — document the contract.
sqlite.go:64 caps the pool at 1. The FX coverage worker (pricing_worker.go) runs every minute and does many DB writes. I confirmed the good news: HTTP fetches happen outside DB transactions (pricing_refresh.go:237), so the single connection is never held across a 15s network call. But user requests still serialize behind worker writes (mitigated by busy_timeout=5000). Fine for a single-user app — but this is load-bearing and undocumented. Worth a comment at the SetMaxOpenConns(1) line explaining why it's safe (no tx spans I/O) so a future change doesn't silently break it.

2. Request bodies are truncated, not rejected, at the size limit.
setup.go:148 uses io.LimitReader(r.Body, 1<<20). A 1 MB+ body gets silently cut and surfaces as a confusing "invalid request body" / "must contain a single JSON object" error rather than a clear 413. http.MaxBytesReader returns an explicit error you can map to 413 Payload Too Large. Low severity, better DX. Same pattern in decodeOptionalJSONBody.

3. BookID = int64(1) is a hardcoded package constant (currencies.go:17), referenced ~everywhere. This is clearly an intentional single-book MVP scoping, not a bug — but it's threaded through dozens of call sites as a bare global. When multi-book lands it'll be an invasive change. No action needed now; flagging so it's a conscious decision, not an accident. Consider a one-line comment at the definition stating it's a deliberate single-book placeholder.

4. pow10 is an O(n) loop on a hot path.
transactions_reconciliation.go:72 loops to build 10^scale, and it's called inside ledger aggregation / balance checks per posting. big.Int has no cheap Exp shortcut, but scales are bounded (≤24, capped at 60) so a small precomputed []*big.Int cache would eliminate repeated allocation. Micro-optimization — only matters at high posting volume.

5. withRequestID always mints a fresh UUID, ignoring inbound X-Request-ID.
middleware.go:56. Behind a trusted proxy that sets a correlation ID, you lose request tracing continuity. Minor; only relevant if you run behind infrastructure that propagates request IDs.

Things I checked that are clean
No internal errors leaked to clients — the err.Error() calls in handlers all carry ValidationError/decode messages; everything else routes through the safe default → INTERNAL_ERROR arm (transactions_errors.go:38).
Marketdata HTTP clients have timeouts and response-body limits (http.go).
Reconciliation period-scoped guard is now fully wired into create/edit/void/unvoid/soft-delete with restore correctly exempt — this resolves the gap my saved memory flagged as "not yet implemented"; I've updated that memory.
Panic recovery, security headers, CSP, HSTS, graceful shutdown all present and correct.
Net: nothing here is a must-fix. The single highest-value item is #1 — not a code change, just a comment locking in the invariant that makes the single-connection design safe. Want me to make any of these edits (the MaxOpenConns comment, MaxBytesReader swap, or the pow10 cache)?

