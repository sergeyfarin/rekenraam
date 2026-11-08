import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [svelte({ compilerOptions: { runes: true } })],
  test: {
    environment: 'node',
    // Node 25+ exposes an experimental global localStorage that warns on every
    // access without --localstorage-file; keep the Node 24 behaviour (no global).
    execArgv: ['--no-experimental-webstorage'],
    include: ['src/**/*.test.ts']
  }
});
