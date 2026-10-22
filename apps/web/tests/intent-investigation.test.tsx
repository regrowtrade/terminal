import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { IntentInvestigation } from "@/components/intent-investigation";
import { SavedPlans } from "@/components/saved-plans";
import {
  intentObservationSchema,
  intentObservationTarget,
  matchesIntentObservationTarget,
} from "@/lib/intent-observation-schema";
import {
  syntheticIntentAccount as account,
  syntheticIntentId,
  syntheticPlanId,
  syntheticSavedPlan,
  syntheticSavedIntent,
  syntheticIntentObservation,
} from "./fixtures/intent-investigation";

afterEach(() => vi.unstubAllGlobals());
const target = intentObservationTarget(
  syntheticSavedPlan(),
  syntheticSavedIntent(),
)!;
const observeButton = () =>
  screen.getByRole("button", { name: "Consultar y guardar observación" });
const historyButton = () =>
  screen.getByRole("button", { name: "Ver últimas 20 observaciones" });

it("opens without requests and sends only owned-context IDs on explicit investigation", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(Response.json(syntheticIntentObservation()));
  vi.stubGlobal("fetch", fetch);
  render(
    <IntentInvestigation accountId={account.id} target={target} enabled />,
  );
  expect(fetch).not.toHaveBeenCalled();
  const user = userEvent.setup();
  await user.click(observeButton());
  expect(await screen.findByText(/OBSERVACIÓN PUNTUAL GUARDADA/)).toBeVisible();
  expect(screen.getByText(/ATRIBUTOS COINCIDENTES/)).toHaveTextContent(
    "PROPIEDAD NO CONFIRMADA",
  );
  expect(
    screen.getByText("9007199254740993 / " + target.clientOrderId),
  ).toBeVisible();
  expect(screen.getByText("2.000 / 0.500000000000000001")).toBeVisible();
  expect(screen.getByText("100.000 / 99.123456789012345678")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe("/api/plans/intents/observe");
  expect(fetch.mock.calls[0][1]).toMatchObject({
    method: "POST",
    cache: "no-store",
    redirect: "error",
  });
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    accountId: account.id,
    intentId: syntheticIntentId,
  });
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /ejecutar|reenviar|enviar orden/i }),
  ).not.toBeInTheDocument();
});

it("explains decimal mismatches and unresolved absence without associating or resending", async () => {
  const mismatched = syntheticIntentObservation();
  mismatched.report.assessment = "mismatch";
  mismatched.report.mismatches = ["price"];
  mismatched.report.observation.order!.price = "100.000000000000000001";
  const unresolved = syntheticIntentObservation();
  unresolved.report.assessment = "unresolved";
  unresolved.report.observation.outcome = "unresolved";
  unresolved.report.observation.reason = "exchange_not_found";
  unresolved.report.observation.order = null;
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(Response.json(mismatched))
    .mockResolvedValueOnce(Response.json(unresolved));
  vi.stubGlobal("fetch", fetch);
  render(
    <IntentInvestigation accountId={account.id} target={target} enabled />,
  );
  const user = userEvent.setup();
  await user.click(observeButton());
  expect(await screen.findByText(/DISCREPANCIAS/)).toHaveTextContent(
    "No se asocia",
  );
  expect(screen.getByRole("listitem")).toHaveTextContent("Precio");
  expect(screen.getByText(/100.000000000000000001/)).toBeVisible();
  await user.click(observeButton());
  expect(await screen.findByText(/NO RESUELTA/)).toHaveTextContent(
    "No reenvíes",
  );
  expect(screen.queryByText(/100.000000000000000001/)).not.toBeInTheDocument();
});

it("loads at most 20 historical observations explicitly with the external reader disabled", async () => {
  const observations = Array.from({ length: 20 }, (_, i) => ({
    ...syntheticIntentObservation(),
    id: `00000000-0000-4000-8000-${String(i + 100).padStart(12, "0")}`,
  }));
  const fetch = vi.fn().mockResolvedValue(Response.json(observations));
  vi.stubGlobal("fetch", fetch);
  render(
    <IntentInvestigation
      accountId={account.id}
      target={target}
      enabled={false}
    />,
  );
  expect(observeButton()).toBeDisabled();
  expect(fetch).not.toHaveBeenCalled();
  await userEvent.setup().click(historyButton());
  expect(
    await screen.findByText(/HISTORIAL GUARDADO · NO ACTUAL/),
  ).toHaveTextContent("no es la historia completa");
  expect(
    screen.getAllByRole("article", { name: "Evidencia guardada de intención" }),
  ).toHaveLength(20);
  expect(fetch.mock.calls[0][0]).toBe("/api/plans/intents/observations");
});

it("distinguishes empty saved history from an unresolved exchange response", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json([])));
  render(
    <IntentInvestigation
      accountId={account.id}
      target={target}
      enabled={false}
    />,
  );
  await userEvent.setup().click(historyButton());
  expect(
    await screen.findByText(/No hay observaciones guardadas/),
  ).toHaveTextContent("No demuestra ausencia");
  expect(screen.queryByText(/NO RESUELTA/)).not.toBeInTheDocument();
});

