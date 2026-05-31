import { sveltekit } from '@sveltejs/kit/vite';
import { paraglideVitePlugin } from '@inlang/paraglide-js';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [
    tailwindcss(),
    paraglideVitePlugin({
      project: './project.inlang',
      outdir: './src/lib/paraglide',
      strategy: ['baseLocale']
    }),
    sveltekit()
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
