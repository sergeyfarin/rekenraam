import { defineConfig } from "vitest/config";
import { sveltekit } from "@sveltejs/kit/vite";
import { svelteTesting } from "@testing-library/svelte/vite";

export default defineConfig({
  plugins: [sveltekit(), svelteTesting()],
  test: {
    environment: "jsdom",
    globals: false,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.{test,spec}.ts"],
    alias: {
      "$app/state": "/src/test/mocks/app-state.ts",
      "$app/navigation": "/src/test/mocks/app-navigation.ts",
      "$app/environment": "/src/test/mocks/app-environment.ts",
    },
  },
});
