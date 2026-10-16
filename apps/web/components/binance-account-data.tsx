"use client";

import { useId, useState } from "react";
import type { AccountSnapshot } from "@/lib/connections-schema";
import {
  allAccountRecords,
  matchesAccountFilter,
  type AccountFilter,
} from "@/lib/account-filter";
import { BinanceMarketData } from "./binance-market-data";
import { ReadPagination, readPageSize } from "./read-pagination";

export function BinanceAccountData({
  snapshot,
}: {
  snapshot: AccountSnapshot;
}) {
  const [filter, setFilter] = useState<AccountFilter>(allAccountRecords);
  const queryId = useId();
  const sideId = useId();
  return (
    <>
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
      {snapshot.assets.length === 0 && (
        <p>No se devolvieron saldos en esta lectura.</p>
      )}
      <fieldset className="account-filters">
        <legend>Filtrar posiciones y órdenes de esta lectura</legend>
        <div className="account-filter-field">
          <label htmlFor={queryId}>Buscar símbolo</label>
          <input
            id={queryId}
            type="search"
            maxLength={30}
            value={filter.query}
            onChange={(e) => setFilter({ ...filter, query: e.target.value })}
            placeholder="BTCUSDT"
          />
        </div>
        <div className="account-filter-field">
          <label htmlFor={sideId}>Lado de posición</label>
          <select
            id={sideId}
            value={filter.positionSide}
            onChange={(e) =>
              setFilter({
                ...filter,
                positionSide: e.target.value as AccountFilter["positionSide"],
              })
            }
          >
            <option value="all">Todos los lados</option>
            <option value="LONG">LONG</option>
            <option value="SHORT">SHORT</option>
            <option value="BOTH">BOTH · Modo unidireccional</option>
          </select>
        </div>
        <button
          className="button secondary"
          type="button"
          onClick={() => setFilter(allAccountRecords)}
        >
          Quitar filtros
        </button>
      </fieldset>
      <p className="muted-copy">
        Filtros locales, sin peticiones nuevas. No filtran saldos ni catálogo de
        instrumentos. LONG/SHORT/BOTH es el lado de posición publicado por
        Binance, no la dirección BUY/SELL de una orden. Se conservan cantidades
        con signo y registros con cantidad cero; no se deduce una estrategia ni
        un cierre confirmado.
      </p>
      <FilteredPositions
        key={`${filter.query}:${filter.positionSide}`}
        snapshot={snapshot}
        filter={filter}
      />
      <BinanceMarketData
        key={`orders:${filter.query}:${filter.positionSide}`}
        snapshot={snapshot}
        filter={filter}
      />
    </>
  );
}

function FilteredPositions({
  snapshot,
  filter,
}: {
  snapshot: AccountSnapshot;
  filter: AccountFilter;
}) {
  const [page, setPage] = useState(0);
  const rows = snapshot.positions.filter((p) =>
    matchesAccountFilter(p, filter),
  );
  const current = Math.min(
    page,
    Math.max(0, Math.ceil(rows.length / readPageSize) - 1),
  );
  return (
    <>
      <div className="table-scroll">
        <table>
          <caption>
            Posiciones USDT · Cantidades y precios decimales ({rows.length} de{" "}
            {snapshot.positions.length} registros de la lectura coinciden)
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
            {rows
              .slice(current * readPageSize, (current + 1) * readPageSize)
              .map((p) => (
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
      <ReadPagination
        label="posiciones"
        total={rows.length}
        page={current}
        onPageChange={setPage}
      />
      {rows.length === 0 && (
        <p>
          {snapshot.positions.length === 0
            ? "No hay posiciones devueltas por esta consulta; no implica ausencia de órdenes ni protección verificada."
            : "No hay posiciones que coincidan con los filtros; la lectura sí contiene otros registros."}
        </p>
      )}
    </>
  );
}
