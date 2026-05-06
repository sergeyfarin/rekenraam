#!/bin/sh
set -eu

: "${BACKUP_DIR:=/backups}"
: "${BACKUP_RETENTION_DAYS:=30}"
: "${PGDATABASE:?PGDATABASE is required}"
: "${PGUSER:?PGUSER is required}"

if [ -n "${PGPASSWORD_FILE:-}" ]; then
  export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"
fi

mkdir -p "$BACKUP_DIR"
test -w "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%d-%H%M%S)"
target="$BACKUP_DIR/rekenraam-$timestamp.dump"

pg_dump -Fc --file "$target" "$PGDATABASE"
test -s "$target"

if [ "$BACKUP_RETENTION_DAYS" -gt 0 ]; then
  find "$BACKUP_DIR" -type f -name 'rekenraam-*.dump' -mtime +"$BACKUP_RETENTION_DAYS" -delete
fi

echo "$target"
