import { defineConfig } from '@playwright/test';
import process from 'node:process';

const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:16888';

export default defineConfig({
  testDir: './playwright',
  use: {
    baseURL
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command:
          'cd .. && rm -f backend/var/e2e.sqlite backend/var/e2e.sqlite-shm backend/var/e2e.sqlite-wal && pnpm build && APP_ENV=development DATABASE_URL=file:backend/var/e2e.sqlite HTTP_ADDR=:16888 ./dist/rekenraam',
        url: `${baseURL}/healthz`,
        timeout: 180_000,
        reuseExistingServer: false
      }
});