it.each([401, 404, 409, 422, 429, 503])(
  "retires earlier evidence on an observation error (%s), without fallback or private details",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(Response.json([syntheticIntentObservation()]))
      .mockResolvedValueOnce(
        Response.json({ error: "private-upstream" }, { status }),
      );
    vi.stubGlobal("fetch", fetch);
    render(
      <IntentInvestigation accountId={account.id} target={target} enabled />,
    );
    const user = userEvent.setup();
    await user.click(historyButton());
    expect(await screen.findByText(/HISTORIAL GUARDADO/)).toBeVisible();
    await user.click(observeButton());
    expect(await screen.findByRole("alert")).toBeVisible();
    expect(
      screen.queryByText(
        /HISTORIAL GUARDADO|ATRIBUTOS COINCIDENTES|private-upstream/,
      ),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
  },
);

it.each([
  "intent",
  "plan",
  "quantity",
  "price",
  "client",
  "purpose",
  "ownership",
  "assessment",
])(
  "rejects evidence not bound to the selected stored target (%s)",
  async (field) => {
    const body = syntheticIntentObservation();
    const expected = body.report.expected;
    const order = body.report.observation.order!;
    if (field === "intent") expected.intentId = syntheticPlanId;
    if (field === "plan") expected.planId = syntheticIntentId;
    if (field === "quantity") {
      expected.quantity = "2.000000000000000001";
      order.origQty = expected.quantity;
    }
    if (field === "price") {
      expected.price = "100.000000000000000001";
      order.price = expected.price;
    }
    if (field === "client") {
      expected.clientOrderId = `rt${"c".repeat(30)}`;
      body.report.observation.query.clientOrderId = expected.clientOrderId;
      order.clientOrderId = expected.clientOrderId;
    }
    if (field === "purpose") expected.purpose = "REBUY";
    if (field === "ownership")
      Object.assign(body.report, { ownershipConfirmed: true });
    if (field === "assessment") body.report.assessment = "unresolved";
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
    render(
      <IntentInvestigation accountId={account.id} target={target} enabled />,
    );
    await userEvent.setup().click(observeButton());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Puede haberse guardado evidencia",
    );
    expect(
      screen.queryByText(/OBSERVACIÓN PUNTUAL GUARDADA/),
    ).not.toBeInTheDocument();
  },
);

it("accepts decimal-scale equivalents while rejecting sub-float changes in the stored criteria", () => {
  const body = syntheticIntentObservation();
  body.report.expected.quantity = "2.0000";
  expect(intentObservationSchema.safeParse(body).success).toBe(true);
  expect(matchesIntentObservationTarget(body, target)).toBe(true);
  body.report.expected.quantity = "2.000000000000000001";
  body.report.observation.order!.origQty = body.report.expected.quantity;
  expect(intentObservationSchema.safeParse(body).success).toBe(true);
  expect(matchesIntentObservationTarget(body, target)).toBe(false);
});

it.each(["oversized", "duplicate", "foreign"])(
  "rejects invalid history as a whole (%s)",
  async (kind) => {
    const first = syntheticIntentObservation();
    const second = syntheticIntentObservation();
    second.id = "00000000-0000-4000-8000-000000000099";
    if (kind === "foreign") second.report.expected.planId = syntheticIntentId;
    const body =
      kind === "oversized"
        ? Array.from({ length: 21 }, () => first)
        : kind === "duplicate"
          ? [first, first]
          : [first, second];
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json(body)));
    render(
      <IntentInvestigation
        accountId={account.id}
        target={target}
        enabled={false}
      />,
    );
    await userEvent.setup().click(historyButton());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No se pudo recuperar el historial",
    );
    expect(screen.queryByRole("article")).not.toBeInTheDocument();
  },
);

it("aborts requests on unmount and never starts simultaneous history/observation requests", async () => {
  let resolve!: (r: Response) => void;
  let signal!: AbortSignal;
  const fetch = vi.fn().mockImplementation((_url, options) => {
    signal = options.signal;
    return new Promise<Response>((r) => {
      resolve = r;
    });
  });
  vi.stubGlobal("fetch", fetch);
  const { unmount } = render(
    <IntentInvestigation accountId={account.id} target={target} enabled />,
  );
  const user = userEvent.setup();
  await user.click(observeButton());
  expect(historyButton()).toBeDisabled();
  await user.click(historyButton());
  expect(fetch).toHaveBeenCalledTimes(1);
  unmount();
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(Response.json(syntheticIntentObservation()));
  });
  expect(screen.queryByRole("article")).not.toBeInTheDocument();
});

