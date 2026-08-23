import { defineConfig } from '@playwright/test';
import process from 'node:process';

const e2ePort = process.env.E2E_PORT ?? '16889';
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${e2ePort}`;
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE;
// A fixed throwaway key, so the MFA journey has somewhere to seal the TOTP
// secret. Without it enrolment returns CONFIG_REQUIRED rather than storing the
// secret in the clear. This value protects nothing: it guards a database that
// is deleted at the start of every run.
const e2eSecretKey = process.env.REKENRAAM_SECRET_KEY ?? 'ZTJlLW9ubHktdGhyb3dhd2F5LWtleS0zMi1ieXRlcyE=';

export default defineConfig({
  testDir: './playwright',
  workers: 1,
  fullyParallel: false,
  use: {
    baseURL,
    // Sandboxes and CI images often ship a Chromium that is not the revision
    // this Playwright release pins, and cannot download the pinned one. Point
    // `PLAYWRIGHT_CHROMIUM_EXECUTABLE` at the browser that is present instead
    // of hand-patching the browser cache. Unset, Playwright uses its own.
    launchOptions: chromiumExecutable ? { executablePath: chromiumExecutable } : undefined
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command:
          `cd .. && rm -f backend/var/e2e.sqlite backend/var/e2e.sqlite-shm backend/var/e2e.sqlite-wal && GOCACHE=\${GOCACHE:-/tmp/rekenraam-go-build-cache} pnpm build && APP_ENV=development DATABASE_URL=file:backend/var/e2e.sqlite REKENRAAM_SECRET_KEY=${e2eSecretKey} HTTP_ADDR=127.0.0.1:${e2ePort} ./dist/rekenraam`,
        url: `${baseURL}/healthz`,
        timeout: 180_000,
        reuseExistingServer: false
      }
});
