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
go test -race ./...
