#!/usr/bin/env sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

pnpm --dir "$ROOT/frontend" run build

cd "$ROOT"
find backend/internal/web/dist -mindepth 1 ! -name README.md -exec rm -rf {} +
cp -R frontend/build/. backend/internal/web/dist/

cd "$ROOT/backend"
go build -o "$ROOT/dist/rekenraam" ./cmd/rekenraam
