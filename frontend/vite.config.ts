import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    port: 16888,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:18888'
    }
  }
});
