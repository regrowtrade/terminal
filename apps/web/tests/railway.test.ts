import { expect, it } from "vitest";
import { createRailwayContext, project } from "railway/iac";
import configuration from "../../../.railway/railway";

it("keeps admin credentials out of the HTTP services and declares reviewed sources", async () => {
  const result = await configuration(
    createRailwayContext({ environment: "production" }),
    project,
  );
  const resources = result.resources?.flat() ?? [];
  const engine = resources.find((resource) => resource.name === "engine");
  const web = resources.find((resource) => resource.name === "web");
  const migrator = resources.find((resource) => resource.name === "migrator");
  expect(engine).toMatchObject({
    source: { repo: "regrowtrade/terminal", branch: "main" },
    deploy: {
      numReplicas: 1,
      healthcheckPath: "/readyz",
      sleepApplication: false,
    },
    variables: {
      DATABASE_URL: { type: "preserve" },
      DATABASE_REQUIRED: { type: "literal", value: "true" },
    },
  });
  expect(web).toMatchObject({
    variables: {
      ENGINE_URL: {
        type: "literal",
        value: "http://${{engine.RAILWAY_PRIVATE_DOMAIN}}:8080",
      },
    },
  });
  expect(engine).not.toHaveProperty("variables.MIGRATION_DATABASE_URL");
  expect(web).not.toHaveProperty("variables.DATABASE_URL");
  expect(web).not.toHaveProperty("variables.MIGRATION_DATABASE_URL");
  expect(migrator).toMatchObject({
    deploy: {
      startCommand: "/usr/local/bin/migrate",
      restartPolicyType: "NEVER",
    },
    variables: {
      MIGRATION_DATABASE_URL: {
        type: "reference",
        resource: "database.postgres",
        output: "DATABASE_URL",
      },
    },
  });
});
