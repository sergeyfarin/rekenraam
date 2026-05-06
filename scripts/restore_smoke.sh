#!/bin/sh
set -eu

: "${BACKUP:?Set BACKUP=/path/to/rekenraam.dump}"
: "${DOCKER:=docker compose}"
: "${RESTORE_DB:=rekenraam_restore_smoke}"
: "${POSTGRES_USER:=rekenraam}"
: "${POSTGRES_DB:=rekenraam}"

if [ ! -s "$BACKUP" ]; then
  echo "Backup file is missing or empty: $BACKUP" >&2
  exit 2
fi

$DOCKER up -d postgres

pg_env='if [ -f /run/secrets/postgres_password ]; then export PGPASSWORD="$(cat /run/secrets/postgres_password)"; fi'

$DOCKER exec -T postgres sh -lc "$pg_env; dropdb --if-exists --username '$POSTGRES_USER' '$RESTORE_DB'"
$DOCKER exec -T postgres sh -lc "$pg_env; createdb --username '$POSTGRES_USER' --template template0 '$RESTORE_DB'"
$DOCKER exec -i postgres sh -lc "$pg_env; pg_restore --username '$POSTGRES_USER' --dbname '$RESTORE_DB'" < "$BACKUP"
$DOCKER exec -T postgres sh -lc "$pg_env; psql --username '$POSTGRES_USER' --dbname '$RESTORE_DB' --tuples-only --command 'SELECT version_num FROM alembic_version LIMIT 1'"
$DOCKER exec -T postgres sh -lc "$pg_env; psql --username '$POSTGRES_USER' --dbname '$RESTORE_DB' --tuples-only --command 'SELECT count(*) FROM books'"
$DOCKER exec -T postgres sh -lc "$pg_env; dropdb --if-exists --username '$POSTGRES_USER' '$RESTORE_DB'"

echo "Restore smoke passed for $BACKUP"
