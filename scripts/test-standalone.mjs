import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { cpSync, mkdirSync } from "node:fs";
import { createServer } from "node:net";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const root = fileURLToPath(new URL("../", import.meta.url));
const engineDirectory = join(root, "apps", "engine");
const webDirectory = join(root, "apps", "web");
const standalone = join(webDirectory, ".next", "standalone", "apps", "web");
const binary = join(
  root,
  ".local",
  "bin",
  process.platform === "win32" ? "standalone-test.exe" : "standalone-test",
);
mkdirSync(join(root, ".local", "bin"), { recursive: true });
execFileSync("go", ["build", "-o", binary, "./cmd/engine"], {
  cwd: engineDirectory,
  stdio: "inherit",
});
cpSync(
  join(webDirectory, ".next", "static"),
  join(standalone, ".next", "static"),
  { recursive: true },
);

async function availablePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const port = server.address().port;
  await new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
  return port;
}

const enginePort = await availablePort();
let webPort = await availablePort();
while (webPort === enginePort) webPort = await availablePort();
const origin = `http://127.0.0.1:${webPort}`;
const engineOrigin = `http://127.0.0.1:${enginePort}`;
const env = {
  ...process.env,
  NODE_ENV: "production",
  ENGINE_ADDRESS: `127.0.0.1:${enginePort}`,
  ENGINE_URL: engineOrigin,
  ENGINE_API_TOKEN: randomBytes(48).toString("base64"),
  TERMINAL_PASSWORD: randomBytes(48).toString("base64"),
  SESSION_SECRET: randomBytes(48).toString("base64"),
  APP_ORIGIN: origin,
  HOSTNAME: "127.0.0.1",
  PORT: String(webPort),
  DATABASE_REQUIRED: "false",
};
delete env.DATABASE_URL;
delete env.MIGRATION_DATABASE_URL;
delete env.MIGRATIONS_DIR;

const children = [];
const startupErrors = [];
function start(command, args, cwd) {
  const child = spawn(command, args, { cwd, env, stdio: "inherit" });
  child.once("error", () =>
    startupErrors.push("No se pudo iniciar un servicio de prueba."),
  );
  children.push(child);
  return child;
}

async function waitUntilReady(url) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (
      startupErrors.length ||
      children.some((child) => child.exitCode !== null)
    ) {
      throw new Error("Un servicio de prueba no pudo iniciar.");
    }
    try {
      const response = await fetch(url, {
        redirect: "manual",
        signal: AbortSignal.timeout(1000),
      });
      if (response.ok) return;
    } catch {
      /* Retry only a local readiness probe, never an order. */
    }
    await delay(100);
  }
  throw new Error("Los servicios de prueba no están listos.");
}

async function stop(child) {
  if (child.exitCode !== null || !child.pid) return;
  const exited = new Promise((resolve) => child.once("exit", resolve));
  child.kill();
  await Promise.race([exited, delay(5000)]);
  if (child.exitCode === null) child.kill("SIGKILL");
}

try {
  start(binary, [], engineDirectory);
  start(process.execPath, [join(standalone, "server.js")], standalone);
  await waitUntilReady(`${engineOrigin}/readyz`);
  await waitUntilReady(`${origin}/login`);
  const planRequest = {
    mode: "clean",
    symbol: "BTCUSDT",
    side: "LONG",
    entry: "100",
    quantity: "1",
    risk: "2",
    rewardRatio: "3",
    riskUnit: "amount",
    accountReference: "100",
    accountBasis: "equity",
    spacingPercent: "4",
    leverage: 10,
    maxLevels: 4,
    maxMargin: "100",
    costReserve: "0",
  };
  const preview = (cookie) =>
    fetch(`${origin}/api/plans/preview`, {
      method: "POST",
      redirect: "manual",
      signal: AbortSignal.timeout(10_000),
      headers: {
        Origin: origin,
        "Content-Type": "application/json",
        ...(cookie ? { Cookie: cookie } : {}),
      },
      body: JSON.stringify(planRequest),
    });
  assert.equal(
    (await preview()).status,
    401,
    "Anonymous access must be rejected.",
  );
  const login = await fetch(`${origin}/api/auth/login`, {
    method: "POST",
    redirect: "manual",
    signal: AbortSignal.timeout(10_000),
    headers: {
      Origin: origin,
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams({ password: env.TERMINAL_PASSWORD }),
  });
  assert.equal(login.status, 303, "Login must succeed.");
  const setCookie = login.headers.get("set-cookie");
  assert.ok(
    setCookie?.startsWith("__Host-regrow-session="),
    "Use a production host cookie.",
  );
  assert.ok(
    /; secure/i.test(setCookie) &&
      /; httponly/i.test(setCookie) &&
      /; samesite=strict/i.test(setCookie),
    "Use protected session flags.",
  );
  // Node explicitly sends the test cookie on loopback HTTP. Browser production
  // cookies still require HTTPS; this does not weaken application settings.
  const response = await preview(setCookie.split(";")[0]);
  assert.equal(response.status, 200, "Private preview must succeed.");
  const text = await response.text();
  assert.equal(
    text.includes(env.ENGINE_API_TOKEN),
    false,
    "Never return the engine token.",
  );
  const plan = JSON.parse(text);
  assert.equal(plan.simulation, true);
  assert.equal(plan.levels[0].stopPrice, "98");
  assert.equal(plan.levels[0].takeProfit, "106");
  console.log(
    "PASS: standalone production login and private Go preview; synthetic data, no exchanges.",
  );
} finally {
  await Promise.all(children.map(stop));
}
