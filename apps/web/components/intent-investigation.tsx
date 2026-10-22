"use client";

import { useEffect, useRef, useState } from "react";
import {
  intentObservationSchema,
  matchesIntentObservationTarget,
  type IntentObservation,
  type IntentObservationTarget,
} from "@/lib/intent-observation-schema";

// Keyed by account/plan/intent in SavedPlans. Closing, switching or reloading
// that context unmounts this panel and retires in-flight evidence.
export function IntentInvestigation({
  accountId,
  target,
  enabled,
}: {
  accountId: string;
  target: IntentObservationTarget;
  enabled: boolean;
}) {
  const [current, setCurrent] = useState<IntentObservation | null>(null);
  const [history, setHistory] = useState<IntentObservation[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);

  async function load(operation: "observe" | "observations") {
    if (active.current || (operation === "observe" && !enabled)) return;
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    setError("");
    setCurrent(null);
    setHistory(null);
    try {
      const signal = AbortSignal.any([
        controller.signal,
        AbortSignal.timeout(22_000),
      ]);
      const response = await fetch(`/api/plans/intents/${operation}`, {
        method: "POST",
        cache: "no-store",
        redirect: "error",
        signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId, intentId: target.intentId }),
      });
      if (!response.ok) {
        if (controller.signal.aborted) return;
        const messages: Record<number, string> = {
          401: "Sesión no válida. Vuelve a iniciar sesión.",
          404: "Cuenta, plan o intención no disponible.",
          409: "La intención o su plan cambió. Actualiza los planes antes de volver a consultar.",
          422: "Intención no compatible o permisos de lectura no válidos. No actives trading.",
          429: "Demasiadas consultas. Espera antes de reintentar.",
        };
        if (messages[response.status]) {
          setError(messages[response.status]);
          return;
        }
        throw new Error("Evidence unavailable");
      }
      const body: unknown = await response.json();
      if (controller.signal.aborted) return;
      signal.throwIfAborted();
      const observations =
        operation === "observe"
          ? [intentObservationSchema.parse(body)]
          : intentObservationSchema.array().max(20).parse(body);
      if (
        observations.some(
          (item) => !matchesIntentObservationTarget(item, target),
        ) ||
        new Set(observations.map((item) => item.id)).size !==
          observations.length
      )
        throw new Error("Unexpected intent evidence");
      if (operation === "observe") setCurrent(observations[0]);
      else setHistory(observations);
    } catch {
      if (!controller.signal.aborted)
        setError(
          operation === "observe"
            ? "No se pudo confirmar la observación. Puede haberse guardado evidencia; consulta su historial antes de reintentar. No se muestran resultados anteriores ni se enviaron órdenes."
            : "No se pudo recuperar el historial de esta intención. No se muestran resultados anteriores como actuales.",
        );
    } finally {
      if (active.current === controller) {
        active.current = null;
        setBusy(false);
      }
    }
  }

  return (
    <section
      className="intent-investigation"
      aria-label="Investigación de intención"
      aria-busy={busy}
    >
      <h3>Investigar intención guardada</h3>
      <p className="notice">
        INTENCIÓN DE SIMULACIÓN · NUNCA ENVIADA POR LA TERMINAL · EJECUCIÓN
        BLOQUEADA
      </p>
      <dl>
        <dt>Plan / intención</dt>
        <dd>
          {target.planId} / {target.intentId}
        </dd>
        <dt>Criterios guardados</dt>
        <dd>
          {target.symbol} · {target.purpose} · {target.positionSide}
        </dd>
        <dt>Cantidad / precio esperados</dt>
        <dd>
          {target.quantity} / {target.price}
        </dd>
        <dt>ID de cliente guardado</dt>
        <dd>
          <code>{target.clientOrderId}</code>
        </dd>
      </dl>
      <p className="muted-copy">
        El servidor relee el plan y consulta por su símbolo e ID de cliente, sin
        aceptar selectores editables. Consultar una coincidencia no demuestra
        envío ni propiedad: Binance permite reutilizar IDs de cliente.
      </p>
      <button
        type="button"
        className="button secondary"
        disabled={busy || !enabled}
        onClick={() => void load("observe")}
      >
        Consultar y guardar observación
      </button>
      <button
        type="button"
        className="button secondary"
        disabled={busy}
        onClick={() => void load("observations")}
      >
        Ver últimas 20 observaciones
      </button>
      <p className="muted-copy">
        La primera acción sí contacta Binance y requiere el lector habilitado;
        el historial solo consulta PostgreSQL, incluso con el lector
        desactivado. Sin consultas automáticas, reintentos ni cambios en la
        intención.
      </p>
      {!enabled && (
        <p>
          El lector externo está desactivado; solo puedes consultar evidencia
          guardada.
        </p>
      )}
      {error && (
        <p className="notice danger" role="alert">
          {error}
        </p>
      )}
      {current && (
        <>
          <p role="status">OBSERVACIÓN PUNTUAL GUARDADA · NO EN VIVO</p>
          <IntentEvidence observation={current} />
        </>
      )}
      {history !== null && (
        <>
          <p role="status">
            HISTORIAL GUARDADO · NO ACTUAL · Hasta 20 observaciones, más
            recientes primero; no es la historia completa.
          </p>
          {history.length === 0 ? (
            <p>
              No hay observaciones guardadas para esta intención. No demuestra
              ausencia de órdenes ni ejecuciones.
            </p>
          ) : (
            history.map((observation) => (
              <IntentEvidence key={observation.id} observation={observation} />
            ))
          )}
        </>
      )}
    </section>
  );
}

