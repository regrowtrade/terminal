"use client";

import { useEffect, useRef, useState } from "react";
import {
  orderLookupSchema,
  orderQuerySchema,
  type OrderLookup,
} from "@/lib/order-lookup-schema";

// Mounted with the account as its key: switching accounts aborts and discards
// in-flight observations rather than showing them under a different account.
export function BinanceOrderLookup({
  accountId,
  disabled,
}: {
  accountId: string;
  disabled: boolean;
}) {
  const [symbol, setSymbol] = useState("");
  const [selector, setSelector] = useState("orderId");
  const [identifier, setIdentifier] = useState("");
  const [result, setResult] = useState<OrderLookup | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);
  function clear() {
    setResult(null);
    setError("");
  }
  async function lookup() {
    if (disabled || active.current) return;
    clear();
    const parsed = orderQuerySchema.safeParse({
      symbol,
      [selector]: identifier,
    });
    if (!parsed.success) {
      setError(
        "Indica un símbolo USDT y un identificador válido. Usa un único tipo de ID.",
      );
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    try {
      const signal = AbortSignal.any([
        controller.signal,
        AbortSignal.timeout(22_000),
      ]);
      const response = await fetch("/api/connections/binance/order", {
        method: "POST",
        cache: "no-store",
        redirect: "error",
        signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId, ...parsed.data }),
      });
      if (!response.ok) throw new Error("Lookup unavailable");
      const body: unknown = await response.json();
      if (controller.signal.aborted) return;
      signal.throwIfAborted();
      const next = orderLookupSchema.parse(body);
      if (
        next.query.symbol !== parsed.data.symbol ||
        next.query.orderId !== parsed.data.orderId ||
        next.query.clientOrderId !== parsed.data.clientOrderId
      )
        throw new Error("Selector mismatch");
      setResult(next);
    } catch {
      if (!controller.signal.aborted)
        setError(
          "No se pudo verificar y guardar la consulta. No se muestran resultados anteriores ni se enviaron órdenes. Espera antes de reintentar.",
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
      className="order-lookup"
      aria-label="Consulta de orden"
      aria-busy={busy}
    >
      <h3>Investigar una orden</h3>
      <p className="muted-copy">
        Consulta manual de LIMIT/MARKET o condicional (STOP/TAKE_PROFIT) por
        símbolo e ID. No consulta órdenes algo, no reconstruye fills y no
        confirma protección ni cierre de posición.
      </p>
      <label htmlFor="order-lookup-symbol">Símbolo USDT</label>
      <input
        id="order-lookup-symbol"
        value={symbol}
        maxLength={28}
        disabled={busy || disabled}
        onChange={(e) => {
          setSymbol(e.target.value);
          clear();
        }}
        placeholder="BTCUSDT"
      />
      <label htmlFor="order-lookup-selector">Tipo de identificador</label>
      <select
        id="order-lookup-selector"
        value={selector}
        disabled={busy || disabled}
        onChange={(e) => {
          setSelector(e.target.value);
          setIdentifier("");
          clear();
        }}
      >
        <option value="orderId">ID del exchange</option>
        <option value="clientOrderId">ID de cliente</option>
      </select>
      <label htmlFor="order-lookup-id">Identificador de orden</label>
      <input
        id="order-lookup-id"
        value={identifier}
        maxLength={selector === "orderId" ? 19 : 36}
        disabled={busy || disabled}
        onChange={(e) => {
          setIdentifier(e.target.value);
          clear();
        }}
      />
      <button
        type="button"
        className="button secondary"
        disabled={busy || disabled || !symbol || !identifier}
        onClick={() => void lookup()}
      >
        {busy ? "Verificando…" : "Verificar y guardar consulta de orden"}
      </button>
      {error && (
        <p role="alert" className="notice danger">
          {error}
        </p>
      )}
      {result && (
        <div>
          <p role="status">
            OBSERVACIÓN PUNTUAL GUARDADA · NO EN VIVO ·{" "}
            {new Date(result.checkedAt).toLocaleString("es")}
          </p>
          {result.outcome === "unresolved" ? (
            <p className="notice danger">
              NO RESUELTA: Binance no devolvió la orden. Esto no demuestra que
              nunca existió, que se canceló o que se ejecutó. No reenvíes una
              orden basándote en esta ausencia.
            </p>
          ) : (
            result.order && (
              <dl>
                <dt>Orden / cliente</dt>
                <dd>
                  {result.order.orderId} / {result.order.clientOrderId}
                </dd>
                <dt>Símbolo / lado / posición</dt>
                <dd>
                  {result.order.symbol} / {result.order.side} /{" "}
                  {result.order.positionSide}
                </dd>
                <dt>Tipo / estado observado</dt>
                <dd>
                  {result.order.type} / {result.order.status}
                </dd>
                <dt>Cantidad original / ejecutada acumulada</dt>
                <dd>
                  {result.order.origQty} / {result.order.executedQty}
                </dd>
                <dt>Precio / precio medio observado</dt>
                <dd>
                  {result.order.price} / {result.order.avgPrice}
                </dd>
                {result.order.type !== "LIMIT" &&
                  result.order.type !== "MARKET" && (
                    <>
                      <dt>Disparo / tipo de precio de activación</dt>
                      <dd>
                        {result.order.stopPrice || "—"} /{" "}
                        {result.order.workingType || "—"}
                      </dd>
                      {result.order.type === "TRAILING_STOP_MARKET" && (
                        <>
                          <dt>Activación / tasa de retroceso</dt>
                          <dd>
                            {result.order.activatePrice || "—"} /{" "}
                            {result.order.priceRate || "—"}
                          </dd>
                        </>
                      )}
                    </>
                  )}
                <dt>Actualización declarada por Binance</dt>
                <dd>
                  {new Date(result.order.updateTime).toLocaleString("es")}
                </dd>
              </dl>
            )
          )}
          <p className="muted-copy">
            Solo evidencia de esta consulta. Sin asignación a estrategias, PnL
            ni recuperación automática; ejecución bloqueada.
          </p>
        </div>
      )}
    </section>
  );
}