async function openInvestigation(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: /Ver intenciones del plan/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: /Investigar ENTRY/ }),
  );
}
function savedFetch() {
  return vi.fn(async (url: string) => {
    if (url === "/api/connections/binance/list")
      return Response.json({
        enabled: true,
        accounts: [
          account,
          {
            ...account,
            id: "00000000-0000-4000-8000-000000000099",
            name: "Other synthetic",
          },
        ],
      });
    if (url === "/api/plans/list") return Response.json([syntheticSavedPlan()]);
    if (url === "/api/plans/intents/list")
      return Response.json([
        syntheticSavedIntent(),
        {
          ...syntheticSavedIntent(),
          id: "00000000-0000-4000-8000-000000000098",
          family: "conditional",
          purpose: "TP",
        },
      ]);
    throw new Error("Unexpected automatic evidence request");
  });
}

it("links only supported stored intents and makes evidence opt-in instead of automatically querying Binance", async () => {
  const fetch = savedFetch();
  vi.stubGlobal("fetch", fetch);
  render(<SavedPlans />);
  const user = userEvent.setup();
  expect(await screen.findByText(/BTCUSDT · CLEAN/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  await openInvestigation(user);
  expect(
    screen.getByRole("region", { name: "Investigación de intención" }),
  ).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(3);
  expect(screen.getByText(/Investigación no disponible/)).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /Investigar TP/ }),
  ).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "Cerrar investigación" }),
  );
  expect(
    screen.queryByRole("region", { name: "Investigación de intención" }),
  ).not.toBeInTheDocument();
});

it.each(["account", "reload", "close"])(
  "aborts investigation on parent context changes (%s) and ignores a late result",
  async (change) => {
    const base = savedFetch();
    let resolve!: (r: Response) => void;
    let signal!: AbortSignal;
    const fetch = vi.fn((url, options) =>
      url === "/api/plans/intents/observe"
        ? new Promise<Response>((r) => {
            signal = options.signal;
            resolve = r;
          })
        : base(url),
    );
    vi.stubGlobal("fetch", fetch);
    render(<SavedPlans />);
    const user = userEvent.setup();
    await openInvestigation(user);
    await user.click(observeButton());
    if (change === "account")
      await user.selectOptions(
        screen.getByLabelText("Cuenta"),
        "00000000-0000-4000-8000-000000000099",
      );
    if (change === "reload")
      await user.click(
        screen.getByRole("button", { name: "Actualizar planes guardados" }),
      );
    if (change === "close")
      await user.click(
        screen.getByRole("button", { name: "Cerrar investigación" }),
      );
    expect(signal.aborted).toBe(true);
    await act(async () => {
      resolve(Response.json(syntheticIntentObservation()));
    });
    expect(
      screen.queryByText(/ATRIBUTOS COINCIDENTES/),
    ).not.toBeInTheDocument();
  },
);

it("distinguishes intent-list failures from empty results and rejects another plan's intents", async () => {
  const base = savedFetch();
  let count = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((url) => {
      if (url === "/api/plans/intents/list") {
        count++;
        return Promise.resolve(
          count === 1
            ? Response.json({ error: "private-error" }, { status: 503 })
            : Response.json([
                { ...syntheticSavedIntent(), planId: syntheticIntentId },
              ]),
        );
      }
      return base(url);
    }),
  );
  render(<SavedPlans />);
  const user = userEvent.setup();
  for (let i = 0; i < 2; i++) {
    await user.click(
      await screen.findByRole("button", { name: /Ver intenciones del plan/ }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No equivale a una lista vacía",
    );
    expect(
      screen.queryByText(/Sin intenciones guardadas|private-error/),
    ).not.toBeInTheDocument();
  }
});

it("aborts a pending intent list when changing account and never renders its late rows", async () => {
  const base = savedFetch();
  let resolve!: (r: Response) => void;
  let signal!: AbortSignal;
  vi.stubGlobal(
    "fetch",
    vi.fn((url, options) =>
      url === "/api/plans/intents/list"
        ? new Promise<Response>((r) => {
            signal = options.signal;
            resolve = r;
          })
        : base(url),
    ),
  );
  render(<SavedPlans />);
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: /Ver intenciones del plan/ }),
  );
  await user.selectOptions(
    screen.getByLabelText("Cuenta"),
    "00000000-0000-4000-8000-000000000099",
  );
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(Response.json([syntheticSavedIntent()]));
  });
  expect(
    screen.queryByRole("button", { name: /Investigar ENTRY/ }),
  ).not.toBeInTheDocument();
});

it.each(["conditional", "hedge", "missing price", "non-deterministic client"])(
  "does not offer unsupported investigation (%s)",
  (kind) => {
    const plan = syntheticSavedPlan(),
      intent = syntheticSavedIntent();
    if (kind === "conditional") {
      intent.family = "conditional";
      intent.purpose = "SL";
    }
    if (kind === "hedge") plan.mode = "hedge";
    if (kind === "missing price") intent.price = null;
    if (kind === "non-deterministic client")
      intent.clientOrderId = "not-an-intent-id";
    expect(intentObservationTarget(plan, intent)).toBeNull();
  },
);
