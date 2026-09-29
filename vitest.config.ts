import { configDefaults, defineConfig } from "vitest/config";
export default defineConfig({
  esbuild: {
    jsx: "automatic",
  },
  resolve: {
    alias: { "@": new URL("./src", import.meta.url).pathname },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    css: true,
    exclude: [...configDefaults.exclude, "e2e/**"],
  },
});
