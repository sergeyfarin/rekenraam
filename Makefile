DOCKER ?= docker compose
API_DIR := apps/api
MIGRATIONS_DIR := apps/api/alembic/versions

.PHONY: api-check api-lint api-typecheck api-up api-down api-logs api-health api-books api-accounts api-smoke api-reset-db api-migrate-new

api-check:
	cd $(API_DIR) && python -m py_compile $$(find src -name '*.py')

api-lint:
	cd $(API_DIR) && ruff check .

api-typecheck:
	cd $(API_DIR) && pyright

api-up:
	$(DOCKER) up -d --build postgres api

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

api-smoke:
	DOCKER='$(DOCKER)' ./scripts/test_api_smoke.sh

api-migrate-new:
	@test -n "$(NAME)" || (echo "Usage: make api-migrate-new NAME=create_users" && exit 1)
	@mkdir -p $(MIGRATIONS_DIR)
	@cd $(API_DIR) && alembic revision -m "$(NAME)"; \
	file=$$(ls -t alembic/versions/*.py | head -n 1); \
	echo "Created $${file}"