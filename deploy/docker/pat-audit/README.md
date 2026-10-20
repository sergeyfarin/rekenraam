# Personal-access-token audit (#357)

Run from a committed checkout with Docker/Compose, curl, Python 3, pnpm and
the repository's Playwright Chromium installed:

```sh
PAT_AUDIT_OUTPUT=/tmp/pat-audit-evidence deploy/docker/pat-audit/run.sh
```

This **deletes and recreates only `rr-proxy-audit` and its volumes**. It binds
loopback ports 16888/8443 and uses private network 172.31.68.0/24; stop another
copy of that fixture first. It reuses the [proxy audit](../proxy-audit/README.md)
onboarding/MFA/key-loss drill, builds the supplied production image, then adds
access logging and two controlled redirect origins. No real credentials,
financial data, provider access or owner deployment are needed.

The Python external client verifies Caddy's local CA, uses a literal ADR 0018
grant oracle and enumerates every static route registration. It exercises all
registered methods and GET/HEAD aliases with all three scope combinations,
owner-cookie fallback with valid CSRF/Origin and token-specific refusal bodies,
encoded/canonical paths, duplicate headers, proxy trust, curl redirects,
native backup/verify/restore and CLI recovery. Exact synthetic
postings remain -2500/+2500 at scale 2 after refusals/restores/recovery. Native
backups retain hash-only credentials; ledger CSV, bundle and QIF do not.

Full Chromium observes an actual back/forward-cache restore of the disclosure
document, checks storage/cache/DOM and shared-session sign-out on a phone, and
checks anonymous `force-cache` refusal. The suite refuses to substitute a
reload or a synthetic event for the cache restore. The browser accepts the
disposable local CA; the independent HTTP client and curl verify its chain.
Headless tabs need an explicit `visibilitychange` signal, as in #268. Sign-out
in another tab is observed at the existing visibility/session refresh boundary
(otherwise the 60-second poll); this audit does not claim instant notification.

`http.json`, `browser.json`, final leakage/file-mode checks and source/image
identifiers are safe evidence. Raw logs, keys, cookies, databases and snapshots
stay in a private scratch directory and are removed with the fixture on exit.
`KEEP=1` retains synthetic secrets for debugging: delete that workdir and run
Compose `down -v` afterward. Never publish it or Playwright failure artifacts.
The script's output directory must be separate from its workdir.

The deterministic Go `TestPersonalAccessTokenAudit*` cases independently cover
queued expiry, committed admission versus revoke/recovery, security-event
failure and expired native restores. They complement these real clients; the
fixture cannot move a production clock or instrument SQLite's C internals.
This evidence covers this private proxy topology, not actual owner hosting
acceptance (#336) or every client's redirect configuration. The deliberate
curl `--location-trusted` negative control forwards to our controlled recipient
and proves unsafe client configuration is observable; it is not a recommended
client option.
