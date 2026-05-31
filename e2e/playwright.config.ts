import { defineConfig } from '@playwright/test';
import process from 'node:process';

const e2ePort = process.env.E2E_PORT ?? '16889';
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${e2ePort}`;

export default defineConfig({
  testDir: './playwright',
  use: {
    baseURL
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command:
          `cd .. && rm -f backend/var/e2e.sqlite backend/var/e2e.sqlite-shm backend/var/e2e.sqlite-wal && GOCACHE=\${GOCACHE:-/tmp/rekenraam-go-build-cache} pnpm build && APP_ENV=development DATABASE_URL=file:backend/var/e2e.sqlite HTTP_ADDR=127.0.0.1:${e2ePort} ./dist/rekenraam`,
        url: `${baseURL}/healthz`,
        timeout: 180_000,
        reuseExistingServer: false
      }
});
