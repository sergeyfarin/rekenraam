#!/usr/bin/env bash
set -euo pipefail

: "${CI_SMOKE_ENV_FILE:=.env.ci.local}"
: "${CI_SMOKE_BASE_URL:=http://localhost:16888}"
: "${CI_SMOKE_FIRST_ADMIN_PASSWORD:=ci-first-admin-password}"

compose() {
  docker compose --env-file "$CI_SMOKE_ENV_FILE" "$@"
}

cleanup() {
  compose down -v >/dev/null 2>&1 || true
  rm -f "$CI_SMOKE_ENV_FILE"
}

trap cleanup EXIT

mkdir -p secrets backups
printf '%s\n' "$CI_SMOKE_FIRST_ADMIN_PASSWORD" > secrets/first_admin_password.txt
cp .env.production.example "$CI_SMOKE_ENV_FILE"
sed -i "s#https://finance.example.com#${CI_SMOKE_BASE_URL}#" "$CI_SMOKE_ENV_FILE"
sed -i 's#replace-with-a-long-random-string#ci-mfa-secret-key-ci-mfa-secret-key#' "$CI_SMOKE_ENV_FILE"
sed -i 's#SESSION_COOKIE_SECURE=true#SESSION_COOKIE_SECURE=false#' "$CI_SMOKE_ENV_FILE"

make prod-config-check PROD_ENV_FILES="--env-file $CI_SMOKE_ENV_FILE"

compose up -d --build --wait

for _ in $(seq 1 60); do
  if curl --silent --fail "${CI_SMOKE_BASE_URL}/api/v1/health" >/dev/null; then
    break
  fi
  sleep 2
done

curl --fail "${CI_SMOKE_BASE_URL}/api/v1/health"
compose exec -T app test -f /data/rekenraam.sqlite3
make backup-smoke DOCKER="docker compose --env-file $CI_SMOKE_ENV_FILE"