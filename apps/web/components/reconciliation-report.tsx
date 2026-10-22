"use client";

import { useState } from "react";
import type { ReconciliationReport as Report } from "@/lib/reconciliation-schema";
import { ReadPagination, readPageSize } from "./read-pagination";

export function ReconciliationReport({ report }: { report: Report }) {
  const [page, setPage] = useState(0);
  return (
    <div>
      <p role="status">REPORTE PUNTUAL · NO EN VIVO · EJECUCIÓN BLOQUEADA</p>
      <p className="notice danger">
        {report.continuity === "gapped"
          ? "CONTINUIDAD CON HUECOS: el flujo está desconectado o hubo una interrupción desde la lectura base. Puede faltar evidencia."
          : "SIN HUECOS REGISTRADOS desde la lectura base al consultar el reporte. Esto no demuestra un estado completo ni sincronizado con Binance."}
      </p>
      <dl>
        <dt>Lectura base guardada</dt>
        <dd>{new Date(report.snapshotCapturedAt).toLocaleString("es")}</dd>
        <dt>Eventos consultados / actualizaciones de órdenes / cuenta</dt>
        <dd>
          {report.totalEvents} / {report.orderUpdates} / {report.accountUpdates}
        </dd>
        <dt>Último evento registrado</dt>
        <dd>
          {report.lastEventTime
            ? new Date(report.lastEventTime).toLocaleString("es")
            : "Sin eventos registrados"}
        </dd>
        <dt>Fills devueltos / total posterior al corte</dt>
        <dd>
          {report.fills.length} / {report.totalFills}
        </dd>
      </dl>
      {report.eventsTruncated && (
        <p className="notice danger">
          EVENTOS TRUNCADOS: el reporte no incluye todo el journal posterior al
          corte.
        </p>
      )}
      {report.fillsTruncated && (
        <p className="notice danger">
          FILLS TRUNCADOS: el límite del reporte es {report.fillLimit}. La
          paginación local no recupera los fills omitidos.
        </p>
      )}
      <p className="muted-copy">
        Solo PostgreSQL; esta consulta no contacta Binance. Corte aproximado por
        reloj del exchange y lecturas REST no atómicas. Los fills son evidencia
        deduplicada, no un ledger completo, PnL ni confirmación de cierre o
        protección. No se combinan con las posiciones como un estado actual.
      </p>
      {report.fills.length > 0 ? (
        <>
          <div className="table-scroll">
            <table>
              <caption>Fills posteriores al corte de la lectura base</caption>
              <thead>
                <tr>
                  <th>Orden / trade</th>
                  <th>Símbolo / lado / posición</th>
                  <th>Precio</th>
                  <th>Cantidad</th>
                  <th>Comisión / activo</th>
                  <th>Tiempo de transacción</th>
                </tr>
              </thead>
              <tbody>
                {report.fills
                  .slice(page * readPageSize, (page + 1) * readPageSize)
                  .map((fill) => (
                    <tr key={`${fill.orderId}:${fill.tradeId}`}>
                      <td>
                        {fill.orderId} / {fill.tradeId}
                      </td>
                      <td>
                        {fill.symbol} / {fill.side} / {fill.positionSide}
                      </td>
                      <td>{fill.price}</td>
                      <td>{fill.quantity}</td>
                      <td>
                        {fill.commission} / {fill.commissionAsset || "—"}
                      </td>
                      <td>
                        {new Date(fill.transactionTime).toLocaleString("es")}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <ReadPagination
            label="fills del reporte"
            total={report.fills.length}
            page={page}
            onPageChange={setPage}
          />
        </>
      ) : (
        <p>
          No hay fills en este reporte. No demuestra ausencia de ejecuciones ni
          cierre de posiciones.
        </p>
      )}
    </div>
  );
}
