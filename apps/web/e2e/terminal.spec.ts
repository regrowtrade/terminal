import { expect, test, type Page } from "@playwright/test";

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
});

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
    page.getByText("REST manual · Solo lectura · Sin órdenes"),
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
