"use client";

import { useEffect, useId, useRef, useState } from "react";
import type { z } from "zod";
import {
  reconciliationReportSchema,
  type ReconciliationReport as Report,
} from "@/lib/reconciliation-schema";
import {
  fillRecoveryRequestSchema,
  fillRecoverySchema,
} from "@/lib/fill-recovery-schema";
import { ReconciliationReport } from "./reconciliation-report";

// The parent unmounts this panel on account/source changes and while loading
// another baseline. Late responses must never reappear under the new context.
export function BinanceReconciliation({
  accountId,
  enabled,
}: {
  accountId: string;
  enabled: boolean;
}) {
  const id = useId();
  const [symbol, setSymbol] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [report, setReport] = useState<Report | null>(null);
  const [recovery, setRecovery] = useState<z.infer<
    typeof fillRecoverySchema
  > | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);

  function clearRecovery() {
    setRecovery(null);
    setError("");
  }

  async function load(operation: "reconcile" | "fills/recover") {
    if (active.current || (operation === "fills/recover" && !enabled)) return;
    setError("");
    setRecovery(null);
    let input: { accountId: string } = { accountId };
    if (operation === "fills/recover") {
      // datetime-local is deliberately interpreted as UTC, not browser time.
      const parsed = fillRecoveryRequestSchema.safeParse({
        accountId,
        symbol,
        startTime: Date.parse(`${start}Z`),
        endTime: Date.parse(`${end}Z`),
      });
      if (!parsed.success) {
        setError(
          "Indica un símbolo USDT y una ventana UTC válida de hasta siete días, con fin posterior al inicio.",
        );
        return;
      }
      input = parsed.data;
    }
    // Recovery can commit even if its response is lost. Retire any old report
    // before I/O; a new report always requires a separate explicit action.
    setReport(null);
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      const signal = AbortSignal.any([
        controller.signal,
        AbortSignal.timeout(22_000),
      ]);
      const response = await fetch(`/api/connections/binance/${operation}`, {
        method: "POST",
        cache: "no-store",
        redirect: "error",
        signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!response.ok) {
        if (operation === "reconcile" && response.status === 404) {
          if (!controller.signal.aborted)
            setError(
              "Cuenta o lectura base no disponible. Solicita una lectura de cuenta antes de consultar el reporte.",
            );
          return;
        }
        throw new Error("Evidence unavailable");
      }
      const body: unknown = await response.json();
      if (controller.signal.aborted) return;
      signal.throwIfAborted();
      if (operation === "reconcile") {
        setReport(reconciliationReportSchema.parse(body));
      } else {
        const next = fillRecoverySchema.parse(body);
        const sent = fillRecoveryRequestSchema.parse(input);
        if (
          next.symbol !== sent.symbol ||
          next.startTime !== sent.startTime ||
          next.endTime !== sent.endTime
        )
          throw new Error("Recovery window mismatch");
        setRecovery(next);
      }
    } catch {
      if (!controller.signal.aborted)
        setError(
          operation === "reconcile"
            ? "No se pudo consultar el reporte. No se muestran resultados anteriores como actuales."
            : "No se pudo confirmar la recuperación. Puede haberse guardado evidencia; consulta el reporte antes de reintentar. No se enviaron órdenes.",
        );
    } finally {
      if (active.current === controller) {
        active.current = null;
        setBusy(false);
      }
    }
  }

  return (
    <section aria-label="Reconciliación de cuenta" aria-busy={busy}>
      <h3>Evidencia de reconciliación</h3>
      <p className="muted-copy">
        Consulta manual del reporte desde PostgreSQL y recuperación de fills.
        Sin monitor continuo, sin reconciliación completa y sin ejecución. No se
        actualiza automáticamente ni modifica planes o intenciones.
      </p>
      <button
        type="button"
        className="button secondary"
        disabled={busy}
        onClick={() => void load("reconcile")}
      >
        Consultar reporte de reconciliación
      </button>
      {report && <ReconciliationReport report={report} />}
      <fieldset disabled={busy || !enabled} className="account-filters">
        <legend>Recuperar fills por REST · Solo lectura</legend>
        <div className="account-filter-field">
          <label htmlFor={`${id}-symbol`}>Símbolo de recuperación</label>
          <input
            id={`${id}-symbol`}
            value={symbol}
            maxLength={28}
            placeholder="BTCUSDT"
            onChange={(e) => {
              setSymbol(e.target.value);
              clearRecovery();
            }}
          />
        </div>
        <div className="account-filter-field">
          <label htmlFor={`${id}-start`}>Inicio de ventana (UTC)</label>
          <input
            id={`${id}-start`}
            type="datetime-local"
            step="1"
            value={start}
            onChange={(e) => {
              setStart(e.target.value);
              clearRecovery();
            }}
          />
        </div>
        <div className="account-filter-field">
          <label htmlFor={`${id}-end`}>Fin de ventana (UTC)</label>
          <input
            id={`${id}-end`}
            type="datetime-local"
            step="1"
            value={end}
            onChange={(e) => {
              setEnd(e.target.value);
              clearRecovery();
            }}
          />
        </div>
        <button
          type="button"
          className="button secondary"
          disabled={busy || !enabled || !symbol || !start || !end}
          onClick={() => void load("fills/recover")}
        >
          Recuperar y guardar fills
        </button>
      </fieldset>
      {!enabled && (
        <p>
          La recuperación REST requiere habilitar el lector externo; el reporte
          de PostgreSQL sigue disponible.
        </p>
      )}
      <p className="muted-copy">
        Ventana máxima de siete días, sujeta a la retención y los límites de
        Binance. Esta acción sí contacta Binance y añade evidencia deduplicada;
        no restablece continuidad ni recupera automáticamente toda la historia.
        Inicio incluido y fin excluido. Las fechas se interpretan en UTC, no en
        tu zona horaria local. Una ventana saturada de un milisegundo también se
        informa como truncada.
      </p>
      {recovery && (
        <div>
          <p role="status">
            RECUPERACIÓN GUARDADA · {recovery.symbol} · SIN EJECUCIÓN
          </p>
          <p>
            Ventana UTC: {new Date(recovery.startTime).toISOString()} —{" "}
            {new Date(recovery.endTime).toISOString()}. Fills nuevos:{" "}
            {recovery.recovered}. Duplicados: {recovery.duplicates}.
          </p>
          {recovery.truncated && (
            <p className="notice danger">
              RECUPERACIÓN TRUNCADA: puede faltar evidencia en esta ventana. No
              se continúa automáticamente.
            </p>
          )}
          <p>
            Consulta de nuevo el reporte para ver la evidencia posterior al
            corte. Cero fills nuevos no demuestra ausencia de ejecuciones ni
            cierre de posiciones.
          </p>
        </div>
      )}
      {error && (
        <p className="notice danger" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
