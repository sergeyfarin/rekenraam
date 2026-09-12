#!/usr/bin/env bash
# pipefail so a failing stage of a pipeline inside this script fails the script;
# `set -e` alone only sees the last stage. It cannot help a caller that pipes
# this script — `... | tail` reports tail's status — that trap is the caller's
# to close. Needs bash: dash has no pipefail.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" test