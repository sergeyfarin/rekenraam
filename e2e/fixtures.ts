import { test as base, expect, type APIRequestContext, type Page } from "@playwright/test";
import { execSync } from "node:child_process";

const ADMIN_EMAIL = "e2e-admin@example.com";
const ADMIN_PASSWORD = "e2e-admin-password-1234";
const ADMIN_DISPLAY_NAME = "E2E Admin";

const SQLITE_DB_PATH = process.env.PLAYWRIGHT_SQLITE_PATH ?? "/data/rekenraam.sqlite3";
const SQLITE_BASELINE_PATH =
  process.env.PLAYWRIGHT_SQLITE_BASELINE_PATH ?? "/data/rekenraam.e2e-baseline.sqlite3";

const API_SERVICE = process.env.PLAYWRIGHT_API_SERVICE ?? "app";
const COMPOSE_FILES = process.env.PLAYWRIGHT_COMPOSE_FILES ?? "compose.yaml";
const COMPOSE_ARGS = COMPOSE_FILES.split(",")
  .map((file) => file.trim())
  .filter(Boolean)
  .map((file) => `-f ${file}`)
  .join(" ");
const COMPOSE = `docker compose ${COMPOSE_ARGS}`.trim();

/**
 * Snapshot/restore-based DB reset. Strategy:
 *
 *   1. First call: capture the post-migration state of `rekenraam` as a
 *      baseline SQLite file. The baseline includes all migration
 *      seeds (the `personal` book, USD commodity, Cash account, $5000 opening
 *      balance — see [apps/api/alembic/versions/0001_initial_schema.py]).
 *
 *   2. Each subsequent call: stop the app, replace the working database with
 *      the baseline, and restart.
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
 * Cost: restart adds ~2s per spec, vs. ~30s for a full compose reset.
 */
let baselineCreated = false;

function composeExec(command: string): void {
  execSync(command, { stdio: ["ignore", "ignore", "pipe"] });
}

function runInAppVolume(shellCommand: string): void {
  execSync(
    `${COMPOSE} run --rm --no-deps --entrypoint sh ${API_SERVICE} -c ${JSON.stringify(shellCommand)}`,
    { stdio: ["ignore", "ignore", "pipe"] },
  );
}

function sqliteSiblingGlob(path: string): string {
  return `${path} ${path}-wal ${path}-shm ${path}-journal`;
}

function ensureSqliteBaseline(): void {
  if (baselineCreated) return;

  composeExec(`${COMPOSE} stop ${API_SERVICE}`);
  try {
    runInAppVolume(
      [
        "set -eu",
        `test -f ${JSON.stringify(SQLITE_DB_PATH)}`,
        `if [ ! -f ${JSON.stringify(SQLITE_BASELINE_PATH)} ]; then cp ${JSON.stringify(SQLITE_DB_PATH)} ${JSON.stringify(SQLITE_BASELINE_PATH)}; fi`,
        `rm -f ${SQLITE_BASELINE_PATH}-wal ${SQLITE_BASELINE_PATH}-shm ${SQLITE_BASELINE_PATH}-journal`,
      ].join("; "),
    );
    baselineCreated = true;
  } finally {
    composeExec(`${COMPOSE} start ${API_SERVICE}`);
    waitForApiHealthy();
  }
}

function resetSqliteDatabase(): void {
  ensureSqliteBaseline();

  composeExec(`${COMPOSE} stop ${API_SERVICE}`);
  try {
    runInAppVolume(
      [
        "set -eu",
        `rm -f ${sqliteSiblingGlob(SQLITE_DB_PATH)}`,
        `cp ${JSON.stringify(SQLITE_BASELINE_PATH)} ${JSON.stringify(SQLITE_DB_PATH)}`,
      ].join("; "),
    );
  } finally {
    composeExec(`${COMPOSE} start ${API_SERVICE}`);
    waitForApiHealthy();
  }
}

function ensureBaseline(): void {
  ensureSqliteBaseline();
}

function waitForApiHealthy(timeoutMs = 30_000): void {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const status = execSync(
        `${COMPOSE} ps --format json ${API_SERVICE}`,
        { encoding: "utf-8" },
      );
      if (status.includes('"Health":"healthy"') || status.includes('"State":"running"')) {
        // Once healthy, also poll the public health endpoint to be sure.
        try {
          execSync(`curl -fsS ${process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:16888"}/api/v1/health`, {
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
 * Reset the e2e database by restoring the baseline captured on first call.
 * After this returns, the API reports
 * `bootstrap_required: true` again with the seeded `personal` book intact.
 */
export function resetDatabase(): void {
  resetSqliteDatabase();
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
