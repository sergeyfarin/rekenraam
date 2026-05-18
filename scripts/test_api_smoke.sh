#!/usr/bin/env bash
set -euo pipefail

: "${DOCKER:=docker compose}"

sh -c "$DOCKER up -d --build app"

BASE_URL="http://localhost:${APP_PORT:-16888}/api/v1"

for _ in $(seq 1 30); do
  if curl --silent --fail "$BASE_URL/health" >/dev/null; then
    break
  fi
  sleep 1
done

curl --silent --fail "$BASE_URL/health"
printf '\n'
curl --silent --fail "$BASE_URL/books"
printf '\n'
curl --silent --fail "$BASE_URL/accounts"
printf '\n'
curl --silent --fail "$BASE_URL/accounts/tree"
printf '\n'
curl --silent --fail "$BASE_URL/accounts/1"
printf '\n'
curl --silent --fail "$BASE_URL/accounts/2/register"
printf '\n'
curl --silent --fail "$BASE_URL/transactions"
printf '\n'
