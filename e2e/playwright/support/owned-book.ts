import { mkdtempSync } from 'node:fs';
import { createServer } from 'node:net';
import path from 'node:path';
import process from 'node:process';
import { test as base, expect, launchApp, stop, waitForHealthy, workerStateRoot } from './fixtures';

/**
 * A fresh app and database for one test (#398).
 *
 * The worker's book is shared by every file the worker runs, so a book-wide
 * read — a self-check run status, its capped samples, its counts — depends on
 * whatever earlier files left behind. A test that asserts one of those facts
 * uses this `test` instead: its pages talk to an app nobody else writes to, so
 * every finding in the run is one the test produced. Per-transaction reads
 * (the attribution diagnostic) are already owned and stay on the worker book.
 *
 * An external E2E_BASE_URL app has one book, so these tests skip there.
 */
export const test = base.extend<{ ownedBook: { url: string; databasePath: string }; ownedBookURL: string; ownedBookDatabasePath: string }>({
  ownedBook: [async ({}, use, testInfo) => {
    testInfo.skip(Boolean(process.env.E2E_BASE_URL), 'needs a book of its own; an external app has one book');
    // Under the worker state root, so global setup clears it and the log
    // survives a failure for inspection.
    const directory = mkdtempSync(path.join(workerStateRoot, 'owned-'));
    const { app, url, databasePath, log, logPath } = launchApp(directory, await freePort());
    try {
      await waitForHealthy(app, url, logPath);
      await use({ url, databasePath });
    } finally {
      await stop(app);
      log.end();
    }
  }, { timeout: 60_000 }],
  ownedBookURL: async ({ ownedBook }, use) => { await use(ownedBook.url); },
  // For seeding shapes the API cannot build quickly, such as thousands of
  // accounts (#518); see bulk-accounts.ts.
  ownedBookDatabasePath: async ({ ownedBook }, use) => { await use(ownedBook.databasePath); },
  baseURL: async ({ ownedBookURL }, use) => { await use(ownedBookURL); }
});
export { expect };

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  await new Promise((resolve) => server.close(resolve));
  if (!address || typeof address === 'string') throw new Error('no free port');
  return address.port;
}
