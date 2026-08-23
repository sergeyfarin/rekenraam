#!/usr/bin/env sh
# The browser journeys fast enough to gate every push: everything except the
# serial release-preflight suite, which stays a deliberate pre-release run.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" exec playwright test --grep-invert "release preflight"
