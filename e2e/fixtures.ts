import { test as base, expect, type APIRequestContext, type Page } from "@playwright/test";
import { execSync } from "node:child_process";

const ADMIN_EMAIL = "e2e-admin@example.com";
const ADMIN_PASSWORD = "e2e-admin-password-1234";
const ADMIN_DISPLAY_NAME = "E2E Admin";

const POSTGRES_CONTAINER = process.env.PLAYWRIGHT_POSTGRES_CONTAINER ?? "rekenraam-postgres-1";
const POSTGRES_USER = process.env.PLAYWRIGHT_POSTGRES_USER ?? "rekenraam";
const POSTGRES_DB = process.env.PLAYWRIGHT_POSTGRES_DB ?? "rekenraam";

const BASELINE_DB = `${POSTGRES_DB}_baseline`;
const API_SERVICE = process.env.PLAYWRIGHT_API_SERVICE ?? "api";

/**
 * Snapshot/restore-based DB reset. Strategy:
 *
 *   1. First call: capture the post-migration state of `rekenraam` as a
 *      template DB `rekenraam_baseline`. The baseline includes all migration
 *      seeds (the `personal` book, USD commodity, Cash account, $5000 opening
 *      balance — see [apps/api/alembic/versions/0001_initial_schema.py]).
 *
 *   2. Each subsequent call: drop `rekenraam`, recreate it from the baseline
 *      template. Postgres copies the template at the page level so this is
 *      effectively a constant-time block-copy regardless of size.
 *
 * Why not TRUNCATE? Migration 0001 seeds rows the UI relies on (book_id=1,
 * commodity_id=1, Cash account at id=2). Truncating wipes those, leaving the
 * frontend pointing at non-existent objects. Snapshot-restore preserves
 * exactly the state the running container started with.
 *
 * Why not a test-only `/api/v1/test/reset` endpoint? The user explicitly
 * chose the snapshot approach to avoid shipping test code in the production
 * image; see [docs/product/phase-3-plan.md] §A2 decision record.
 *
 * Cost: we must stop the API container before dropping its database (Postgres
 * refuses `DROP DATABASE` with open connections). Restart adds ~2s per spec.
 * Net per-spec overhead: ~3s, vs. ~30s for a full compose reset.
 */
let baselineCreated = false;

function pgExec(sql: string, opts: { db?: string } = {}): void {
  const db = opts.db ?? POSTGRES_DB;
  // -v ON_ERROR_STOP=1 turns any SQL error into a non-zero exit so execSync throws.
  execSync(
    `docker exec -i ${POSTGRES_CONTAINER} psql -U ${POSTGRES_USER} -d ${db} -v ON_ERROR_STOP=1 -c ${JSON.stringify(sql)}`,
    { stdio: ["ignore", "ignore", "pipe"] },
  );
}

function ensureBaseline(): void {
  if (baselineCreated) return;

  // Idempotency: check if the baseline already exists from a prior run on the
  // same compose stack (developer running `npm run e2e` repeatedly).
  const existsOutput = execSync(
    `docker exec -i ${POSTGRES_CONTAINER} psql -U ${POSTGRES_USER} -d postgres -tA -c "SELECT 1 FROM pg_database WHERE datname='${BASELINE_DB}'"`,
    { encoding: "utf-8" },
  ).trim();

  if (existsOutput === "1") {
    baselineCreated = true;
    return;
  }

  // CREATE DATABASE ... TEMPLATE requires no other connections to the source.
  // Stop the API container to release its connection pool, snapshot, restart.
  execSync(`docker compose stop ${API_SERVICE}`, { stdio: ["ignore", "ignore", "pipe"] });
  try {
    pgExec(
      `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${POSTGRES_DB}' AND pid <> pg_backend_pid();`,
      { db: "postgres" },
    );
    pgExec(`CREATE DATABASE "${BASELINE_DB}" WITH TEMPLATE "${POSTGRES_DB}" OWNER "${POSTGRES_USER}";`, {
      db: "postgres",
    });
    baselineCreated = true;
  } finally {
    execSync(`docker compose start ${API_SERVICE}`, { stdio: ["ignore", "ignore", "pipe"] });
    // Wait for healthcheck before returning so the first spec doesn't race the API.
    waitForApiHealthy();
  }
}

