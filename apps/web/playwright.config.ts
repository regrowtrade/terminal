import { defineConfig, devices } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { randomBytes } from "node:crypto";

const testPassword = (process.env.REGROW_E2E_PASSWORD ??=
  randomBytes(48).toString("base64"));
const testSecret = randomBytes(48).toString("base64");
const engineToken = randomBytes(48).toString("base64");
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
      env: {
        ENGINE_API_TOKEN: engineToken,
        ENGINE_ADDRESS: "127.0.0.1:8081",
        DATABASE_URL: "",
        DATABASE_REQUIRED: "false",
        BINANCE_READ_ENABLED: "false",
        BINANCE_EGRESS_CONFIRMED: "false",
        CREDENTIAL_KEYS_JSON: "",
        CREDENTIAL_ACTIVE_KEY_ID: "",
      },
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
        SESSION_BACKEND: "local",
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
