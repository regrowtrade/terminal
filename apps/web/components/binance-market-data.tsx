"use client";

import { useState } from "react";
import type { AccountSnapshot } from "@/lib/connections-schema";

export function BinanceMarketData({ snapshot }: { snapshot: AccountSnapshot }) {
  const [symbol, setSymbol] = useState(
    snapshot.positions[0]?.symbol ?? snapshot.instruments[0].symbol,
  );
  const instrument =
    snapshot.instruments.find((i) => i.symbol === symbol) ??
    snapshot.instruments[0];
  return (
    <>
      <div className="table-scroll">
        <table>
          <caption>
            Órdenes abiertas normales ({snapshot.openOrders.length}) · No son
            fills confirmados
          </caption>
          <thead>
            <tr>
              <th>ID / símbolo</th>
              <th>Dirección / posición</th>
              <th>Tipo / estado</th>
              <th>Cantidad / ejecutada</th>
              <th>Precio / disparo</th>
              <th>Indicadores del exchange</th>
            </tr>
          </thead>
          <tbody>
            {snapshot.openOrders.map((o) => (
              <tr key={`${o.symbol}:${o.orderId}`}>
                <td>
                  {o.orderId}
                  <br />
                  {o.symbol}
                </td>
                <td>
                  {o.side} / {o.positionSide}
                </td>
                <td>
                  {o.type} / {o.status}
                </td>
                <td>
                  {o.origQty} / {o.executedQty}
                </td>
                <td>
                  {o.price} /{" "}
                  {o.type === "TRAILING_STOP_MARKET"
                    ? `Activación ${o.activatePrice} · Callback ${o.priceRate}%`
                    : o.stopPrice}
                </td>
                <td>
                  Reduce-only: {o.reduceOnly ? "sí" : "no"} · Cierre total:{" "}
                  {o.closePosition ? "sí" : "no"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {snapshot.openOrders.length === 0 && (
        <p>No se devolvieron órdenes normales abiertas en esta consulta.</p>
      )}
      <div className="table-scroll">
        <table>
          <caption>
            Órdenes condicionales / algo ({snapshot.algoOrders.length}) ·
            Protección no verificada
          </caption>
          <thead>
            <tr>
              <th>ID / símbolo</th>
              <th>Dirección / posición</th>
              <th>Tipo / estado</th>
              <th>Cantidad</th>
              <th>Precio / condición</th>
              <th>Indicadores del exchange</th>
            </tr>
          </thead>
          <tbody>
            {snapshot.algoOrders.map((o) => (
              <tr key={`${o.symbol}:${o.algoId}`}>
                <td>
                  {o.algoId}
                  <br />
                  {o.symbol}
                </td>
                <td>
                  {o.side} / {o.positionSide}
                </td>
                <td>
                  {o.orderType} / {o.algoStatus}
                </td>
                <td>
                  {o.closePosition
                    ? "Cierre total indicado (sin cantidad fija)"
                    : o.quantity}
                </td>
                <td>
                  {o.price} /{" "}
                  {o.orderType === "TRAILING_STOP_MARKET"
                    ? `Activación ${o.activatePrice} · Callback ${o.callbackRate}%`
                    : o.triggerPrice}
                  <br />
                  {o.workingType}
                </td>
                <td>
                  Reduce-only: {o.reduceOnly ? "sí" : "no"} · Cierre total:{" "}
                  {o.closePosition ? "sí" : "no"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {snapshot.algoOrders.length === 0 && (
        <p>
          No se devolvieron órdenes condicionales abiertas en esta consulta.
        </p>
      )}
      <p className="muted-copy">
        Una orden abierta o condicional no demuestra cobertura suficiente ni
        ejecución futura. No se infieren fills, cancelaciones o cierres a partir
        de su ausencia. Las dos listas se consultan en momentos distintos.
      </p>
      <details>
        <summary>
          Instrumentos y filtros USDT perpetuos ({snapshot.instruments.length})
        </summary>
        <label htmlFor="binance-instrument">Instrumento consultado</label>
        <select
          id="binance-instrument"
          value={instrument.symbol}
          onChange={(e) => setSymbol(e.target.value)}
        >
          {snapshot.instruments.map((i) => (
            <option key={i.symbol} value={i.symbol}>
              {i.symbol} · {i.status}
            </option>
          ))}
        </select>
        <p>
          {instrument.contractType} · Margen {instrument.marginAsset} · Estado
          del exchange: {instrument.status}
        </p>
        <div className="table-scroll">
          <table>
            <caption>
              Filtros publicados · Solo información, no validación de una orden
            </caption>
            <thead>
              <tr>
                <th>Filtro</th>
                <th>Mínimo</th>
                <th>Máximo</th>
                <th>Paso real</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Precio</td>
                <td>{instrument.priceFilter.min}</td>
                <td>{instrument.priceFilter.max}</td>
                <td>{instrument.priceFilter.step}</td>
              </tr>
              <tr>
                <td>Cantidad límite</td>
                <td>{instrument.lotSize.min}</td>
                <td>{instrument.lotSize.max}</td>
                <td>{instrument.lotSize.step}</td>
              </tr>
              <tr>
                <td>Cantidad mercado</td>
                <td>{instrument.marketLotSize.min}</td>
                <td>{instrument.marketLotSize.max}</td>
                <td>{instrument.marketLotSize.step}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p>
          Nocional mínimo: {instrument.minNotional} USDT · Multiplicadores de
          precio: {instrument.multiplierDown} / {instrument.multiplierUp} ·
          Límites de órdenes normales/algo: {instrument.maxNumOrders} /{" "}
          {instrument.maxNumAlgoOrders}
        </p>
        {instrument.unrecognizedFilterTypes.length > 0 && (
          <p className="notice danger">
            Filtros no interpretados:{" "}
            {instrument.unrecognizedFilterTypes.join(", ")}. No se ha validado
            su efecto.
          </p>
        )}
        <p className="muted-copy">
          No se usa pricePrecision ni quantityPrecision como tick/step. Un
          componente cero puede indicar un filtro desactivado. El planificador
          de simulación no redondea ni autoriza órdenes con estos datos.
        </p>
      </details>
    </>
  );
}
