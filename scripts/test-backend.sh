#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT/backend"

# Formatting gate. Without this in the wrapper script, CI never sees gofmt
# drift (backlog T-43: four files sat unformatted with a green pipeline).
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "gofmt needs to be run on:" >&2
  echo "$unformatted" >&2
  echo "Fix with: gofmt -w <files>" >&2
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
  go test -race ./...
fi
