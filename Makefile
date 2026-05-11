DOCKER ?= docker compose
DEV_DOCKER ?= docker compose -f compose.yaml -f compose.dev.yaml
CONTAINER_RUNTIME ?= docker
PROD_ENV_FILES ?= --env-file .env.production.example $(if $(wildcard .env),--env-file .env,)
API_DIR := apps/api
MIGRATIONS_DIR := apps/api/alembic/versions

.PHONY: api-check api-lint api-format-check api-typecheck api-test api-test-docker api-test-postgres api-up api-down api-logs api-health api-books api-accounts api-accounts-tree api-account-register api-transactions api-smoke api-reset-db api-migrate-new api-migrate-up api-migrate-down api-migrate-current api-migrate-smoke api-dev-up api-dev-down api-dev-logs web-up web-dev-up prod-config-check backup-now backup-smoke restore-smoke

api-check:
	cd $(API_DIR) && python -m py_compile $$(find src -name '*.py')

api-lint:
	cd $(API_DIR) && ruff check .

api-format-check:
	cd $(API_DIR) && ruff format --check .

api-typecheck:
	cd $(API_DIR) && pyright

api-test:
	cd $(API_DIR) && pytest -q

api-test-docker:
	$(CONTAINER_RUNTIME) run --rm -v "$$(pwd)/$(API_DIR)":/workspace -w /workspace python:3.14-slim-bookworm sh -lc "python -m pip install --upgrade pip >/dev/null && pip install --quiet -e .[dev] && pytest -q"

api-test-postgres:
	$(DEV_DOCKER) down -v
	$(DEV_DOCKER) up -d postgres
	$(DEV_DOCKER) run --rm -e TEST_POSTGRES_HOST=postgres api-dev pytest -q tests/test_repositories.py
	$(DEV_DOCKER) down -v

api-up:
	$(DOCKER) up -d --build postgres api frontend

api-down:
	$(DOCKER) down

api-reset-db:
	$(DOCKER) down -v

api-logs:
	$(DOCKER) logs -f api

api-health:
	curl --fail http://localhost:8080/api/v1/health

api-books:
	curl --fail http://localhost:8080/api/v1/books

api-accounts:
	curl --fail http://localhost:8080/api/v1/accounts

api-accounts-tree:
	curl --fail http://localhost:8080/api/v1/accounts/tree

api-account-register:
	curl --fail http://localhost:8080/api/v1/accounts/2/register

api-transactions:
	curl --fail http://localhost:8080/api/v1/transactions

api-smoke:
	DOCKER='$(DOCKER)' ./scripts/test_api_smoke.sh

api-dev-up:
	$(DEV_DOCKER) up -d --build postgres api-dev frontend-dev

api-dev-down:
	$(DEV_DOCKER) down

api-dev-logs:
	$(DEV_DOCKER) logs -f api-dev

web-up:
	$(DOCKER) up -d --build postgres api frontend

web-dev-up:
	$(DEV_DOCKER) up -d --build postgres api-dev frontend-dev

prod-config-check:
	$(DOCKER) $(PROD_ENV_FILES) -f compose.yaml -f compose.prod.example.yaml config >/dev/null

backup-now:
	$(DOCKER) -f compose.yaml -f compose.prod.example.yaml --profile backup run --rm backup

backup-smoke:
	@mkdir -p backups
	$(DOCKER) -f compose.yaml -f compose.prod.example.yaml --profile backup run --rm -e BACKUP_RETENTION_DAYS=0 backup
	@test -n "$$(ls -t backups/rekenraam-*.dump 2>/dev/null | head -n 1)"

restore-smoke:
	@test -n "$(BACKUP)" || (echo "Usage: make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.dump" && exit 1)
	DOCKER='$(DOCKER)' BACKUP='$(BACKUP)' ./scripts/restore_smoke.sh

api-migrate-new:
	@test -n "$(NAME)" || (echo "Usage: make api-migrate-new NAME=create_users" && exit 1)
	@mkdir -p $(MIGRATIONS_DIR)
	@cd $(API_DIR) && alembic revision -m "$(NAME)"; \
	file=$$(ls -t alembic/versions/*.py | head -n 1); \
	echo "Created $${file}"

api-migrate-up:
	cd $(API_DIR) && alembic upgrade head

api-migrate-down:
	cd $(API_DIR) && alembic downgrade $(if $(REV),$(REV),base)

api-migrate-current:
	cd $(API_DIR) && alembic current

api-migrate-smoke:
	$(DEV_DOCKER) down -v
	$(DEV_DOCKER) up -d postgres
	$(DEV_DOCKER) run --rm -e TEST_POSTGRES_HOST=postgres api-dev pytest -q tests/test_migrations.py
	$(DEV_DOCKER) down -v
