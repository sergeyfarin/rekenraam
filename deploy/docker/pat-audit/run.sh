#!/usr/bin/env bash
# #357: only synthetic state, one disposable Compose project, redacted evidence.
set -euo pipefail
umask 077
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
work="${PAT_AUDIT_WORKDIR:-$(mktemp -d)}"
output="${PAT_AUDIT_OUTPUT:-$(mktemp -d)}"
mkdir -p "$work" "$output"
case "$(realpath "$output")/" in
	"$(realpath "$work")/"*) echo 'Output must be outside the private workdir' >&2; exit 1 ;;
esac
compose() {
	docker compose -p rr-proxy-audit --env-file "$work/audit.env" \
		-f "$repo/deploy/docker/compose.yaml" -f "$repo/deploy/docker/proxy-audit/compose.yaml" \
		-f "$repo/deploy/docker/pat-audit/compose.yaml" "$@"
}
cleanup() {
	if [[ "${KEEP:-0}" != 1 ]]; then
		compose down -v >/dev/null 2>&1 || true
		rm -rf "$work"
	fi
}
trap cleanup EXIT

# Reuse the production-image/TLS/onboarding/MFA/key-loss drill, then add access
# logging and controlled redirect recipients. No production feature flag.
KEEP=1 PROXY_AUDIT_WORKDIR="$work" "$repo/deploy/docker/proxy-audit/run.sh"
compose up -d --no-deps --force-recreate proxy
python3 "$repo/deploy/docker/pat-audit/probe.py" --workdir "$work" --output "$output/http.json"
PAT_AUDIT_OWNER="$work/browser-owner.json" PAT_AUDIT_BROWSER_OUTPUT="$output/browser.json" \
	pnpm --dir "$repo/e2e" exec playwright test -c proxy-audit.config.ts personal-access-token-audit.spec.ts --reporter=list

# Inspect live DB/WAL/SHM bytes as leakage evidence, never as a restorable
# snapshot. Native backup/verify/restore above use the application's real CLI.
compose exec -T rekenraam sh -c 'stat -c "%a %n" /app/data/rekenraam.sqlite*' > "$output/file-modes.txt"
for suffix in '' '-wal' '-shm'; do
	compose cp "rekenraam:/app/data/rekenraam.sqlite$suffix" "$work/live.sqlite$suffix"
done
compose logs --no-color rekenraam proxy > "$work/final-logs.txt"
python3 - "$work" "$output" <<'PY'
import json, pathlib, re, sys
work, output = map(pathlib.Path, sys.argv[1:])
files = [*work.glob('live.sqlite*'), work / 'final-logs.txt']
for file in files:
    if re.search(rb'rr_pat_[A-Za-z0-9_-]{43}', file.read_bytes()):
        raise SystemExit('plaintext PAT detector failed: ' + file.name)
(output / 'final-leakage.json').write_text(json.dumps({
    'plaintext_absent': True, 'files': [file.name for file in files],
    'live_files_are_not_backups': True,
}, indent=2) + '\n')
PY
git -C "$repo" rev-parse HEAD > "$output/source-sha.txt"
git -C "$repo" status --porcelain > "$output/source-status.txt"
docker inspect --format '{{.Image}}' rr-proxy-audit-rekenraam-1 > "$output/app-image.txt"
docker inspect --format '{{.Image}}' rr-proxy-audit-proxy-1 > "$output/proxy-image.txt"
echo "PAT audit passed; redacted evidence: $output"
