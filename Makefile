DOCKER ?= docker compose
API_MANIFEST := apps/api/Cargo.toml
MIGRATIONS_DIR := apps/api/migrations

.PHONY: api-check api-up api-down api-logs api-health api-books api-migrate-new

api-check:
	cargo check --manifest-path $(API_MANIFEST)

api-up:
	$(DOCKER) up -d --build postgres api

api-down:
	$(DOCKER) down

api-logs:
	$(DOCKER) logs -f api

api-health:
	curl --fail http://localhost:8080/health

api-books:
	curl --fail http://localhost:8080/api/v1/books

api-migrate-new:
	@test -n "$(NAME)" || (echo "Usage: make api-migrate-new NAME=create_users" && exit 1)
	@mkdir -p $(MIGRATIONS_DIR)
	@stamp=$$(date +%Y%m%d%H%M%S); \
	file="$(MIGRATIONS_DIR)/$${stamp}_$(NAME).sql"; \
	touch "$${file}"; \
	echo "Created $${file}"