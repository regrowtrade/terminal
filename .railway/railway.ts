import {
  defineRailway,
  github,
  postgres,
  preserve,
  project,
  service,
} from "railway/iac";

// No provider calls occur while evaluating this file. Review config plan before
// applying, and select a region where Binance Futures access is permitted.
export default defineRailway((ctx) => {
  const db = postgres("postgres");
  const source = github("regrowtrade/terminal", { branch: "main" });
  const engineBuild = {
    builder: "DOCKERFILE" as const,
    dockerfilePath: "infra/engine.Dockerfile",
  };
  const migrator = service("migrator", {
    source,
    build: engineBuild,
    start: "/usr/local/bin/migrate",
    replicas: 1,
    deploy: { restartPolicyType: "NEVER", sleepApplication: false },
    env: { MIGRATION_DATABASE_URL: db.env.DATABASE_URL },
  });
  const engine = service("engine", {
    source,
    build: engineBuild,
    healthcheck: "/readyz",
    replicas: 1,
    deploy: {
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 5,
      sleepApplication: false,
    },
    env: {
      PORT: "8080",
      ENGINE_ADDRESS: "[::]:8080",
      ENGINE_API_TOKEN: ctx.shared.ENGINE_API_TOKEN,
      DATABASE_REQUIRED: "true",
      // Never reference postgres.DATABASE_URL here: that is an admin credential.
      DATABASE_URL: preserve(),
    },
  });
  const web = service("web", {
    source,
    build: { builder: "DOCKERFILE", dockerfilePath: "infra/web.Dockerfile" },
    healthcheck: "/login",
    replicas: 1,
    deploy: {
      restartPolicyType: "ON_FAILURE",
      restartPolicyMaxRetries: 5,
      sleepApplication: false,
    },
    env: {
      PORT: "3000",
      APP_ORIGIN: preserve(),
      TERMINAL_PASSWORD: ctx.shared.TERMINAL_PASSWORD,
      SESSION_SECRET: ctx.shared.SESSION_SECRET,
      ENGINE_API_TOKEN: ctx.shared.ENGINE_API_TOKEN,
      ENGINE_URL: "http://${{engine.RAILWAY_PRIVATE_DOMAIN}}:8080",
      SESSION_BACKEND: "postgres",
    },
  });
  return project("regrow-terminal", { resources: [db, migrator, engine, web] });
});
