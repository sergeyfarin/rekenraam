# Redacted PAT audit evidence — 2026-10-20

See the [dated report](../../personal-access-token-audit-2026-10-20.md) for
scope, expected/actual outcomes, negative controls and retained limits.

- `metadata.json`: tested source, clean-checkout assertion, production image
  IDs, counts and validation identities. The runtime Go inputs at the backend
  gate SHA are identical to the final external-fixture SHA.
- `routes.csv`: every registered method and GET/HEAD alias, three independent
  scope combinations, and every forbidden pair retried with a valid owner
  cookie plus valid CSRF/Origin. Non-HEAD refusal bodies are independently
  checked as the Bearer-scope error, not just an arbitrary 403.
- `summary.json`: exact remaining path/header/attribution/redirect/storage/
  backup/restore/recovery/export/log cases and redacted CLI exit results.
- `browser.json`: actual Chromium bfcache restore, phone disclosure/storage
  and documented shared-tab/session/cache boundary observations.
- `file-modes.txt`: live database/WAL/SHM/lock files are 0600; preserved
  pre-restore directories are private 0700.
- `final-leakage.json`: final live-file and access/app-log byte-pattern checks
  after browser-created credentials, without storing the sampled files.
- `negative-controls.txt`: filtered/redacted pre-fix and intermediate-fix
  failure output for #359. No generated credential is retained.
- `backend-gate.txt`: final shared formatting/vet/backend race output.

All data is synthetic. No raw app/proxy log, cookie, key, token/hash, database,
WAL/SHM byte sample or native snapshot is published. Live-file byte copies are
leakage observations, not a backup/restore mechanism. The native drill uses
the real application's backup service and verify/restore/recover-owner CLI.

Published CSV/log text uses LF line endings and normalized whitespace;
CSV fields and the filtered failure messages retain their observed values.

`csp-followthrough.json`, `csp-negative-controls.txt` and `browser-smoke.txt`
record #361's production-policy measurement fix, its failing/passing controls,
all eight real subview configurations and the full 363-case shared smoke gate.
This test-helper follow-through changes no Go/frontend production inputs; the
independent PAT fixture/source and image identities above remain valid.
