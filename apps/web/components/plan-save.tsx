"use client";

import { useEffect, useRef, useState } from "react";
import { connectionStateSchema } from "@/lib/connections-schema";
import type { PlanRequest } from "@/lib/plans";
import { savedPlanSchema } from "@/lib/saved-plans-schema";

type Account = { id: string; name: string };

// Saves the current computed plan (and its deterministic intents for clean/dca)
// to a provisioned account. It never sends orders; hedge plans persist without
// intents because their decomposition is not implemented yet.
export function PlanSave({ request }: { request: PlanRequest }) {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [accountId, setAccountId] = useState("");
  const [saving, setSaving] = useState(false);
  const [savedId, setSavedId] = useState("");
  const [intents, setIntents] = useState(0);
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
        setAccountId(state.accounts[0]?.id ?? "");
      })
      .catch(() => {});
    return () => controller.abort();
  }, []);
  async function save() {
    if (!accountId || saving) return;
    setSaving(true);
    setError("");
    setSavedId("");
    setIntents(0);
    try {
      const response = await fetch("/api/plans/save", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId, plan: request }),
      });
      const body: unknown = await response.json();
      if (!response.ok)
        throw new Error(readError(body, "No se pudo guardar el plan."));
      const saved = savedPlanSchema.parse(body);
      setSavedId(saved.planId);
      if (request.mode !== "hedge") {
        const intentResponse = await fetch("/api/plans/intents/save", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ accountId, planId: saved.planId }),
        });
        const intentBody: unknown = await intentResponse.json();
        if (
          intentResponse.ok &&
          intentBody &&
          typeof intentBody === "object" &&
          "count" in intentBody &&
          typeof intentBody.count === "number"
        ) {
          setIntents(intentBody.count);
        }
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "No se pudo guardar el plan.");
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="plan-save" aria-label="Guardar plan">
      {accounts.length === 0 ? (
        <p className="muted-copy">
          No hay cuentas provisionadas. Provisiona una cuenta Binance para
          guardar planes.
        </p>
      ) : (
        <>
          <label htmlFor="plan-save-account">Guardar en la cuenta</label>
          <select
            id="plan-save-account"
            value={accountId}
            onChange={(e) => {
              setAccountId(e.target.value);
              setSavedId("");
              setError("");
            }}
            disabled={saving}
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
            disabled={saving || !accountId}
            onClick={() => void save()}
          >
            {saving ? "Guardando…" : "Guardar plan (simulación)"}
          </button>
        </>
      )}
      {savedId && (
        <p role="status">
          Plan guardado · {savedId}
          {request.mode !== "hedge"
            ? ` · ${intents} intenciones derivadas`
            : " · intenciones de cobertura pendientes"}
        </p>
      )}
      {error && (
        <p className="notice danger" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

export function readError(body: unknown, fallback: string): string {
  return typeof body === "object" &&
    body !== null &&
    "error" in body &&
    typeof body.error === "string"
    ? body.error
    : fallback;
}
