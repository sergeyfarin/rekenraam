#!/usr/bin/env bash
# The acceptance-mapped browser subset (T-61).
#
# These cases map one-to-one onto the acceptance criteria in a plan's
# validation matrix, so closing an initiative can point at a suite rather than
# at an argument. They are tagged "[acceptance]" in their titles; everything
# else stays in the broader suites, which are split by cost rather than by what
# they prove.
#
# pipefail so a failing stage of a pipeline inside this script fails the script;
# `set -e` alone only sees the last stage. It cannot help a caller that pipes
# this script — `... | tail` reports tail's status — that trap is the caller's
# to close. Needs bash: dash has no pipefail.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" exec playwright test --grep "\[acceptance\]"
