import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { BinanceOrderLookup } from "@/components/binance-order-lookup";
import { syntheticOrderLookup } from "./fixtures/order-lookup";

afterEach(() => vi.unstubAllGlobals());
const accountId = "00000000-0000-4000-8000-000000000001";
it("makes no automatic request and keeps live reads disabled when unavailable", () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<BinanceOrderLookup accountId={accountId} disabled />);
  expect(fetch).not.toHaveBeenCalled();
  expect(screen.getByRole("button")).toBeDisabled();
});
it("shows exact observed evidence and clears it on edits and failed requests", async () => {
  const body = syntheticOrderLookup();
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(body))
    .mockResolvedValueOnce(Response.json({}, { status: 503 }));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceOrderLookup accountId={accountId} disabled={false} />);
  await user.type(screen.getByLabelText("Símbolo USDT"), "BTCUSDT");
  await user.type(
    screen.getByLabelText("Identificador de orden"),
    "9007199254740993",
  );
  await user.click(screen.getByRole("button"));
  expect(await screen.findByText(/OBSERVACIÓN PUNTUAL GUARDADA/)).toBeVisible();
  expect(screen.getByText("9007199254740993 / synthetic-limit")).toBeVisible();
  expect(screen.getByText("90.00000001 / 89.12345678")).toBeVisible();
  expect(fetch.mock.calls[0][1].body).toBe(
    JSON.stringify({ accountId, ...body.query }),
  );
  await user.click(screen.getByRole("button"));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    /No se pudo verificar y guardar/,
  );
  expect(
    screen.queryByText(/OBSERVACIÓN PUNTUAL GUARDADA/),
  ).not.toBeInTheDocument();
  await user.type(screen.getByLabelText("Símbolo USDT"), "X");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
it("does not turn a not-found response into cancellation, execution or a resend", async () => {
  const body = syntheticOrderLookup();
  body.outcome = "unresolved";
  body.reason = "exchange_not_found";
  body.order = null;
  body.query = { symbol: "BTCUSDT", clientOrderId: "synthetic-limit" };
  const fetch = vi.fn().mockResolvedValue(Response.json(body));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  render(<BinanceOrderLookup accountId={accountId} disabled={false} />);
  await user.type(screen.getByLabelText("Símbolo USDT"), "BTCUSDT");
  await user.selectOptions(
    screen.getByLabelText("Tipo de identificador"),
    "clientOrderId",
  );
  await user.type(
    screen.getByLabelText("Identificador de orden"),
    "synthetic-limit",
  );
  await user.click(screen.getByRole("button"));
  expect(await screen.findByText(/NO RESUELTA/)).toHaveTextContent(
    /No reenvíes/,
  );
  expect(
    screen.queryByRole("button", { name: /enviar|cancelar|ejecutar/i }),
  ).not.toBeInTheDocument();
  await user.clear(screen.getByLabelText("Identificador de orden"));
  expect(screen.queryByText(/NO RESUELTA/)).not.toBeInTheDocument();
});
it("aborts on unmount and never displays a response for another selector", async () => {
  let resolve!: (r: Response) => void;
  const fetch = vi.fn().mockImplementation(
    () =>
      new Promise<Response>((r) => {
        resolve = r;
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  const view = render(
    <BinanceOrderLookup accountId={accountId} disabled={false} />,
  );
  await user.type(screen.getByLabelText("Símbolo USDT"), "BTCUSDT");
  await user.type(screen.getByLabelText("Identificador de orden"), "123");
  await user.click(screen.getByRole("button"));
  resolve(Response.json(syntheticOrderLookup()));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(
    screen.queryByText(/OBSERVACIÓN PUNTUAL GUARDADA/),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button"));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  const signal = fetch.mock.calls[1][1].signal as AbortSignal;
  view.unmount();
  expect(signal.aborted).toBe(true);
  resolve(Response.json(syntheticOrderLookup()));
});
