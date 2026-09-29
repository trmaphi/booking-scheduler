import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  outputDir: process.env.E2E_OUTPUT_DIR ?? "./test-results",
  globalTeardown: "./e2e/global-teardown.ts",
  retries: 0,
  workers: 1,
  use: {
    baseURL: process.env.E2E_WEB_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