function waitForApiHealthy(timeoutMs = 30_000): void {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const status = execSync(
        `docker compose ps --format json ${API_SERVICE}`,
        { encoding: "utf-8" },
      );
      if (status.includes('"Health":"healthy"') || status.includes('"State":"running"')) {
        // Once healthy, also poll the public health endpoint to be sure.
        try {
          execSync(`curl -fsS ${process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000"}/api/v1/health`, {
            stdio: ["ignore", "ignore", "pipe"],
          });
          return;
        } catch {
          // not ready, try again
        }
      }
    } catch {
      // compose ps failed, retry
    }
    // eslint-disable-next-line @typescript-eslint/no-loop-func
    execSync("sleep 0.5");
  }
  throw new Error(`API did not become healthy within ${timeoutMs}ms`);
}

/**
 * Reset the e2e database by dropping it and restoring from the baseline
 * template captured on first call. After this returns, the API reports
 * `bootstrap_required: true` again with the seeded `personal` book intact.
 */
export function resetDatabase(): void {
  ensureBaseline();

  // Stop API to release connections, drop+recreate from template, restart.
  execSync(`docker compose stop ${API_SERVICE}`, { stdio: ["ignore", "ignore", "pipe"] });
  try {
    pgExec(
      `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('${POSTGRES_DB}', '${BASELINE_DB}') AND pid <> pg_backend_pid();`,
      { db: "postgres" },
    );
    pgExec(`DROP DATABASE IF EXISTS "${POSTGRES_DB}";`, { db: "postgres" });
    pgExec(`CREATE DATABASE "${POSTGRES_DB}" WITH TEMPLATE "${BASELINE_DB}" OWNER "${POSTGRES_USER}";`, {
      db: "postgres",
    });
  } finally {
    execSync(`docker compose start ${API_SERVICE}`, { stdio: ["ignore", "ignore", "pipe"] });
    waitForApiHealthy();
  }
}

/**
 * Issue a bootstrap-admin request against the running API, leaving the
 * resulting session cookie on the Playwright `APIRequestContext` so subsequent
 * calls (and `page.goto`) are authenticated.
 */
export async function bootstrapAdmin(api: APIRequestContext): Promise<void> {
  const response = await api.post("/api/v1/auth/bootstrap/admin", {
    data: {
      email: ADMIN_EMAIL,
      password: ADMIN_PASSWORD,
      display_name: ADMIN_DISPLAY_NAME,
    },
  });
  expect(response.ok(), `bootstrap failed: ${response.status()} ${await response.text()}`).toBeTruthy();
}

export async function login(api: APIRequestContext): Promise<void> {
  const response = await api.post("/api/v1/auth/login", {
    data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  expect(response.ok(), `login failed: ${response.status()}`).toBeTruthy();
}

type Fixtures = {
  /**
   * Per-test database reset. Runs before the test body. Combined with
   * `workers: 1` in the Playwright config, this gives each spec a clean
   * baseline regardless of prior spec ordering.
   */
  cleanDatabase: void;

  /**
   * Authenticated APIRequestContext. Bootstraps the admin user (which also
   * logs in) and shares cookies with the browser context so `page.goto`
   * lands on a signed-in view.
   */
  authedApi: APIRequestContext;

  /**
   * Browser page already navigated to the home tab as an authenticated user.
   * Backed by `authedApi` — the session cookie is shared via storageState.
   */
  loggedIn: Page;
};

export const test = base.extend<Fixtures>({
  cleanDatabase: [
    async ({}, use) => {
      resetDatabase();
      await use();
    },
    { auto: true },
  ],

  authedApi: async ({ playwright }, use) => {
    const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000";
    const api = await playwright.request.newContext({ baseURL });
    await bootstrapAdmin(api);
    await use(api);
    await api.dispose();
  },

  loggedIn: async ({ browser, authedApi }, use) => {
    // Pull the cookies set on the API context and replay them on a fresh
    // browser context so the SPA boots already authenticated.
    const storageState = await authedApi.storageState();
    const context = await browser.newContext({ storageState });
    const page = await context.newPage();
    await page.goto("/");
    await use(page);
    await context.close();
  },
});

export { expect };
export { ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_DISPLAY_NAME };