const mismatchLabels = {
  type: "Tipo",
  side: "Dirección BUY/SELL",
  positionSide: "Lado de posición",
  quantity: "Cantidad",
  price: "Precio",
  reduceOnly: "Solo reducción",
};

function IntentEvidence({
  observation: item,
}: {
  observation: IntentObservation;
}) {
  const { report } = item;
  const order = report.observation.order;
  return (
    <article
      className="saved-plan"
      aria-label="Evidencia guardada de intención"
    >
      <p>
        Consulta: {new Date(report.observation.checkedAt).toLocaleString("es")}{" "}
        · Guardada: {new Date(item.storedAt).toLocaleString("es")}
      </p>
      {report.assessment === "attributes_match" ? (
        <p className="notice">
          ATRIBUTOS COINCIDENTES · PROPIEDAD NO CONFIRMADA. No prueba que esta
          orden proceda de la intención.
        </p>
      ) : report.assessment === "unresolved" ? (
        <p className="notice danger">
          NO RESUELTA: Binance no devolvió la orden. No demuestra que nunca
          existió, que se canceló o que se ejecutó. No reenvíes una orden
          basándote en esta ausencia.
        </p>
      ) : (
        <>
          <p className="notice danger">
            DISCREPANCIAS · No se asocia la orden ni se modifica la intención.
          </p>
          <ul>
            {report.mismatches.map((field) => (
              <li key={field}>{mismatchLabels[field]}</li>
            ))}
          </ul>
        </>
      )}
      {order && (
        <dl>
          <dt>ID de orden observado / cliente</dt>
          <dd>
            {order.orderId} / {order.clientOrderId}
          </dd>
          <dt>Tipo / estado declarado</dt>
          <dd>
            {order.type} / {order.status}
          </dd>
          <dt>Símbolo / dirección / posición</dt>
          <dd>
            {order.symbol} / {order.side} / {order.positionSide}
          </dd>
          <dt>Cantidad original / ejecutada acumulada</dt>
          <dd>
            {order.origQty} / {order.executedQty}
          </dd>
          <dt>Precio / precio medio observado</dt>
          <dd>
            {order.price} / {order.avgPrice}
          </dd>
        </dl>
      )}
      <p className="muted-copy">
        Incluso FILLED describe la orden observada, no un cierre de posición,
        protección confirmada ni un ledger de fills. Sin PnL ni ejecución
        habilitada.
      </p>
    </article>
  );
}
