"use client";

import { useState, useEffect } from "react";
import {
  connectionStateSchema,
  snapshotSchema,
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
  useEffect(() => {
    if (!snapshot) return;
    const tick = () =>
      setAge(Math.max(0, Math.floor((performance.now() - receivedAt) / 1000)));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [snapshot, receivedAt]);
  async function load(operation: "list" | "snapshot") {
    if (busy) return;
    setBusy(true);
    setError("");
    setSnapshot(null);
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
          <small>REST manual · Solo lectura · Sin órdenes</small>
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
      {state?.enabled && state.accounts.length > 0 && (
        <div>
          <label htmlFor="binance-account">Cuenta de lectura</label>
          <select
            id="binance-account"
            value={selected}
            onChange={(e) => {
              setSelected(e.target.value);
              setSnapshot(null);
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
            disabled={busy || !selected}
            onClick={() => void load("snapshot")}
          >
            Consultar saldos y posiciones
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
            {age >= 30 ? "LECTURA ANTIGUA" : "LECTURA PUNTUAL"} · Recibido hace{" "}
            {age} s · {new Date(snapshot.capturedAt).toLocaleString("es")}
          </p>
          <p className="muted-copy">
            Consultas REST no atómicas, sin sincronización continua ni órdenes
            normales/condicionales. No confirma protección ni disponibilidad
            para ejecutar. Esta lectura no alimenta automáticamente el
            planificador; las otras vistas siguen siendo ilustrativas.
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
        </section>
      )}
    </div>
  );
}
