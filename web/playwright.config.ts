import { defineConfig } from "@playwright/test";
import { existsSync } from "node:fs";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: process.env.E2E_BASE_URL,
    headless: true,
    viewport: { width: 1440, height: 1000 },
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : existsSync("/Applications/Google Chrome.app")
        ? { channel: "chrome" }
        : {},
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
