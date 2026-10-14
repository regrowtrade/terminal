import { defineConfig, devices } from "@playwright/test";
import { fileURLToPath } from "node:url";

const testPassword = "e2e-only-private-password-never-deploy";
const testSecret = "e2e-only-session-secret-never-deploy-123456";
const engineToken = "e2e-only-engine-token-never-deploy-123456";
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    channel: process.env.PLAYWRIGHT_CHANNEL,
    baseURL: "http://localhost:3100",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    {
      name: "mobile",
      use: { ...devices["iPhone 13"], defaultBrowserType: "chromium" },
    },
  ],
  webServer: [
    {
      command: "go run ./cmd/engine",
      cwd: fileURLToPath(new URL("../engine", import.meta.url)),
      url: "http://127.0.0.1:8081/healthz",
      env: { ENGINE_API_TOKEN: engineToken, ENGINE_ADDRESS: "127.0.0.1:8081" },
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "npm run dev -- --hostname localhost --port 3100",
      url: "http://localhost:3100/login",
      env: {
        TERMINAL_PASSWORD: testPassword,
        SESSION_SECRET: testSecret,
        ENGINE_API_TOKEN: engineToken,
        ENGINE_URL: "http://127.0.0.1:8081",
        APP_ORIGIN: "http://localhost:3100",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
