"use client";

import { useState, useEffect } from "react";
import { BinanceMarketData } from "./binance-market-data";
import { ObservationComparison } from "./observation-comparison";
import {
  storedComparisonSchema,
  type StoredComparison,
} from "@/lib/observation-schema";
import {
  connectionStateSchema,
  snapshotSchema,
  storedSnapshotSchema,
  type ConnectionState,
  type AccountSnapshot,
} from "@/lib/connections-schema";

export function BinanceConnection() {
  const [state, setState] = useState<ConnectionState | null>(null);
  const [snapshot, setSnapshot] = useState<AccountSnapshot | null>(null);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [age, setAge] = useState(0);
  const [receivedAt, setReceivedAt] = useState(0);
  const [storedAt, setStoredAt] = useState<string | null>(null);
  const [comparison, setComparison] = useState<StoredComparison | null>(null);
  useEffect(() => {
    if (!snapshot) return;
    const tick = () =>
      setAge(Math.max(0, Math.floor((performance.now() - receivedAt) / 1000)));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [snapshot, receivedAt]);
  async function load(
    operation: "list" | "snapshot" | "latest" | "comparison",
  ) {
    if (busy) return;
    setBusy(true);
    setError("");
    setSnapshot(null);
    setStoredAt(null);
    setComparison(null);
    try {
      const response = await fetch(`/api/connections/binance/${operation}`, {
        method: "POST",
        cache: "no-store",
        redirect: "error",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          operation === "list" ? {} : { accountId: selected },
        ),
      });
      const body: unknown = await response.json();
      if (!response.ok) {
        setError(
          typeof body === "object" &&
            body !== null &&
            "error" in body &&
            typeof body.error === "string"
            ? body.error
            : "Lectura no disponible.",
        );
        return;
      }
      if (operation === "list") {
        const next = connectionStateSchema.parse(body);
        setState(next);
        setSelected(next.accounts[0]?.id ?? "");
      } else if (operation === "comparison") {
        setComparison(storedComparisonSchema.parse(body));
      } else if (operation === "latest") {
        const stored = storedSnapshotSchema.parse(body);
        setSnapshot(stored.snapshot);
        setStoredAt(stored.storedAt);
        setReceivedAt(performance.now());
      } else {
        setSnapshot(snapshotSchema.parse(body));
        setReceivedAt(performance.now());
      }
    } catch {
      setError(
        "No se pudo leer la cuenta. No se muestran datos anteriores como actuales.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="binance-readonly">
      <div className="exchange-row">
        <span className="exchange-icon binance">B</span>
        <div>
          <strong>Binance Futures USDT</strong>
          <small>REST manual · Solo lectura · Sin ejecución</small>
        </div>
        <span className="pill muted">
          {state?.enabled
            ? "LECTURA HABILITADA"
            : "DESACTIVADO / SIN COMPROBAR"}
        </span>
      </div>
      <p className="muted-copy">
        Las claves se provisionan en una consola administrativa con entrada
        oculta. No pegues claves aquí. Requiere PostgreSQL, cifrado e IP de
        salida fija autorizada; Railway Hobby aún tiene pendiente esa salida.
      </p>
      <button
        className="button secondary"
        type="button"
        disabled={busy}
        onClick={() => void load("list")}
      >
        {busy ? "Consultando…" : "Comprobar conexión"}
      </button>
      {state && !state.enabled && (
        <p role="status">
          El motor mantiene la lectura real desactivada. No se contactó a
          Binance.
        </p>
      )}
      {state?.enabled && state.accounts.length === 0 && (
        <p role="status">No hay credenciales provisionadas.</p>
      )}
      {state && state.accounts.length > 0 && (
        <div>
          <label htmlFor="binance-account">Cuenta de lectura</label>
          <select
            id="binance-account"
            value={selected}
            onChange={(e) => {
              setSelected(e.target.value);
              setSnapshot(null);
              setStoredAt(null);
              setComparison(null);
              setError("");
            }}
            disabled={busy}
          >
            {state.accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          <button
            className="button secondary"
            type="button"
            disabled={busy || !selected || !state.enabled}
            onClick={() => void load("snapshot")}
          >
            Consultar cuenta y órdenes
          </button>
          <button
            className="button secondary"
            type="button"
            disabled={busy || !selected}
            onClick={() => void load("latest")}
          >
            Mostrar última lectura guardada
          </button>
          <button
            className="button secondary"
            type="button"
            disabled={busy || !selected}
            onClick={() => void load("comparison")}
          >
            Ver cambios de la última lectura
          </button>
        </div>
      )}
      {error && (
        <p className="notice danger" role="alert">
          {error}
        </p>
      )}
      {snapshot && (
        <section aria-label="Lectura de Binance">
          <p role="status">
            {storedAt
              ? "LECTURA GUARDADA · NO ACTUAL"
              : age >= 30
                ? "LECTURA ANTIGUA"
                : "LECTURA PUNTUAL"}{" "}
            · Recibido hace {age} s ·{" "}
            {new Date(snapshot.capturedAt).toLocaleString("es")}
          </p>
          {storedAt && (
            <p className="notice danger">
              Recuperada de PostgreSQL; guardada el{" "}
              {new Date(storedAt).toLocaleString("es")}. No se contactó a
              Binance y no se verificó el estado actual de la cuenta.
            </p>
          )}
          <p className="muted-copy">
            Consultas REST no atómicas, sin sincronización continua. Incluye
            saldos, posiciones, órdenes normales/condicionales e instrumentos.
            No confirma protección ni disponibilidad para ejecutar. Esta lectura
            no alimenta automáticamente el planificador; las otras vistas siguen
            siendo ilustrativas.
          </p>
          <p className="muted-copy">
            Inicio de lectura:{" "}
            {new Date(snapshot.startedAt).toLocaleString("es")} · Fin de
            lectura: {new Date(snapshot.capturedAt).toLocaleString("es")}. La
            antigüedad de cada dato puede ser mayor que el tiempo desde la
            recepción.
          </p>
          <div className="table-scroll">
            <table>
              <caption>Saldos por activo (no sumar monedas distintas)</caption>
              <thead>
                <tr>
                  <th>Activo</th>
                  <th>Saldo</th>
                  <th>Disponible</th>
                  <th>PnL flotante</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.assets.map((a) => (
                  <tr key={a.asset}>
                    <td>{a.asset}</td>
                    <td>{a.walletBalance}</td>
                    <td>{a.availableBalance}</td>
                    <td>{a.unrealizedProfit}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="table-scroll">
            <table>
              <caption>
                Posiciones USDT · Cantidades y precios decimales
              </caption>
              <thead>
                <tr>
                  <th>Activo / lado</th>
                  <th>Cantidad</th>
                  <th>Entrada</th>
                  <th>Marca</th>
                  <th>PnL flotante</th>
                  <th>Liquidación estimada</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.positions.map((p) => (
                  <tr key={`${p.symbol}:${p.positionSide}`}>
                    <td>
                      {p.symbol} · {p.positionSide}
                    </td>
                    <td>{p.positionAmt}</td>
                    <td>{p.entryPrice}</td>
                    <td>{p.markPrice}</td>
                    <td>{p.unRealizedProfit}</td>
                    <td>{p.liquidationPrice}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {snapshot.positions.length === 0 && (
            <p>
              No hay posiciones devueltas por esta consulta; no implica ausencia
              de órdenes ni protección verificada.
            </p>
          )}
          <BinanceMarketData key={snapshot.capturedAt} snapshot={snapshot} />
        </section>
      )}
      {comparison && <ObservationComparison comparison={comparison} />}
    </div>
  );
}
