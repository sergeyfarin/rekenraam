import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    port: 1888,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:16888'
    }
  }
});
