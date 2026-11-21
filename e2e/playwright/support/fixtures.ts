/**
 * One app instance per Playwright worker.
 *
 * Every spec signs in as the single owner and writes into one book, so specs
 * sharing a database have to take turns. Instead each worker starts its own
 * `dist/rekenraam` (built once in global-setup.ts) on its own port, against
 * its own SQLite file and backup directory, and `baseURL` points the worker's
 * pages at it. Workers then run side by side without seeing each other's
 * books.
 *
 * The database is keyed by project and parallel index, not by worker process:
 * a worker restarted after a failure picks up the same file, so files still
 * see one database for the whole run, as they did when the run shared one.
 * The `bootstrap` project gets a key of its own, which is what hands
 * auth.spec.ts the owner-less database it needs.
 *
 * With E2E_BASE_URL set, the external app is used as-is and nothing starts;
 * the config then holds the run to one worker, since that app has one book.
 *
 * Specs import `test` and `expect` from here, not from '@playwright/test'.
 */

import { test as base } from '@playwright/test';
import { spawn, type ChildProcess } from 'node:child_process';
import { createWriteStream, mkdirSync } from 'node:fs';
import { once } from 'node:events';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

export * from '@playwright/test';

/** Where a worker's app lives. `databasePath` is unset for an external app. */
export type Backend = { url: string; databasePath?: string };

export const repoRoot = fileURLToPath(new URL('../../../', import.meta.url));
/** Every worker's database, backups and log sit under here; global setup clears it. */
export const workerStateRoot = path.join(repoRoot, 'backend/var/e2e');
/** Prepared once by global setup; cases launch it without invoking the Go compiler. */
export const referenceDateServerPath = path.join(workerStateRoot, 'reference-date-server.test');
/**
 * Workers listen on consecutive ports from here. It stays clear of the stub
 * (16890) and of the old single-instance port (16889).
 */
const firstWorkerPort = Number(process.env.E2E_PORT ?? '16900');
const trading212StubPort = process.env.TRADING212_STUB_PORT ?? '16890';
// A fixed throwaway key, so the MFA journey has somewhere to seal the TOTP
// secret. Without it enrolment returns CONFIG_REQUIRED rather than storing the
// secret in the clear. This value protects nothing: it guards databases that
// are deleted at the start of every run.
const e2eSecretKey = process.env.REKENRAAM_SECRET_KEY ?? 'ZTJlLW9ubHktdGhyb3dhd2F5LWtleS0zMi1ieXRlcyE=';

export const test = base.extend<object, { backend: Backend }>({
  backend: [
    async ({}, use, workerInfo) => {
      if (process.env.E2E_BASE_URL) {
        await use({ url: process.env.E2E_BASE_URL });
        return;
      }

      const key = `${workerInfo.project.name}-${workerInfo.parallelIndex}`;
      const directory = path.join(workerStateRoot, key);
      mkdirSync(directory, { recursive: true });
      const port = firstWorkerPort + workerInfo.parallelIndex;
      const { app, url, databasePath, log, logPath } = launchApp(directory, port);
      // Support code that edits the database directly (legacy-dividend.ts)
      // runs in this worker process and finds the file here.
      process.env.E2E_DATABASE_PATH = databasePath;

      try {
        await waitForHealthy(app, url, logPath);
        await use({ url, databasePath });
      } finally {
        await stop(app);
        log.end();
      }
    },
    { scope: 'worker', timeout: 60_000 }
  ],
  baseURL: async ({ backend }, use) => {
    await use(backend.url);
  }
});

/**
 * Starts `dist/rekenraam` on `port` against `directory/rekenraam.sqlite`, with
 * its log in `directory/app.log`. Backups land beside the database. Callers
 * wait for health and stop the process.
 */
export function launchApp(directory: string, port: number) {
  const databasePath = path.join(directory, 'rekenraam.sqlite');
  const url = `http://127.0.0.1:${port}`;
  const logPath = path.join(directory, 'app.log');
  const log = createWriteStream(logPath, { flags: 'a' });
  const app = spawn(path.join(repoRoot, 'dist/rekenraam'), [], {
    cwd: repoRoot,
    env: {
      ...process.env,
      APP_ENV: 'development',
      DATABASE_URL: `file:${databasePath}`,
      HTTP_ADDR: `127.0.0.1:${port}`,
      REKENRAAM_SECRET_KEY: e2eSecretKey,
      // The Trading 212 stub (stubs/trading212.mjs), shared by every
      // worker: its accounts are keyed by API key and each spec registers
      // its own. The backend honors these overrides only in development,
      // so no real API key can be redirected (T-128). MARKET_DATA_BASE_URL
      // points every quote/FX provider at the stub, whose unknown paths
      // return 404, so background FX coverage never imports a live rate
      // that would change what a journey expects (#226).
      TRADING212_BASE_URL: `http://127.0.0.1:${trading212StubPort}/api/v0`,
      MARKET_DATA_BASE_URL: `http://127.0.0.1:${trading212StubPort}/market-data`,
      // Only the stub ever sees this key (MARKET_DATA_BASE_URL above).
      TWELVE_DATA_API_KEY: 'e2e-twelve-data',
      MASSIVE_API_KEY: 'e2e-massive'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  });
  app.stdout?.pipe(log);
  app.stderr?.pipe(log);
  return { app, url, databasePath, log, logPath };
}

export async function waitForHealthy(app: ChildProcess, url: string, logPath: string): Promise<void> {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (app.exitCode !== null) throw new Error(`e2e app exited with ${app.exitCode} before it was healthy; see ${logPath}`);
    try {
      if ((await fetch(`${url}/healthz`)).ok) return;
    } catch {
      // Not listening yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`e2e app at ${url} was not healthy within 30s; see ${logPath}`);
}

export async function stop(app: ChildProcess): Promise<void> {
  if (app.exitCode !== null || app.signalCode !== null) return;
  const exited = once(app, 'exit');
  app.kill('SIGTERM');
  const timer = setTimeout(() => app.kill('SIGKILL'), 10_000);
  await exited;
  clearTimeout(timer);
}
