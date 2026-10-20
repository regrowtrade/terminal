"use client";

import { useEffect, useRef, useState } from "react";
import { connectionStateSchema } from "@/lib/connections-schema";
import {
  storedPlanSchema,
  storedIntentSchema,
  type StoredIntent,
  type StoredPlan,
} from "@/lib/saved-plans-schema";
import { readError } from "./plan-save";

type Account = { id: string; name: string };

// Read-only view of persisted decisions and their derived (never sent) intents.
export function SavedPlans() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [accountId, setAccountId] = useState("");
  const [plans, setPlans] = useState<StoredPlan[]>([]);
  const [intents, setIntents] = useState<Record<string, StoredIntent[]>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    active.current = controller;
    fetch("/api/connections/binance/list", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
      signal: controller.signal,
    })
      .then((r) => r.json())
      .then((body: unknown) => {
        const state = connectionStateSchema.parse(body);
        setAccounts(state.accounts);
        const first = state.accounts[0]?.id ?? "";
        setAccountId(first);
        if (first) void load(first);
      })
      .catch(() => {});
    return () => controller.abort();
  }, []);
  async function load(selected: string) {
    if (!selected) return;
    setBusy(true);
    setError("");
    setPlans([]);
    setIntents({});
    try {
      const response = await fetch("/api/plans/list", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId: selected }),
      });
      const body: unknown = await response.json();
      if (!response.ok)
        throw new Error(readError(body, "No se pudo listar los planes."));
      const listed = storedPlanSchema.array().parse(body);
      setPlans(listed);
      const byPlan: Record<string, StoredIntent[]> = {};
      for (const plan of listed) {
        try {
          const intentResponse = await fetch("/api/plans/intents/list", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ accountId: selected, planId: plan.planId }),
          });
          const intentBody: unknown = await intentResponse.json();
          if (intentResponse.ok) {
            byPlan[plan.planId] = storedIntentSchema.array().parse(intentBody);
          }
        } catch {
          byPlan[plan.planId] = [];
        }
      }
      setIntents(byPlan);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "No se pudo listar los planes.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div aria-label="Planes guardados" aria-busy={busy}>
      {accounts.length === 0 ? (
        <p className="muted-copy">
          No hay cuentas provisionadas. Los planes se guardan ligados a una
          cuenta.
        </p>
      ) : (
        <>
          <label htmlFor="saved-plans-account">Cuenta</label>
          <select
            id="saved-plans-account"
            value={accountId}
            onChange={(e) => {
              setAccountId(e.target.value);
              void load(e.target.value);
            }}
            disabled={busy}
          >
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          {error && (
            <p className="notice danger" role="alert">
              {error}
            </p>
          )}
          {plans.length === 0 && !busy && (
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
              {(intents[plan.planId] ?? []).length > 0 && (
                <ul className="intent-list">
                  {intents[plan.planId].map((intent) => (
                    <li key={intent.id}>
                      <strong>{intent.purpose}</strong> {intent.positionSide} ·{" "}
                      {intent.family} · qty {intent.quantity}
                      {intent.price ? ` · precio ${intent.price}` : ""} ·{" "}
                      <code>{intent.clientOrderId}</code> · {intent.status}
                    </li>
                  ))}
                </ul>
              )}
            </article>
          ))}
        </>
      )}
    </div>
  );
}
