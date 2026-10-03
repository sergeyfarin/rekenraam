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
// The Trading 212 provider stub (stubs/trading212.mjs). The app is pointed at
// it with TRADING212_BASE_URL, which the backend refuses outside
// APP_ENV=development, so no real API key can ever be redirected (T-128).
const trading212StubPort = process.env.TRADING212_STUB_PORT ?? '16890';

export default defineConfig({
  testDir: './playwright',
  workers: 1,
  fullyParallel: false,
  // auth.spec.ts is the bootstrap journey: it needs a database with no owner
  // account, and every other spec creates one on its way in. That has always
  // been true and was held up only by "auth" sorting before every other
  // filename — which stopped being true the moment a spec starting with "a"
  // arrived. A project dependency states the requirement instead of leaving it
  // to alphabetical luck.
  projects: [
    { name: 'bootstrap', testMatch: /auth\.spec\.ts/ },
    {
      name: 'app',
      testIgnore: /auth\.spec\.ts/,
      dependencies: ['bootstrap']
    }
  ],
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
    : [
        {
          command: `TRADING212_STUB_PORT=${trading212StubPort} node stubs/trading212.mjs`,
          url: `http://127.0.0.1:${trading212StubPort}/healthz`,
          timeout: 10_000,
          reuseExistingServer: false
        },
        {
          command:
            `cd .. && rm -f backend/var/e2e.sqlite backend/var/e2e.sqlite-shm backend/var/e2e.sqlite-wal && GOCACHE=\${GOCACHE:-/tmp/rekenraam-go-build-cache} pnpm build && APP_ENV=development DATABASE_URL=file:backend/var/e2e.sqlite REKENRAAM_SECRET_KEY=${e2eSecretKey} TRADING212_BASE_URL=http://127.0.0.1:${trading212StubPort}/api/v0 HTTP_ADDR=127.0.0.1:${e2ePort} ./dist/rekenraam`,
          url: `${baseURL}/healthz`,
          timeout: 180_000,
          reuseExistingServer: false
        }
      ]
});
