import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { cpSync, mkdirSync, readFileSync } from "node:fs";
import pg from "pg";
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
  SESSION_BACKEND: "postgres",
};
delete env.DATABASE_URL;
delete env.MIGRATION_DATABASE_URL;
delete env.MIGRATIONS_DIR;
delete env.TEST_DATABASE_URL;
// No production URLs: an explicit disposable loopback test server only.
let admin;
let setup;
let databaseName;
let roleName;
async function prepareDatabase() {
  if (!process.env.TEST_DATABASE_URL) return;
  const uri = new URL(process.env.TEST_DATABASE_URL);
  assert.ok(
    ["127.0.0.1", "localhost", "[::1]"].includes(uri.hostname),
    "Standalone tests require a disposable loopback database.",
  );
  assert.equal(uri.searchParams.get("host"), null);
  admin = new pg.Client({ connectionString: uri.href });
  await admin.connect();
  const suffix = randomBytes(12).toString("hex");
  databaseName = `terminal_smoke_${suffix}`;
  roleName = `terminal_role_${suffix}`;
  await admin.query(`CREATE DATABASE ${databaseName}`);
  uri.pathname = `/${databaseName}`;
  const migrationEnv = { ...env, MIGRATION_DATABASE_URL: uri.href };
  execFileSync("go", ["run", "./cmd/migrate"], {
    cwd: engineDirectory,
    env: migrationEnv,
    stdio: "inherit",
  });
  setup = new pg.Client({ connectionString: uri.href });
  await setup.connect();
  const password = randomBytes(48).toString("hex");
  await setup.query(
    `CREATE ROLE ${roleName} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS PASSWORD '${password}'`,
  );
  await setup.query(
    readFileSync(
      join(root, "database", "runtime-grants.sql"),
      "utf8",
    ).replaceAll("terminal_runtime", roleName),
  );
  uri.username = roleName;
  uri.password = password;
  env.DATABASE_URL = uri.href;
  env.DATABASE_REQUIRED = "true";
}

const children = [];
const startupErrors = [];
function start(command, args, cwd, childEnv = env) {
  const child = spawn(command, args, { cwd, env: childEnv, stdio: "inherit" });
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
  await prepareDatabase();
  start(binary, [], engineDirectory);
  const webEnv = { ...env };
  delete webEnv.DATABASE_URL;
  start(process.execPath, [join(standalone, "server.js")], standalone, webEnv);
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
      "X-Forwarded-Host": "untrusted.example",
    },
    body: new URLSearchParams({ password: env.TERMINAL_PASSWORD }),
  });
  assert.equal(
    login.status,
    303,
    "Login must redirect to the canonical origin.",
  );
  if (!env.DATABASE_URL) {
    assert.equal(
      login.headers.get("location"),
      `${origin}/login?error=unavailable`,
    );
    assert.equal(
      login.headers.has("set-cookie"),
      false,
      "Production must not issue stateless or local sessions without PostgreSQL.",
    );
    const direct = await fetch(`${engineOrigin}/v1/plans/preview`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${env.ENGINE_API_TOKEN}`,
      },
      body: JSON.stringify(planRequest),
    });
    assert.equal(direct.status, 200);
    assert.equal((await direct.json()).levels[0].stopPrice, "98");
    console.log(
      "PASS: standalone production fails closed without PostgreSQL; Go simulation preview verified. Full persisted-session test requires TEST_DATABASE_URL (CI).",
    );
  } else {
    assert.equal(
      login.headers.get("location"),
      `${origin}/`,
      "Proxy headers must not control login redirects.",
    );
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
    const secondLogin = await fetch(`${origin}/api/auth/login`, {
      method: "POST",
      redirect: "manual",
      headers: {
        Origin: origin,
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: new URLSearchParams({ password: env.TERMINAL_PASSWORD }),
    });
    const secondCookie = secondLogin.headers.get("set-cookie")?.split(";")[0];
    assert.ok(secondCookie, "A second device can sign in.");
    const logout = await fetch(`${origin}/api/auth/logout`, {
      method: "POST",
      redirect: "manual",
      headers: { Origin: origin, Cookie: setCookie.split(";")[0] },
    });
    assert.equal(logout.status, 303);
    assert.equal(
      (await preview(setCookie.split(";")[0])).status,
      401,
      "Copied cookie must fail after logout.",
    );
    assert.equal(
      (await preview(secondCookie)).status,
      200,
      "Other session survives individual logout.",
    );
    const logoutAll = await fetch(`${origin}/api/auth/logout-all`, {
      method: "POST",
      redirect: "manual",
      headers: { Origin: origin, Cookie: secondCookie },
    });
    assert.equal(logoutAll.status, 303);
    assert.equal(
      (await preview(secondCookie)).status,
      401,
      "All sessions revoked in PostgreSQL.",
    );
    console.log(
      "PASS: standalone production + real PostgreSQL: login, private preview, individual/global revocation and rejection of copied cookies. Synthetic data, no exchanges.",
    );
  }
} catch (error) {
  // Drivers/process errors may contain SQL or URLs: keep diagnostics generic.
  if (error instanceof assert.AssertionError) console.error(error.message);
  throw new Error(
    "Standalone verification failed; inspect the failed assertion using disposable local data, never production credentials.",
  );
} finally {
  await Promise.all(children.map(stop));
  if (setup) await setup.end();
  if (admin) {
    if (databaseName)
      await admin.query(`DROP DATABASE IF EXISTS ${databaseName} WITH (FORCE)`);
    if (roleName) await admin.query(`DROP ROLE IF EXISTS ${roleName}`);
    await admin.end();
  }
}
