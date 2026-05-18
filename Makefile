DOCKER ?= docker compose
CONTAINER_RUNTIME ?= docker
UV ?= uv
UV_CACHE_DIR ?= /tmp/rekenraam-uv-cache
PROD_ENV_FILES ?= --env-file .env.production.example $(if $(wildcard .env),--env-file .env,)
API_DIR := apps/api
MIGRATIONS_DIR := apps/api/alembic/versions

.PHONY: api-check api-lint api-format-check api-typecheck api-test api-test-fast api-test-coverage api-test-docker api-test-sqlite api-test-db api-ci api-up api-down api-logs api-health api-books api-accounts api-accounts-tree api-account-register api-transactions api-smoke api-reset-db api-migrate-new api-migrate-up api-migrate-down api-migrate-current api-migrate-smoke web-install web-check web-test web-build web-ci web-up prod-config-check operational-self-hosting-smoke ci backup-now backup-smoke restore-smoke

api-check:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run python -m py_compile $$(find src -name '*.py')

api-lint:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run ruff check .

api-format-check:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run ruff format --check .

api-typecheck:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pyright

api-test:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pytest -q

api-test-fast:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pytest -q --ignore=tests/e2e

api-test-coverage:
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pytest -q --cov=rekenraam_api --cov-report=term-missing --cov-report=xml --cov-report=html

api-ci: api-check api-lint api-format-check api-typecheck api-test-coverage

api-test-docker:
	$(CONTAINER_RUNTIME) run --rm -v "$$(pwd)/$(API_DIR)":/workspace -w /workspace python:3.14-slim-bookworm sh -lc "python -m pip install --upgrade pip >/dev/null && pip install --quiet -e .[dev] && pytest -q"

api-test-sqlite: api-test

api-test-db: api-test

api-up:
	$(DOCKER) up -d --build app

api-down:
	$(DOCKER) down

api-reset-db:
	$(DOCKER) down -v

api-logs:
	$(DOCKER) logs -f app

api-health:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/health

api-books:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/books

api-accounts:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/accounts

api-accounts-tree:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/accounts/tree

api-account-register:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/accounts/2/register

api-transactions:
	curl --fail http://localhost:$${APP_PORT:-16888}/api/v1/transactions

api-smoke:
	DOCKER='$(DOCKER)' ./scripts/test_api_smoke.sh

web-install:
	npm ci

web-check:
	npm run check

web-test:
	npm test

web-build:
	npm run build

web-ci: web-install web-check web-test

web-up: api-up

prod-config-check:
	$(DOCKER) $(PROD_ENV_FILES) -f compose.yaml -f compose.public.yaml -f compose.proxy.yaml config >/dev/null

operational-self-hosting-smoke:
	bash ./scripts/operational_self_hosting_smoke.sh

ci: api-ci web-ci web-build operational-self-hosting-smoke

backup-now:
	@mkdir -p backups
	$(DOCKER) run --rm --no-deps app python -m rekenraam_api.tools.sqlite_backup

backup-smoke:
	@mkdir -p backups
	$(DOCKER) run --rm --no-deps -e BACKUP_RETENTION_DAYS=0 app python -m rekenraam_api.tools.sqlite_backup
	@test -n "$$(ls -t backups/rekenraam-*.sqlite3 2>/dev/null | head -n 1)"

restore-smoke:
	@test -n "$(BACKUP)" || (echo "Usage: make restore-smoke BACKUP=backups/rekenraam-YYYYmmdd-HHMMSS.sqlite3" && exit 1)
	$(DOCKER) run --rm --no-deps app python -m rekenraam_api.tools.sqlite_restore_smoke /backups/$$(basename "$(BACKUP)")

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
	cd $(API_DIR) && UV_CACHE_DIR=$(UV_CACHE_DIR) $(UV) run pytest -q tests/test_migrations.py
