import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { constants } from 'node:fs';
import { access, mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test as base, expect, referenceDateServerPath, repoRoot } from './fixtures';

// A reference read the server forgot to pin falls back to SQLite's
// date('now'), which follows the real clock, not the frozen one; a hard-coded
// instant stops exposing that fallback once the real date passes it. A spec
// reads the real UTC day once (realUTCToday) and derives its instant and every
// expected date from that one reading (utcDay): two clock reads put them on
// different days when UTC midnight falls between them.
export function realUTCToday(): number {
  const now = new Date();
  return Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
}

// The UTC calendar day offset days from today (a realUTCToday reading).
export function utcDay(today: number, days: number): string {
  return new Date(today + days * 86_400_000).toISOString().slice(0, 10);
}

// The fixed clock exists only in a Go test executable; the production binary
// keeps its real clock. Each case gets a real HTTP server and isolated SQLite
// file, serving the same built frontend with the normal API/auth middleware.
export const test = base.extend<{ referenceInstant: string; fixedReferenceURL: string }>({
  referenceInstant: [`${utcDay(realUTCToday(), 7)}T23:01:00Z`, { option: true }],
  fixedReferenceURL: [async ({ referenceInstant }, use) => {
    await access(referenceDateServerPath, constants.X_OK);
    const directory = await mkdtemp(path.join(tmpdir(), 'rekenraam-reference-date-'));
    const addressFile = path.join(directory, 'address');
    const server = spawn(referenceDateServerPath, [
      '-test.run=^TestOwnerReferenceBrowserServer$', '-test.timeout=3m'
    ], {
      cwd: repoRoot,
      detached: true,
      env: {
        ...process.env,
        TMPDIR: directory,
        REKENRAAM_REFERENCE_BROWSER_ADDRESS_FILE: addressFile,
        REKENRAAM_REFERENCE_BROWSER_INSTANT: referenceInstant
      },
      stdio: ['ignore', 'pipe', 'pipe']
    });
    const closed = once(server, 'close');
    let output = '';
    server.stdout?.on('data', chunk => { output += chunk; });
    server.stderr?.on('data', chunk => { output += chunk; });
    try {
      let url = '';
      await expect.poll(async () => {
        if (server.exitCode !== null || server.signalCode !== null) throw new Error(`fixed-date server exited: ${output}`);
        try { url = await readFile(addressFile, 'utf8'); } catch { return false; }
        try { return (await fetch(`${url}/healthz`)).ok; } catch { return false; }
      }, { timeout: 45_000 }).toBe(true);
      await use(url);
    } catch (error) {
      throw new Error(`fixed-date server failed: ${output}`, { cause: error });
    } finally {
      if (server.pid && server.exitCode === null && server.signalCode === null) process.kill(-server.pid, 'SIGTERM');
      await closed;
      await rm(directory, { recursive: true, force: true });
    }
  }, { timeout: 60_000 }],
  baseURL: async ({ fixedReferenceURL }, use) => { await use(fixedReferenceURL); }
});
export { expect };
