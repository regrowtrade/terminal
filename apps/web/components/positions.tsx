"use client";

import Link from "next/link";
import { useState } from "react";
import { ArrowDownLeft, ArrowUpRight, Search } from "lucide-react";
import { examplePositions, money } from "@/lib/demo";
import { useWorkspace } from "./workspace";

export function Positions({ full = false }: { full?: boolean }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const { openPlan } = useWorkspace();
  const rows = examplePositions.filter(
    (p) =>
      p.symbol.includes(query.toUpperCase()) &&
      (filter === "all" || p.side === filter),
  );
  return (
    <section className="card positions-card">
      <div className="card-heading">
        <div className="heading-inline">
          <h2>{full ? "Explorar posiciones" : "Posiciones de ejemplo"}</h2>
          <span className="count-badge">{rows.length}</span>
        </div>
        {full ? (
          <div className="positions-controls">
            <label className="search-field">
              <Search size={15} />
              <input
                aria-label="Buscar activo"
                placeholder="Buscar activo"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <select
              aria-label="Filtrar por lado"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            >
              <option value="all">Todos los lados</option>
              <option value="LONG">LONG</option>
              <option value="SHORT">SHORT</option>
            </select>
          </div>
        ) : (
          <Link href="/positions" className="text-link">
            Ver posiciones <ArrowUpRight size={14} />
          </Link>
        )}
      </div>
      <div className="table-scroll">
        <table>
          <caption className="sr-only">
            Posiciones sintéticas, con precios ilustrativos. No conectadas a un
            exchange.
          </caption>
          <thead>
            <tr>
              <th>ACTIVO / LADO</th>
              <th>ESTRATEGIA</th>
              <th className="align-right">ENTRADA</th>
              <th className="align-right">CANTIDAD</th>
              <th className="align-right">PNL FLOTANTE</th>
              <th>
                <span className="sr-only">Acción</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id}>
                <td>
                  <div className="asset-cell">
                    <span className={`asset-icon ${p.color}`}>
                      {p.asset === "BTC" ? "₿" : "Ξ"}
                    </span>
                    <span>
                      <strong>
                        {p.asset}
                        <small> / USDT</small>
                      </strong>
                      <span className={`side-label ${p.side.toLowerCase()}`}>
                        {p.side === "LONG" ? (
                          <ArrowUpRight size={11} />
                        ) : (
                          <ArrowDownLeft size={11} />
                        )}
                        {p.side}
                      </span>
                    </span>
                  </div>
                </td>
                <td>
                  <span className="strategy-tag">{p.strategy}</span>
                </td>
                <td className="align-right numeric">{money(p.entry)}</td>
                <td className="align-right numeric">{p.quantity}</td>
                <td
                  className={`align-right numeric ${p.pnl >= 0 ? "positive" : "negative"}`}
                >
                  {p.pnl >= 0 ? "+" : ""}
                  {money(p.pnl)} <small>USDT</small>
                </td>
                <td className="align-right">
                  <button
                    className="table-action"
                    onClick={() =>
                      openPlan(p.strategy === "Cobertura" ? "hedge" : "clean")
                    }
                    aria-label={`Calcular ejemplo ${p.symbol} ${p.side}`}
                  >
                    <ArrowUpRight size={17} />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && (
          <p className="empty-inline" role="status">
            No hay ejemplos que coincidan con tu búsqueda.
          </p>
        )}
      </div>
      <div className="card-footnote">
        Precios ficticios para explorar la interfaz. No son cotizaciones de
        mercado.
      </div>
    </section>
  );
}
