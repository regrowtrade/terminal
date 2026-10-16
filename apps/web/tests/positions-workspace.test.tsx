import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToString } from "react-dom/server";
import { afterEach, expect, it, vi } from "vitest";
import { PositionsWorkspace } from "@/components/positions-workspace";
import { BinanceAccountData } from "@/components/binance-account-data";
import { syntheticAccountSnapshot } from "./fixtures/binance";

afterEach(() => vi.unstubAllGlobals());
const accounts = [
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
];
async function list(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Comprobar conexión" }));
}

it("disables the server-rendered read control until hydration installs its event handlers", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json({ enabled: false, accounts: [] }));
  vi.stubGlobal("fetch", fetch);
  const container = document.createElement("div");
  document.body.append(container);
  container.innerHTML = renderToString(<PositionsWorkspace />);
  expect(
    within(container).getByRole("button", { name: "Comprobar conexión" }),
  ).toBeDisabled();
  expect(fetch).not.toHaveBeenCalled();
  render(<PositionsWorkspace />, { container, hydrate: true });
  expect(
    within(container).getByRole("button", { name: "Comprobar conexión" }),
  ).toBeEnabled();
  await list(userEvent.setup());
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("defaults to an empty read-only account view, never synthetic exposure or automatic requests", () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<PositionsWorkspace />);
  expect(
    screen.getByRole("button", { name: "Cuenta Binance · Solo lectura" }),
  ).toHaveAttribute("aria-pressed", "true");
  expect(
    screen.getByRole("region", { name: "Cuenta Binance de solo lectura" }),
  ).toBeVisible();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

it("recovers an explicitly historical account with exact signed decimal quantities", async () => {
  const data = syntheticAccountSnapshot();
  data.startedAt = "2020-01-01T00:00:00Z";
  data.capturedAt = "2020-01-01T00:00:01Z";
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: false, accounts }))
    .mockResolvedValueOnce(
      Response.json({ snapshot: data, storedAt: "2020-01-01T00:00:02Z" }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<PositionsWorkspace />);
  await list(user);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(
    screen.getByRole("button", { name: "Consultar cuenta y órdenes" }),
  ).toBeDisabled();
  await user.click(
    screen.getByRole("button", { name: "Mostrar última lectura guardada" }),
  );
  expect(await screen.findByText(/LECTURA GUARDADA · NO ACTUAL/)).toBeVisible();
  expect(screen.queryByText(/LECTURA PUNTUAL/)).not.toBeInTheDocument();
  const positions = () =>
    screen.getByRole("table", { name: /Posiciones USDT/ });
  expect(
    within(positions()).getByRole("cell", { name: "1.000000000000000001" }),
  ).toBeVisible();
  expect(
    within(positions()).getByRole("cell", { name: "-2.000000000000000001" }),
  ).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch.mock.calls[1][1].body).toBe(
    JSON.stringify({ accountId: accounts[0].id }),
  );
  expect(
    screen.queryByRole("button", {
      name: /calcular ejemplo|cancelar|ejecutar|enviar orden/i,
    }),
  ).not.toBeInTheDocument();
});

it("filters by position side, never BUY/SELL, without refetching or changing balances", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceAccountData snapshot={syntheticAccountSnapshot()} />);
  const positions = () =>
    screen.getByRole("table", { name: /Posiciones USDT/ });
  await user.selectOptions(screen.getByLabelText("Lado de posición"), "SHORT");
  expect(
    within(positions()).queryByRole("cell", { name: "1.000000000000000001" }),
  ).not.toBeInTheDocument();
  expect(screen.getAllByRole("cell", { name: "BUY / SHORT" })).toHaveLength(2);
  expect(screen.getByRole("cell", { name: /9007199254740993/ })).toBeVisible();
  expect(screen.getByRole("cell", { name: /9007199254740995/ })).toBeVisible();
  expect(
    screen.queryByRole("cell", { name: /9007199254740997/ }),
  ).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText("Lado de posición"), "BOTH");
  expect(
    within(positions()).getByRole("cell", { name: "ETHUSDT · BOTH" }),
  ).toBeVisible();
  expect(within(positions()).getAllByRole("row")).toHaveLength(2);
  expect(screen.getByRole("cell", { name: /9007199254740997/ })).toBeVisible();
  expect(
    screen.getByText(/No hay órdenes condicionales que coincidan/),
  ).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Quitar filtros" }));
  fireEvent.change(screen.getByRole("searchbox", { name: "Buscar símbolo" }), {
    target: { value: "missing" },
  });
  expect(screen.getByText(/No hay posiciones que coincidan/)).toBeVisible();
  expect(screen.getByRole("cell", { name: "100.00000001" })).toBeVisible(); // balances unaffected
  expect(fetch).not.toHaveBeenCalled();
});

