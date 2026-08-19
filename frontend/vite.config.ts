import { sveltekit } from '@sveltejs/kit/vite';
import { paraglideVitePlugin } from '@inlang/paraglide-js';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { svelteAnnouncerCsp } from './vite/svelte-announcer-csp.js';

export default defineConfig({
  plugins: [
    tailwindcss(),
    paraglideVitePlugin({
      project: './project.inlang',
      outdir: './src/lib/paraglide',
      // Production is static output served by the Go binary, so a URL-segment
      // strategy would mean new routes and a change to the SPA fallback. This
      // order keeps locale selection entirely client-side: an explicit choice
      // wins, otherwise the browser's own preference, otherwise English.
      strategy: ['localStorage', 'preferredLanguage', 'baseLocale']
    }),
    sveltekit(),
    svelteAnnouncerCsp()
  ],
  server: {
    port: 1888,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:16888',
        xfwd: true
      }
    }
  }
});
