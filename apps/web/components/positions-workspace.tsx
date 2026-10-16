"use client";

import { useState } from "react";
import { BinanceConnection } from "./binance-connection";
import { Positions } from "./positions";

export function PositionsWorkspace() {
  const [source, setSource] = useState<"account" | "examples">("account");
  return (
    <>
      <div
        className="data-source-controls"
        role="group"
        aria-label="Origen de posiciones"
      >
        <button
          className="button secondary"
          type="button"
          aria-pressed={source === "account"}
          onClick={() => setSource("account")}
        >
          Cuenta Binance · Solo lectura
        </button>
        <button
          className="button secondary"
          type="button"
          aria-pressed={source === "examples"}
          onClick={() => setSource("examples")}
        >
          Ejemplos de simulación
        </button>
      </div>
      {source === "account" ? (
        <section
          className="card account-read-card"
          aria-label="Cuenta Binance de solo lectura"
        >
          <h2>Posiciones y órdenes de la cuenta</h2>
          <p className="muted-copy">
            Sin consulta automática. Comprueba las cuentas disponibles y elige
            consultar Binance o recuperar una lectura guardada. No se mezclan
            ejemplos ni se asignan estrategias a posiciones u órdenes.
          </p>
          <BinanceConnection />
        </section>
      ) : (
        <>
          <p className="notice">
            EJEMPLOS DE SIMULACIÓN · Precios y cantidades ficticios, separados
            de la cuenta. La calculadora no importa datos ni ejecuta órdenes.
          </p>
          <Positions full />
        </>
      )}
    </>
  );
}
