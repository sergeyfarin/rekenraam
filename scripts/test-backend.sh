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
  # Ordinary integration fixtures copy a process-wide migrated SQLite template
  # (internal/testdb) instead of replaying the full schema per test. Keep Go's
  # default per-package timeout visible: if a package approaches it again, the
  # fixture economics need attention rather than more timeout headroom (T-70).
  go test -race ./...
fi
