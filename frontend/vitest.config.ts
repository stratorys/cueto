import { defineConfig } from "vitest/config";

// Separate from vite.config.ts: the analysis/* modules under test are pure (no
// DOM, no Vue plugin), so this config carries no plugins. Keeping it plugin-free
// also avoids a vite-version type clash between vitest's bundled vite and the
// project's vite 8.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
