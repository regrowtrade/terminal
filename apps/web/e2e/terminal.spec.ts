import { expect, test, type Page } from "@playwright/test";
import {
  syntheticSnapshot,
  syntheticAccountSnapshot,
} from "../tests/fixtures/binance";
import { syntheticComparison } from "../tests/fixtures/comparison";
import { syntheticOrderLookup } from "../tests/fixtures/order-lookup";
import { syntheticReconciliation } from "../tests/fixtures/reconciliation";
import {
  syntheticIntentAccount,
  syntheticIntentId,
  syntheticPlanId,
  syntheticSavedPlan,
  syntheticSavedIntent,
  syntheticIntentObservation,
} from "../tests/fixtures/intent-investigation";

async function login(page: Page) {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login/);
  await page
    .getByLabel("Contraseña de acceso")
    .fill(process.env.REGROW_E2E_PASSWORD!);
  await page.getByRole("button", { name: "Entrar a mi terminal" }).click();
  await expect(
    page.getByRole("heading", { name: "Tu operativa, en perspectiva." }),
  ).toBeVisible();
}

test("requires login and protects the preview API", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login/);
  const response = await request.post("/api/plans/preview", { data: {} });
  expect(response.status()).toBe(401);
  for (const operation of [
    "list",
    "snapshot",
    "latest",
    "comparison",
    "events",
    "reconcile",
    "order",
    "fills/recover",
  ]) {
    const reading = await request.post(
      `/api/connections/binance/${operation}`,
      { data: { accountId: "00000000-0000-4000-8000-000000000001" } },
    );
    expect(reading.status()).toBe(401);
  }
  for (const operation of ["observe", "observations"]) {
    const response = await request.post(`/api/plans/intents/${operation}`, {
      data: {
        accountId: syntheticIntentAccount.id,
        intentId: syntheticIntentId,
      },
    });
    expect(response.status()).toBe(401);
  }
});

