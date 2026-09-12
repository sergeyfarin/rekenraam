#!/usr/bin/env bash
# The browser journeys fast enough to gate every push: everything except the
# serial release-preflight suite, which stays a deliberate pre-release run.
#
# pipefail so a failing stage of a pipeline inside this script fails the script;
# `set -e` alone only sees the last stage. It cannot help a caller that pipes
# this script — `... | tail` reports tail's status — that trap is the caller's
# to close. Needs bash: dash has no pipefail.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" exec playwright test --grep-invert "release preflight"
