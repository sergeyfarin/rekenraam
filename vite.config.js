import { defineConfig } from "vite";
import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";

const host = process.env.TAURI_DEV_HOST;

// https://vite.dev/config/
export default defineConfig(async () => ({
  plugins: [tailwindcss(), sveltekit()],

  // Keep the legacy fixed-port settings while src-tauri remains a migration reference.
  //
  // Keep command output visible during transitional desktop reference runs.
  clearScreen: false,
  // Use a fixed port for compatibility with the transitional desktop reference.
  server: {
    port: 1420,
    strictPort: true,
    host: host || false,
    hmr: host
      ? {
          protocol: "ws",
          host,
          port: 1421,
        }
      : undefined,
    watch: {
      // Avoid watching the legacy desktop reference tree.
      ignored: ["**/src-tauri/**"],
    },
  },
}));
