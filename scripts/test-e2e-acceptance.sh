#!/usr/bin/env sh
# The acceptance-mapped browser subset (T-61).
#
# These cases map one-to-one onto the acceptance criteria in a plan's
# validation matrix, so closing an initiative can point at a suite rather than
# at an argument. They are tagged "[acceptance]" in their titles; everything
# else stays in the broader suites, which are split by cost rather than by what
# they prove.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/e2e" exec playwright test --grep "\[acceptance\]"
