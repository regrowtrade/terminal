"use client";

import { useEffect, useRef, useState } from "react";
import { connectionStateSchema } from "@/lib/connections-schema";
import {
  storedPlanSchema,
  storedIntentSchema,
  type StoredIntent,
  type StoredPlan,
} from "@/lib/saved-plans-schema";
import {
  intentObservationTarget,
  type IntentObservationTarget,
} from "@/lib/intent-observation-schema";
import { IntentInvestigation } from "./intent-investigation";

async function read(path: string, input: object, controller: AbortController) {
  const signal = AbortSignal.any([
    controller.signal,
    AbortSignal.timeout(22_000),
  ]);
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    cache: "no-store",
    redirect: "error",
    signal,
  });
  if (!response.ok) throw new Error("Saved decisions unavailable");
  const body: unknown = await response.json();
  signal.throwIfAborted();
  return body;
}
function parsePlans(body: unknown) {
  const listed = storedPlanSchema.array().max(20).parse(body);
  if (new Set(listed.map((p) => p.planId)).size !== listed.length)
    throw new Error("Duplicate plan");
  return listed;
}

export function SavedPlans() {
  const [accounts, setAccounts] = useState<{ id: string; name: string }[]>([]);
  const [accountId, setAccountId] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [plans, setPlans] = useState<StoredPlan[]>([]);
  const [intents, setIntents] = useState<Record<string, StoredIntent[]>>({});
  const [intentErrors, setIntentErrors] = useState<Record<string, string>>({});
  const [target, setTarget] = useState<IntentObservationTarget | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const active = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    active.current = controller;
    void (async () => {
      try {
        const state = connectionStateSchema.parse(
          await read("/api/connections/binance/list", {}, controller),
        );
        if (controller.signal.aborted) return;
        setAccounts(state.accounts);
        setEnabled(state.enabled);
        const first = state.accounts[0]?.id ?? "";
        setAccountId(first);
        if (first) {
          const listed = parsePlans(
            await read("/api/plans/list", { accountId: first }, controller),
          );
          if (!controller.signal.aborted) setPlans(listed);
        }
      } catch {
        if (!controller.signal.aborted)
          setError(
            "No se pudieron cargar las cuentas o los planes guardados. No se muestran datos de otra cuenta.",
          );
      } finally {
        if (active.current === controller) {
          active.current = null;
          setBusy(false);
        }
      }
    })();
    return () => {
      controller.abort();
      active.current?.abort();
    };
  }, []);

  async function load(selected: string, planId?: string) {
    if (!selected || active.current) return;
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    setError("");
    setTarget(null);
    if (planId) {
      setIntents((previous) => {
        const next = { ...previous };
        delete next[planId];
        return next;
      });
      setIntentErrors((previous) => ({ ...previous, [planId]: "" }));
    } else {
      setPlans([]);
      setIntents({});
      setIntentErrors({});
    }
    try {
      if (planId) {
        const listed = storedIntentSchema
          .array()
          .parse(
            await read(
              "/api/plans/intents/list",
              { accountId: selected, planId },
              controller,
            ),
          );
        if (controller.signal.aborted) return;
        if (
          listed.some((intent) => intent.planId !== planId) ||
          new Set(listed.map((i) => i.id)).size !== listed.length
        )
          throw new Error("Unexpected intents");
        setIntents((previous) => ({ ...previous, [planId]: listed }));
      } else {
        const listed = parsePlans(
          await read("/api/plans/list", { accountId: selected }, controller),
        );
        if (!controller.signal.aborted) setPlans(listed);
      }
    } catch {
      if (!controller.signal.aborted) {
        if (planId)
          setIntentErrors((previous) => ({
            ...previous,
            [planId]:
              "No se pudieron leer las intenciones de este plan. No equivale a una lista vacía.",
          }));
        else
          setError(
            "No se pudieron leer los planes de esta cuenta. No se muestran resultados anteriores.",
          );
      }
    } finally {
      if (active.current === controller) {
        active.current = null;
        setBusy(false);
      }
    }
  }

  return (
    <div aria-label="Planes guardados" aria-busy={busy}>
      {error && (
        <p className="notice danger" role="alert">
          {error}
        </p>
      )}
      {accounts.length === 0 ? (
        <p className="muted-copy">
          {busy
            ? "Consultando cuentas y planes guardados…"
            : error
              ? "Lectura de planes no disponible."
              : "No hay cuentas provisionadas. Los planes se guardan ligados a una cuenta."}
        </p>
      ) : (
        <>
          <label htmlFor="saved-plans-account">Cuenta</label>
          <select
            id="saved-plans-account"
            value={accountId}
            onChange={(e) => {
              active.current?.abort();
              active.current = null;
              setAccountId(e.target.value);
              void load(e.target.value);
            }}
          >
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="button secondary"
            disabled={busy}
            onClick={() => void load(accountId)}
          >
            Actualizar planes guardados
          </button>
          <p className="muted-copy">
            Planes de simulación, nunca enviados. Las intenciones se cargan por
            plan; investigar e historial son acciones separadas, sin consultas
            automáticas a Binance.
          </p>
          {plans.length === 0 && !busy && !error && (
            <p className="muted-copy">Sin planes guardados para esta cuenta.</p>
          )}
          {plans.map((plan) => (
            <article key={plan.planId} className="saved-plan">
              <div className="exchange-row">
                <div>
                  <strong>
                    {plan.symbol} · {plan.mode.toUpperCase()}
                  </strong>
                  <small>
                    Riesgo {plan.riskAmount} USDT ·{" "}
                    {new Date(plan.createdAt).toLocaleString("es")}
                  </small>
                </div>
                <span className="pill muted">NO EJECUTADO</span>
              </div>
              <button
                type="button"
                className="button secondary"
                aria-label={`Ver intenciones del plan ${plan.planId}`}
                disabled={busy}
                onClick={() => void load(accountId, plan.planId)}
              >
                Ver intenciones
              </button>
              {intentErrors[plan.planId] && (
                <p role="alert" className="notice danger">
                  {intentErrors[plan.planId]}
                </p>
              )}
              {intents[plan.planId]?.length === 0 && (
                <p>
                  Sin intenciones guardadas para este plan; no describe las
                  órdenes de la cuenta.
                </p>
              )}
              {(intents[plan.planId] ?? []).length > 0 && (
                <ul className="intent-list">
                  {intents[plan.planId].map((intent) => {
                    const candidate = intentObservationTarget(plan, intent);
                    return (
                      <li key={intent.id}>
                        <strong>{intent.purpose}</strong> {intent.positionSide}{" "}
                        · {intent.family} · qty {intent.quantity}
                        {intent.price ? ` · precio ${intent.price}` : ""} ·{" "}
                        <code>{intent.clientOrderId}</code> · {intent.status}
                        {candidate ? (
                          <button
                            type="button"
                            className="button secondary compact"
                            disabled={busy}
                            onClick={() => setTarget(candidate)}
                          >
                            Investigar {intent.purpose} · {intent.clientOrderId}
                          </button>
                        ) : (
                          <p className="muted-copy">
                            Investigación no disponible: solo ENTRY/REBUY
                            normales de planes clean/dca válidos; TP/SL y
                            cobertura siguen pendientes.
                          </p>
                        )}
                      </li>
                    );
                  })}
                </ul>
              )}
            </article>
          ))}
          {target && (
            <>
              <button
                type="button"
                className="button secondary"
                onClick={() => setTarget(null)}
              >
                Cerrar investigación
              </button>
              <IntentInvestigation
                key={`${accountId}:${target.planId}:${target.intentId}`}
                accountId={accountId}
                target={target}
                enabled={enabled}
              />
            </>
          )}
        </>
      )}
    </div>
  );
}
