#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" exec playwright test release-preflight.spec.ts
