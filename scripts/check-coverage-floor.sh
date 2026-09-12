#!/usr/bin/env bash
# pipefail so a failing stage of a pipeline inside this script fails the script;
# `set -e` alone only sees the last stage. It cannot help a caller that pipes
# this script — `... | tail` reports tail's status — that trap is the caller's
# to close. Needs bash: dash has no pipefail.
set -euo pipefail

# Reads the output of `COVERAGE=1 scripts/test-backend.sh` (or a file holding
# it), echoes the merged total into the GitHub job summary, and fails if the
# total dropped below FLOOR.
#
# FLOOR is a tripwire against erosion, not a target: keep it a couple of points
# under the current number, and raise it deliberately when the level rises.
FLOOR="${COVERAGE_FLOOR:-73.0}"

INPUT="${1:-/dev/stdin}"

# `|| true` because under pipefail a grep that matches nothing would abort here,
# replacing the explanation below with a bare non-zero exit. An absent total is
# a case this script reports on, not a case it crashes on.
TOTAL="$(grep -o 'total:.*[0-9.]\+%' "$INPUT" | grep -o '[0-9.]\+%' | tr -d '%' | tail -1 || true)"

if [ -z "$TOTAL" ]; then
  echo "could not find a merged coverage total in $INPUT" >&2
  exit 1
fi

echo "backend merged coverage: ${TOTAL}% (floor ${FLOOR}%)"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "Backend merged coverage: **${TOTAL}%** (floor ${FLOOR}%)" >>"$GITHUB_STEP_SUMMARY"
fi

if [ "$(awk -v t="$TOTAL" -v f="$FLOOR" 'BEGIN { print (t < f) ? 1 : 0 }')" = "1" ]; then
  echo "coverage ${TOTAL}% is below the floor ${FLOOR}%" >&2
  exit 1
fi
