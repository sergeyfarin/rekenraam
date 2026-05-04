// The current Docker frontend is served as a static SvelteKit build.
// A server adapter can replace this once the web runtime needs SSR.
import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      fallback: "index.html",
    }),
    alias: {
      "$lib": "./src/lib",
      "$lib/*": "./src/lib/*"
    }
  },
};

export default config;
