#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/frontend" run build

cd "$ROOT"
find backend/internal/web/dist -mindepth 1 ! -name README.md -exec rm -rf {} +
cp -R frontend/build/. backend/internal/web/dist/

cd "$ROOT/backend"
REKENRAAM_RUN_EMBEDDED_WEB_TESTS=1 go test ./internal/web -run TestHandlerServesEmbeddedBuildOutput

# -trimpath keeps the builder's absolute paths out of the artifact: without it
# the binary carries ~1200 references to whatever home directory built it,
# which both leaks that layout to anyone who downloads a release and means two
# machines can never produce the same bytes.
#
# CGO_ENABLED=0 makes it a static binary. The SQLite driver is pure Go
# (modernc.org/sqlite, ADR 0004), so nothing here needs libc; leaving cgo on
# only links the host's glibc and ties the release to that version.
CGO_ENABLED=0 go build -trimpath -o "$ROOT/dist/rekenraam" ./cmd/rekenraam
