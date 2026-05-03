#!/usr/bin/env bash
set -euo pipefail

: "${DOCKER:=docker compose}"

sh -c "$DOCKER up -d --build postgres api"

for _ in $(seq 1 30); do
  if curl --silent --fail http://localhost:8080/api/v1/health >/dev/null; then
    break
  fi
  sleep 1
done

curl --silent --fail http://localhost:8080/api/v1/health
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/books
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/accounts
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/accounts/tree
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/accounts/1
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/accounts/2/register
printf '\n'
curl --silent --fail http://localhost:8080/api/v1/transactions
printf '\n'