DOCKER ?= docker compose -f compose.postgres.yaml
SQLITE_DOCKER ?= docker compose -f compose.sqlite.yaml
DEV_DOCKER ?= docker compose -f compose.dev.yaml
TEST_DOCKER ?= docker compose -f compose.dev.yaml -f compose.test.yaml
CONTAINER_RUNTIME ?= docker
UV ?= uv
UV_CACHE_DIR ?= /tmp/rekenraam-uv-cache
PROD_ENV_FILES ?= --env-file .env.production.example $(if $(wildcard .env),--env-file .env,)
API_DIR := apps/api
MIGRATIONS_DIR := apps/api/alembic/versions

.PHONY: api-check api-lint api-format-check api-typecheck api-test api-test-fast api-test-coverage api-test-docker api-test-sqlite api-test-db api-test-postgres api-test-postgres-coverage api-up api-down api-logs api-health api-books api-accounts api-accounts-tree api-account-register api-transactions api-smoke api-reset-db api-migrate-new api-migrate-up api-migrate-down api-migrate-current api-migrate-smoke api-migrate-smoke-postgres api-dev-up api-dev-down api-dev-logs sqlite-up sqlite-down sqlite-logs web-up web-dev-up prod-config-check prod-sqlite-config-check backup-now backup-smoke restore-smoke

api-check:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run python -m py_compile $$(find src -name '*.py')

api-lint:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run ruff check .

api-format-check:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run ruff format --check .

api-typecheck:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pyright

api-test:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) REPOSITORY_DB_BACKENDS=sqlite API_E2E_DB_BACKEND=sqlite $(UV) run pytest -q -m "not postgres_compat"

api-test-fast:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) REPOSITORY_DB_BACKENDS=sqlite $(UV) run pytest -q -m "not postgres_compat" --ignore=tests/e2e

api-test-coverage:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) REPOSITORY_DB_BACKENDS=sqlite API_E2E_DB_BACKEND=sqlite $(UV) run pytest -q -m "not postgres_compat" --cov=rekenraam_api --cov-report=term-missing --cov-report=xml --cov-report=html

api-test-docker:
	$(CONTAINER_RUNTIME) run --rm -v "$$(pwd)/$(API_DIR)":/workspace -w /workspace python:3.14-slim-bookworm sh -lc "python -m pip install --upgrade pip >/dev/null && pip install --quiet -e .[dev] && pytest -q"

api-test-sqlite:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) REPOSITORY_DB_BACKENDS=sqlite API_E2E_DB_BACKEND=sqlite $(UV) run pytest -q -m "not postgres_compat"

api-test-db: api-test-sqlite api-test-postgres

api-test-postgres:
	$(TEST_DOCKER) down -v
	$(TEST_DOCKER) up -d postgres
	$(TEST_DOCKER) run --rm -e TEST_POSTGRES_HOST=postgres -e REPOSITORY_DB_BACKENDS=postgresql api-dev pytest -q tests/test_repositories.py
	$(TEST_DOCKER) down -v

api-test-postgres-coverage:
	$(TEST_DOCKER) down -v
	$(TEST_DOCKER) up -d postgres
	$(TEST_DOCKER) run --rm -e TEST_POSTGRES_HOST=postgres -e REPOSITORY_DB_BACKENDS=postgresql api-dev pytest -q tests/test_repositories.py tests/test_migrations.py --cov=rekenraam_api --cov-report=term-missing
	$(TEST_DOCKER) down -v

api-up:
	$(DOCKER) up -d --build postgres api frontend

api-down:
	$(DOCKER) down

api-reset-db:
	$(DOCKER) down -v

api-logs:
	$(DOCKER) logs -f api

api-health:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/health

api-books:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/books

api-accounts:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/accounts

api-accounts-tree:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/accounts/tree

api-account-register:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/accounts/2/register

api-transactions:
	curl --fail http://localhost:$${FRONTEND_PORT:-3000}/api/v1/transactions

api-smoke:
	DOCKER='$(DOCKER)' ./scripts/test_api_smoke.sh

api-dev-up:
	$(DEV_DOCKER) up -d --build postgres api-dev frontend-dev

api-dev-down:
	$(DEV_DOCKER) down

api-dev-logs:
	$(DEV_DOCKER) logs -f api-dev

sqlite-up:
	$(SQLITE_DOCKER) up -d --build app

sqlite-down:
	$(SQLITE_DOCKER) down

sqlite-logs:
	$(SQLITE_DOCKER) logs -f app

web-up:
	$(DOCKER) up -d --build postgres api frontend

web-dev-up:
	$(DEV_DOCKER) up -d --build postgres api-dev frontend-dev

prod-config-check:
	$(DOCKER) $(PROD_ENV_FILES) -f compose.prod.example.yaml -f compose.proxy.yaml config >/dev/null

prod-sqlite-config-check:
	$(SQLITE_DOCKER) $(PROD_ENV_FILES) -f compose.sqlite.public.yaml -f compose.proxy.yaml config >/dev/null

backup-now:
	$(DOCKER) -f compose.prod.example.yaml --profile backup run --rm backup

backup-smoke:
	@mkdir -p backups
	$(DOCKER) -f compose.prod.example.yaml --profile backup run --rm -e BACKUP_RETENTION_DAYS=0 backup
	@test -n "$$(ls -t backups/rekenraam-*.dump 2>/dev/null | head -n 1)"

restore-smoke:
	@test -n "$(BACKUP)" || (echo "Usage: make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.dump" && exit 1)
	DOCKER='$(DOCKER)' BACKUP='$(BACKUP)' ./scripts/restore_smoke.sh

api-migrate-new:
	@test -n "$(NAME)" || (echo "Usage: make api-migrate-new NAME=create_users" && exit 1)
	@mkdir -p $(MIGRATIONS_DIR)
	@cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run alembic revision -m "$(NAME)"; \
	file=$$(ls -t alembic/versions/*.py | head -n 1); \
	echo "Created $${file}"

api-migrate-up:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run alembic upgrade head

api-migrate-down:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run alembic downgrade $(if $(REV),$(REV),base)

api-migrate-current:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run alembic current

api-migrate-smoke:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) REPOSITORY_DB_BACKENDS=sqlite SKIP_SQLITE_MIGRATION_TEST=0 $(UV) run pytest -q -m "not postgres_compat" tests/test_migrations.py

api-migrate-smoke-postgres:
	$(TEST_DOCKER) down -v
	$(TEST_DOCKER) up -d postgres
	$(TEST_DOCKER) run --rm -e TEST_POSTGRES_HOST=postgres -e REPOSITORY_DB_BACKENDS=postgresql -e SKIP_SQLITE_MIGRATION_TEST=1 api-dev pytest -q -m "postgres_compat" tests/test_migrations.py
	$(TEST_DOCKER) down -v