it("keeps simulation opt-in and erases account data when changing source", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: true, accounts }))
    .mockResolvedValueOnce(Response.json(syntheticAccountSnapshot()));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<PositionsWorkspace />);
  await list(user);
  await user.click(
    screen.getByRole("button", { name: "Consultar cuenta y órdenes" }),
  );
  expect(
    await screen.findByRole("table", { name: /Posiciones USDT/ }),
  ).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: "Ejemplos de simulación" }),
  );
  expect(
    screen.queryByRole("region", { name: "Lectura de Binance" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("table", { name: /Posiciones sintéticas/ }),
  ).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: "Cuenta Binance · Solo lectura" }),
  );
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Cuenta de lectura")).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(2);
});

it("clears all financial data on account change, failed refresh or an invalid complete response, without examples fallback", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: true, accounts }))
    .mockResolvedValueOnce(Response.json(syntheticAccountSnapshot()))
    .mockResolvedValueOnce(
      Response.json({ error: "Cuenta no disponible." }, { status: 404 }),
    )
    .mockResolvedValueOnce(
      Response.json({ ...syntheticAccountSnapshot(), executionReady: true }),
    )
    .mockResolvedValueOnce(
      Response.json({ error: "Sesión no válida." }, { status: 401 }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<PositionsWorkspace />);
  await list(user);
  const update = screen.getByRole("button", {
    name: "Consultar cuenta y órdenes",
  });
  await user.click(update);
  expect(
    await screen.findByRole("table", { name: /Posiciones USDT/ }),
  ).toBeVisible();
  await user.selectOptions(
    screen.getByLabelText("Cuenta de lectura"),
    accounts[1].id,
  );
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  await user.click(update);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Cuenta no disponible.",
  );
  expect(fetch.mock.calls[2][1].body).toBe(
    JSON.stringify({ accountId: accounts[1].id }),
  );
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  await user.click(update);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    /No se pudo leer la cuenta/,
  );
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  await user.click(update);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Sesión no válida.",
  );
  expect(screen.queryByLabelText("Cuenta de lectura")).not.toBeInTheDocument();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});

it("aborts an in-flight request when leaving the account view and ignores late responses", async () => {
  let finish!: (response: Response) => void;
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ enabled: true, accounts }))
    .mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          finish = resolve;
        }),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<PositionsWorkspace />);
  await list(user);
  const button = screen.getByRole("button", {
    name: "Consultar cuenta y órdenes",
  });
  fireEvent.click(button);
  fireEvent.click(button);
  expect(fetch).toHaveBeenCalledTimes(2);
  const signal = fetch.mock.calls[1][1].signal as AbortSignal;
  expect(signal.aborted).toBe(false);
  await user.click(
    screen.getByRole("button", { name: "Ejemplos de simulación" }),
  );
  expect(signal.aborted).toBe(true);
  await user.click(
    screen.getByRole("button", { name: "Cuenta Binance · Solo lectura" }),
  );
  await act(async () => finish(Response.json(syntheticAccountSnapshot())));
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});

it("bounds rendered positions/orders to pages of 50 and resets pages when filters change", async () => {
  const snapshot = syntheticAccountSnapshot();
  snapshot.positions = Array.from({ length: 51 }, (_, i) => ({
    ...snapshot.positions[0],
    symbol: `A${String(i).padStart(3, "0")}USDT`,
  }));
  snapshot.openOrders = Array.from({ length: 51 }, (_, i) => ({
    ...snapshot.openOrders[0],
    orderId: String(1000 + i),
  }));
  snapshot.algoOrders = Array.from({ length: 51 }, (_, i) => ({
    ...snapshot.algoOrders[0],
    algoId: String(2000 + i),
  }));
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceAccountData snapshot={snapshot} />);
  const positions = screen.getByRole("table", { name: /Posiciones USDT/ });
  const orders = screen.getByRole("table", {
    name: /Órdenes abiertas normales/,
  });
  const algos = screen.getByRole("table", { name: /Órdenes condicionales/ });
  for (const table of [positions, orders, algos])
    expect(within(table).getAllByRole("row")).toHaveLength(51);
  for (const label of [
    "posiciones",
    "órdenes normales",
    "órdenes condicionales",
  ])
    await user.click(
      screen.getByRole("button", { name: `Siguientes ${label}` }),
    );
  for (const table of [positions, orders, algos])
    expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(within(positions).getByText("A050USDT · LONG")).toBeVisible();
  await user.type(
    screen.getByRole("searchbox", { name: "Buscar símbolo" }),
    "a000",
  );
  expect(screen.getByRole("cell", { name: "A000USDT · LONG" })).toBeVisible();
  expect(
    screen.getByRole("button", { name: "Anteriores posiciones" }),
  ).toBeDisabled();
  expect(snapshot.positions).toHaveLength(51);
  expect(snapshot.openOrders).toHaveLength(51);
  expect(fetch).not.toHaveBeenCalled();
});
