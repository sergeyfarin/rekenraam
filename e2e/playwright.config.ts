import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './playwright',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:16888'
  }
});
