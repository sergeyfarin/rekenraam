#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT/backend"

# Formatting gate. Without this in the wrapper script, CI never sees gofmt
# drift (backlog T-49: four files sat unformatted with a green pipeline).
#
# Resolve gofmt from the toolchain `go` actually selected, never whichever one
# happens to be first on PATH. Those differ whenever a version manager pins an
# older Go while go.mod's `go` directive makes the go command switch to a newer
# one, and gofmt's output is not stable across releases (1.27 reindents
# multi-value composite literals in return statements). A stale gofmt reports
# both false negatives — drift that CI then rejects — and false positives on
# files that are correctly formatted for the toolchain being compiled with.
GOFMT="$(go env GOROOT)/bin/gofmt"
if [ ! -x "$GOFMT" ]; then
  echo "no usable gofmt in the selected toolchain at $GOFMT" >&2
  echo "go is $(command -v go) reporting $(go version)" >&2
  exit 1
fi

unformatted="$("$GOFMT" -l .)"
if [ -n "$unformatted" ]; then
  echo "gofmt needs to be run on:" >&2
  echo "$unformatted" >&2
  echo "Fix with: $GOFMT -w <files>" >&2
  exit 1
fi

go vet ./...

# COVERAGE=1 runs a separate, non-race coverage pass (a combined run is slow
# and muddies both signals). Writes backend/coverage.out and prints the merged
# total. Default mode is unchanged: the race-detector run.
if [ "${COVERAGE:-0}" = "1" ]; then
  go test ./... -coverpkg=./... -coverprofile=coverage.out
  go tool cover -func=coverage.out | tail -1
else
  # -timeout above Go's 10m default, which is a per-*package* budget and not a
  # considered one. internal/api is ~310 integration tests that each migrate a
  # fresh SQLite database, already 325-way parallel, and the race detector
  # multiplies that on a 2-4 core runner: the job measured 11m36s on 2026-08-26
  # (8c2d7206) with the package alone just under the limit, so the next handful
  # of tests tipped it into "panic: test timed out after 10m0s" while the tests
  # still listed as running were 1-second auth cases. Nothing hung; the budget
  # was simply smaller than the suite.
  #
  # Measured 2026-08-27: the package passes in 386s (6m26s) under -race on six
  # cores, and exceeded 600s on the runner — so CI is roughly 1.6x slower and
  # 25m is headroom, not a cost estimate. If that 386s figure grows past about
  # ten minutes locally, raising this number again is the wrong move; see T-70.
  go test -race -timeout 25m ./...
fi