for (const enabled of [true, false]) {
  test(`saved intent investigation is explicit and historical evidence works with reader ${enabled ? "enabled" : "disabled"}`, async ({
    page,
  }) => {
    await login(page);
    const requests: string[] = [];
    const other = "00000000-0000-4000-8000-000000000099";
    await page.route("**/api/connections/binance/list", (route) =>
      route.fulfill({
        json: {
          enabled,
          accounts: [
            syntheticIntentAccount,
            { ...syntheticIntentAccount, id: other, name: "Other synthetic" },
          ],
        },
      }),
    );
    let reads = 0;
    await page.route("**/api/plans/**", (route) => {
      const operation = new URL(route.request().url()).pathname.replace(
        "/api/plans/",
        "",
      );
      requests.push(operation);
      const input = route.request().postDataJSON();
      if (operation === "list")
        return route.fulfill({
          json: input.accountId === other ? [] : [syntheticSavedPlan()],
        });
      if (operation === "intents/list") {
        expect(input).toEqual({
          accountId: syntheticIntentAccount.id,
          planId: syntheticPlanId,
        });
        return route.fulfill({
          json: [
            syntheticSavedIntent(),
            {
              ...syntheticSavedIntent(),
              id: "00000000-0000-4000-8000-000000000098",
              family: "conditional",
              purpose: "TP",
            },
          ],
        });
      }
      expect(input).toEqual({
        accountId: syntheticIntentAccount.id,
        intentId: syntheticIntentId,
      });
      if (operation === "intents/observations")
        return route.fulfill({
          json: enabled ? [syntheticIntentObservation()] : [],
        });
      if (operation === "intents/observe") {
        expect(enabled).toBe(true);
        reads++;
        const item = syntheticIntentObservation();
        if (reads === 2) {
          item.report.assessment = "unresolved";
          item.report.observation.outcome = "unresolved";
          item.report.observation.reason = "exchange_not_found";
          item.report.observation.order = null;
        }
        if (reads > 2)
          return route.fulfill({
            status: 409,
            json: { error: "synthetic changed decision" },
          });
        return route.fulfill({ json: item });
      }
      return route.fulfill({
        status: 503,
        json: { error: "Unexpected synthetic operation" },
      });
    });
    await page.goto("/settings");
    const saved = page.locator('[aria-label="Planes guardados"]');
    await expect(saved.getByText(/BTCUSDT · CLEAN/)).toBeVisible();
    expect(requests).toEqual(["list"]);
    await saved
      .getByRole("button", { name: /Ver intenciones del plan/ })
      .click();
    await expect(saved.getByText(/Investigación no disponible/)).toBeVisible();
    await expect(
      saved.getByRole("button", { name: /Investigar TP/ }),
    ).toHaveCount(0);
    await saved.getByRole("button", { name: /Investigar ENTRY/ }).click();
    const panel = saved.getByRole("region", {
      name: "Investigación de intención",
    });
    await expect(panel).toBeVisible();
    expect(requests).toEqual(["list", "intents/list"]);
    const observe = panel.getByRole("button", {
      name: "Consultar y guardar observación",
    });
    if (enabled) {
      await observe.click();
      await expect(panel.getByText(/ATRIBUTOS COINCIDENTES/)).toContainText(
        "PROPIEDAD NO CONFIRMADA",
      );
      await expect(panel.getByText(/9007199254740993/)).toBeVisible();
      await expect(
        panel.getByText("2.000 / 0.500000000000000001", { exact: true }),
      ).toBeVisible();
      await expect(
        panel.getByText("100.000 / 99.123456789012345678", { exact: true }),
      ).toBeVisible();
    } else await expect(observe).toBeDisabled();
    await panel
      .getByRole("button", { name: "Ver últimas 20 observaciones" })
      .click();
    await expect(
      panel.getByText(/HISTORIAL GUARDADO · NO ACTUAL/),
    ).toBeVisible();
    await expect(panel.getByText(/OBSERVACIÓN PUNTUAL GUARDADA/)).toHaveCount(
      0,
    );
    if (enabled) {
      await observe.click();
      await expect(panel.getByText(/NO RESUELTA/)).toContainText("No reenvíes");
      await expect(panel.getByText(/HISTORIAL GUARDADO/)).toHaveCount(0);
      await observe.click();
      await expect(panel.getByRole("alert")).toContainText(
        "La intención o su plan cambió",
      );
      await expect(panel.getByText(/NO RESUELTA/)).toHaveCount(0);
    } else
      await expect(
        panel.getByText(/No hay observaciones guardadas/),
      ).toContainText("No demuestra ausencia");
    await expect(
      panel.getByRole("button", { name: /ejecutar|reenviar|enviar orden/i }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await saved.getByLabel("Cuenta", { exact: true }).selectOption(other);
    await expect(panel).toHaveCount(0);
    await expect(saved.getByText(/Sin planes guardados/)).toBeVisible();
  });
}

test("dashboard and a real Go preview work without executing orders", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await login(page);
  await expect(page.getByText("Motor disponible")).toBeAttached();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "Nuevo plan" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Calcular vista previa" }).click();
  await expect(dialog.getByText("NO EJECUTADO")).toBeVisible();
  await expect(
    dialog.getByRole("cell", { name: "98,0000", exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("cell", { name: "106,0000", exact: true }),
  ).toBeVisible();
  if (test.info().project.name === "mobile")
    await dialog.getByRole("button", { name: "Cerrar calculadora" }).click();
  else await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("button", { name: "Nuevo plan" })).toBeFocused();
  expect(errors).toEqual([]);
  await page.screenshot({
    path: `test-results/dashboard-${test.info().project.name}.png`,
    fullPage: true,
  });
});

test("risk is recalculated at a recovered hedge snapshot", async ({ page }) => {
  await login(page);
  await page.getByRole("link", { name: "Estrategias", exact: true }).click();
  await page.getByRole("button", { name: /Cobertura/ }).click();
  await page.getByLabel("PnL realizado de toda la operación").fill("1");
  await page.getByRole("button", { name: /Usar adicional sugerido/ }).click();
  await expect(page.getByLabel("Riesgo adicional autorizado")).toHaveValue("1");
  await page.getByRole("button", { name: "Calcular vista previa" }).click();
  await expect(page.getByText("NO EJECUTADO")).toBeVisible();
  await expect(
    page.getByText("Resultado de referencia", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: `test-results/hedge-${test.info().project.name}.png`,
    fullPage: true,
  });
});

test("position filters work and logout removes access", async ({ page }) => {
  await login(page);
  const copiedCookies = await page.context().cookies();
  await page.getByRole("link", { name: "Posiciones", exact: true }).click();
  await page
    .getByRole("button", { name: "Ejemplos de simulación", exact: true })
    .click();
  await page.getByRole("textbox", { name: "Buscar activo" }).fill("missing");
  await expect(page.getByRole("status")).toHaveText(
    "No hay ejemplos que coincidan con tu búsqueda.",
  );
  await page.getByRole("button", { name: "Cerrar sesión" }).click();
  await expect(page).toHaveURL(/\/login/);
  await page.context().addCookies(copiedCookies);
  await page.goto("/strategies");
  await expect(page).toHaveURL(/\/login/);
});

test("settings keeps live access disabled for local sessions and never requests keys", async ({
  page,
}) => {
  await login(page);
  await page.goto("/settings");
  await expect(
    page.getByText("REST manual · Solo lectura · Sin ejecución"),
  ).toBeVisible();
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  await expect(
    page
      .getByRole("alert")
      .filter({ hasText: "requieren sesiones PostgreSQL" }),
  ).toContainText("requieren sesiones PostgreSQL");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("synthetic read-only orders and filters render without exchange access", async ({
  page,
}) => {
  await login(page);
  // Browser-only fixtures exercise rendering. They do not enable the engine
  // adapter, supply keys or send any requests to Binance.
  await page.route("**/api/connections/binance/list", (route) =>
    route.fulfill({
      json: {
        enabled: true,
        accounts: [
          {
            id: "00000000-0000-4000-8000-000000000001",
            name: "Synthetic",
            exchange: "binance",
            environment: "production",
          },
        ],
      },
    }),
  );
  let reads = 0;
  await page.route("**/api/connections/binance/snapshot", (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({ json: syntheticSnapshot() })
      : route.fulfill({
          status: 503,
          json: { error: "Lectura sintética no disponible." },
        });
  });
  await page.route("**/api/connections/binance/latest", (route) =>
    route.fulfill({
      json: {
        snapshot: {
          ...syntheticSnapshot(),
          startedAt: "2020-01-01T00:00:00Z",
          capturedAt: "2020-01-01T00:00:01Z",
        },
        storedAt: "2020-01-01T00:00:02Z",
      },
    }),
  );
  await page.goto("/settings");
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Consultar cuenta y órdenes", exact: true })
    .click();
  const reading = page.getByRole("region", { name: "Lectura de Binance" });
  await expect(
    reading.getByRole("cell", { name: /9007199254740993/ }),
  ).toBeVisible();
  await expect(
    reading.getByRole("cell", { name: /9007199254740995/ }),
  ).toBeVisible();
  await reading.getByText(/Instrumentos y filtros USDT perpetuos/).click();
  await expect(
    reading.getByRole("table", { name: /Filtros publicados/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: /enviar orden|ejecutar orden|cancelar orden/i,
    }),
  ).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Consultar cuenta y órdenes", exact: true })
    .click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Lectura sintética" }),
  ).toBeVisible();
  await expect(reading).toHaveCount(0);
  await page
    .getByRole("button", {
      name: "Mostrar última lectura guardada",
      exact: true,
    })
    .click();
  await expect(page.getByText(/LECTURA GUARDADA · NO ACTUAL/)).toBeVisible();
  await expect(page.getByText(/Recuperada de PostgreSQL/)).toBeVisible();
});

test("normal-order lookup shows point-in-time evidence and unresolved absence without trading", async ({
  page,
}) => {
  await login(page);
  const accountId = "00000000-0000-4000-8000-000000000001";
  await page.route("**/api/connections/binance/list", (route) =>
    route.fulfill({
      json: {
        enabled: true,
        accounts: [
          {
            id: accountId,
            name: "Synthetic",
            exchange: "binance",
            environment: "production",
          },
        ],
      },
    }),
  );
  let reads = 0;
  await page.route("**/api/connections/binance/order", (route) => {
    reads++;
    expect(route.request().postDataJSON()).toEqual({
      accountId,
      symbol: "BTCUSDT",
      orderId: "9007199254740993",
    });
    const body = syntheticOrderLookup();
    if (reads > 1) {
      body.outcome = "unresolved";
      body.reason = "exchange_not_found";
      body.order = null;
    }
    return route.fulfill({ json: body });
  });
  await page.goto("/settings");
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  const panel = page.getByRole("region", { name: "Consulta de orden" });
  await expect(panel).toBeVisible();
  expect(reads).toBe(0);
  await panel.getByLabel("Símbolo USDT").fill("BTCUSDT");
  await panel.getByLabel("Identificador de orden").fill("9007199254740993");
  await panel.getByRole("button").click();
  await expect(panel.getByText(/OBSERVACIÓN PUNTUAL GUARDADA/)).toBeVisible();
  await expect(panel.getByText("90.00000001 / 89.12345678")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await panel.getByRole("button").click();
  await expect(panel.getByText(/NO RESUELTA/)).toContainText("No reenvíes");
  await expect(panel.getByText("90.00000001 / 89.12345678")).toHaveCount(0);
  await expect(
    panel.getByRole("button", { name: /enviar|cancelar|ejecutar/i }),
  ).toHaveCount(0);
  await panel.getByLabel("Identificador de orden").fill("123");
  await expect(panel.getByText(/NO RESUELTA/)).toHaveCount(0);
});

test("account reconciliation and fill recovery stay explicit, bounded and read-only", async ({
  page,
}) => {
  await login(page);
  const accountId = "00000000-0000-4000-8000-000000000001";
  const requests: string[] = [];
  await page.route("**/api/connections/binance/**", (route) => {
    const operation = new URL(route.request().url()).pathname.replace(
      "/api/connections/binance/",
      "",
    );
    requests.push(operation);
    if (operation === "list")
      return route.fulfill({
        json: {
          enabled: true,
          accounts: [accountId, "00000000-0000-4000-8000-000000000002"].map(
            (id, i) => ({
              id,
              name: `Synthetic ${i}`,
              exchange: "binance",
              environment: "production",
            }),
          ),
        },
      });
    if (operation === "reconcile") {
      expect(route.request().postDataJSON()).toEqual({ accountId });
      const report = syntheticReconciliation();
      report.fills[0].commission = "-0.001000000000000001";
      return route.fulfill({
        json: {
          ...report,
          continuity: "gapped",
          eventsTruncated: true,
          fillsTruncated: true,
          totalFills: 250,
        },
      });
    }
    if (operation === "fills/recover") {
      const query = {
        accountId,
        symbol: "BTCUSDT",
        startTime: Date.parse("2020-01-01T00:00:00Z"),
        endTime: Date.parse("2020-01-02T00:00:00Z"),
      };
      expect(route.request().postDataJSON()).toEqual(query);
      return route.fulfill({
        json: {
          source: "binance-production",
          mode: "read_only",
          executionReady: false,
          symbol: query.symbol,
          startTime: query.startTime,
          endTime: query.endTime,
          recovered: 1,
          duplicates: 2,
          truncated: true,
        },
      });
    }
    return route.fulfill({
      status: 503,
      json: { error: "Unexpected synthetic operation" },
    });
  });
  await page.goto("/positions");
  expect(requests).toEqual([]);
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  const panel = page.getByRole("region", { name: "Reconciliación de cuenta" });
  await expect(panel).toBeVisible();
  expect(requests).toEqual(["list"]);
  const reportButton = panel.getByRole("button", {
    name: "Consultar reporte de reconciliación",
  });
  await reportButton.click();
  await expect(panel.getByText(/CONTINUIDAD CON HUECOS/)).toBeVisible();
  await expect(panel.getByText(/EVENTOS TRUNCADOS/)).toBeVisible();
  await expect(panel.getByText(/FILLS TRUNCADOS/)).toBeVisible();
  await expect(
    panel.getByRole("cell", { name: "9007199254740993 / 12345678" }),
  ).toBeVisible();
  await expect(
    panel.getByRole("cell", { name: "0.0005", exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByRole("cell", { name: "-0.001000000000000001 / USDT" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await panel.getByLabel("Símbolo de recuperación").fill("BTCUSDT");
  await panel.getByLabel("Inicio de ventana (UTC)").fill("2020-01-01T00:00");
  await panel.getByLabel("Fin de ventana (UTC)").fill("2020-01-02T00:00");
  await panel
    .getByRole("button", { name: "Recuperar y guardar fills" })
    .click();
  await expect(panel.getByText(/RECUPERACIÓN GUARDADA/)).toBeVisible();
  await expect(panel.getByText(/RECUPERACIÓN TRUNCADA/)).toBeVisible();
  await expect(panel.getByText(/REPORTE PUNTUAL/)).toHaveCount(0);
  expect(requests).toEqual(["list", "reconcile", "fills/recover"]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await reportButton.click();
  await expect(panel.getByText(/REPORTE PUNTUAL/)).toBeVisible();
  await expect(
    panel.getByRole("button", {
      name: /enviar orden|cancelar orden|ejecutar/i,
    }),
  ).toHaveCount(0);
  await page
    .getByLabel("Cuenta de lectura", { exact: true })
    .selectOption("00000000-0000-4000-8000-000000000002");
  await expect(panel.getByRole("table")).toHaveCount(0);
  await expect(panel.getByLabel("Símbolo de recuperación")).toHaveValue("");
  expect(requests).toEqual(["list", "reconcile", "fills/recover", "reconcile"]);
});

test("settings can revoke every existing session", async ({
  page,
  browser,
}) => {
  await login(page);
  const otherContext = await browser.newContext({
    baseURL: "http://localhost:3100",
  });
  try {
    const otherPage = await otherContext.newPage();
    await login(otherPage);
    await page.goto("/settings");
    await page
      .getByRole("button", { name: "Cerrar todas las sesiones", exact: true })
      .click();
    await expect(page).toHaveURL(/\/login/);
    await otherPage.goto("/strategies");
    await expect(otherPage).toHaveURL(/\/login/);
  } finally {
    await otherContext.close();
  }
});

test("account positions remain separate from examples and preserve historical precision", async ({
  page,
}) => {
  await login(page);
  const requests: string[] = [];
  await page.route("**/api/connections/binance/**", (route) => {
    const operation = new URL(route.request().url()).pathname
      .split("/")
      .at(-1)!;
    requests.push(operation);
    if (operation === "list")
      return route.fulfill({
        json: {
          enabled: false,
          accounts: [
            {
              id: "00000000-0000-4000-8000-000000000001",
              name: "Synthetic one",
              exchange: "binance",
              environment: "production",
            },
            {
              id: "00000000-0000-4000-8000-000000000002",
              name: "Synthetic two",
              exchange: "binance",
              environment: "production",
            },
          ],
        },
      });
    if (operation === "latest") {
      const snapshot = syntheticAccountSnapshot();
      snapshot.startedAt = "2020-01-01T00:00:00Z";
      snapshot.capturedAt = "2020-01-01T00:00:01Z";
      return route.fulfill({
        json: { snapshot, storedAt: "2020-01-01T00:00:02Z" },
      });
    }
    return route.fulfill({
      status: 503,
      json: { error: "Unexpected operation in offline test" },
    });
  });
  await page.goto("/positions");
  await expect(
    page.getByRole("button", { name: "Cuenta Binance · Solo lectura" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("table")).toHaveCount(0);
  expect(requests).toEqual([]);
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Consultar cuenta y órdenes" }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Mostrar última lectura guardada" })
    .click();
  await expect(page.getByText(/LECTURA GUARDADA · NO ACTUAL/)).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "1.000000000000000001", exact: true }),
  ).toBeVisible();
  await page
    .getByLabel("Lado de posición", { exact: true })
    .selectOption("SHORT");
  await expect(
    page.getByRole("cell", { name: "-2.000000000000000001", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "1.000000000000000001", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("cell", { name: /9007199254740993/ }),
  ).toBeVisible();
  await page
    .getByRole("searchbox", { name: "Buscar símbolo", exact: true })
    .fill("missing");
  await expect(page.getByText(/No hay posiciones que coincidan/)).toBeVisible();
  expect(requests).toEqual(["list", "latest"]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "Quitar filtros" }).click();
  await page
    .getByLabel("Cuenta de lectura", { exact: true })
    .selectOption("00000000-0000-4000-8000-000000000002");
  await expect(page.getByRole("table")).toHaveCount(0);
  await page
    .getByRole("button", { name: "Ejemplos de simulación", exact: true })
    .click();
  await expect(
    page.getByRole("table", { name: /Posiciones sintéticas/ }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Cuenta Binance · Solo lectura", exact: true })
    .click();
  await expect(page.getByRole("table")).toHaveCount(0);
  await expect(
    page.getByLabel("Cuenta de lectura", { exact: true }),
  ).toHaveCount(0);
  expect(requests).toEqual(["list", "latest"]);
});

test("synthetic comparison preserves exact values and unknown missing-order state", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/connections/binance/list", (route) =>
    route.fulfill({
      json: {
        enabled: false,
        accounts: [
          {
            id: "00000000-0000-4000-8000-000000000001",
            name: "Synthetic one",
            exchange: "binance",
            environment: "production",
          },
          {
            id: "00000000-0000-4000-8000-000000000002",
            name: "Synthetic two",
            exchange: "binance",
            environment: "production",
          },
        ],
      },
    }),
  );
  let requests = 0;
  await page.route("**/api/connections/binance/comparison", (route) => {
    requests++;
    return requests === 1
      ? route.fulfill({ json: syntheticComparison() })
      : route.fulfill({
          status: 503,
          json: { error: "Comparación sintética no disponible." },
        });
  });
  await page.goto("/settings");
  await page
    .getByRole("button", { name: "Comprobar conexión", exact: true })
    .click();
  await page
    .getByRole("button", {
      name: "Ver cambios de la última lectura",
      exact: true,
    })
    .click();
  const region = page.getByRole("region", {
    name: "Comparación guardada de Binance",
  });
  await expect(region.getByRole("status")).toHaveText(
    "COMPARACIÓN GUARDADA · NO ESTADO ACTUAL",
  );
  await expect(
    region.getByRole("cell", { name: "1.000000000000000001", exact: true }),
  ).toBeVisible();
  await expect(region.getByText(/estado final desconocido/)).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page
    .getByLabel("Cuenta de lectura")
    .selectOption("00000000-0000-4000-8000-000000000002");
  await expect(region).toHaveCount(0);
  await page
    .getByRole("button", {
      name: "Ver cambios de la última lectura",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Comparación sintética" }),
  ).toBeVisible();
  await expect(region).toHaveCount(0);
});
