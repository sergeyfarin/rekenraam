# Rekenraam

Rekenraam is structured as a monorepo with separate backend, frontend, API-test, e2e-test, and deployment areas.

## Ports

- App/frontend development URL: `http://localhost:16888`
- Backend development API URL: `http://localhost:18888`
- Production single binary and Docker URL: `http://localhost:16888`

The backend dev port uses `18888`, a lucky number pattern associated with wealth and success in Chinese numerology.

## Layout

```text
backend/        Go backend, SQLite access, migrations, backend tests, and embedded frontend assets
frontend/       SvelteKit app compiled to static files
api/            Bruno API tests and OpenAPI description
e2e/            End-to-end browser tests
docs/           Architecture notes and early product decisions
scripts/        Build and developer workflow scripts
deploy/         Docker and deployment notes
dist/           Local release output
```

## Architecture Notes

Early decisions for the self-hosted finance app are documented in [docs/early-architecture-decisions.md](docs/early-architecture-decisions.md). Keep that document current when a feature introduces a durable product or technical constraint.

## Run Development Servers

Install root and frontend dependencies once:

```sh
npm install
cd frontend
npm install
cd ..
```

Start the backend and frontend together from the repo root:

```sh
npm run dev
```

This uses `concurrently` to run both processes in one terminal with labeled `backend` and `frontend` logs. Stop both with `Ctrl+C`.

Open:

```text
http://localhost:16888
```

During development, SvelteKit serves the app on `16888` and proxies `/api` requests to the Go backend on `18888`.

You can still run each side separately when needed:

```sh
npm run dev:backend
npm --prefix frontend run dev
```

## Test Backend

Backend tests are independent from Node and the frontend:

```sh
cd backend
go test ./...
```

Run the backend manually:

```sh
cd backend
go run ./cmd/rekenraam
```

Check the hello API:

```sh
curl http://localhost:18888/api/hello
```

## Test Frontend

Install dependencies once:

```sh
cd frontend
npm install
```

Run SvelteKit checks:

```sh
npm run check
```

Build the static frontend:

```sh
npm run build
```

## Test API With Bruno

Open `api/bruno/` in Bruno.

Use the `local` environment when the backend is running directly on `18888`.

Use the `app` environment when testing the integrated binary or Docker app on `16888`.

## Test E2E

Install e2e dependencies once:

```sh
cd e2e
npm install
```

For development-mode e2e tests, run the backend and frontend dev servers first, then:

```sh
npm test
```

For integrated app testing, run the single binary or Docker app on `16888`, then:

```sh
E2E_BASE_URL=http://localhost:16888 npm test
```

## Build Single Binary

Install frontend dependencies first if needed:

```sh
cd frontend
npm install
```

Build the static frontend, copy it into the Go embed directory, and compile the binary:

```sh
./scripts/build-single-binary.sh
```

The output is:

```text
dist/rekenraam
```

## Deploy Single Binary

Copy `dist/rekenraam` to the target machine.

Create a data directory for SQLite:

```sh
mkdir -p data
```

Run the app:

```sh
HTTP_ADDR=:16888 DATABASE_URL=file:data/rekenraam.sqlite ./dist/rekenraam
```

Open:

```text
http://localhost:16888
```

For a real server, run the binary under a process manager such as `systemd`, and keep the SQLite data directory on persistent storage.

## Deploy Docker

Build and run with Docker Compose:

```sh
docker compose -f deploy/docker/compose.yaml up --build
```

Open:

```text
http://localhost:16888
```

The Compose file mounts SQLite data into a Docker volume named `rekenraam-data`.
