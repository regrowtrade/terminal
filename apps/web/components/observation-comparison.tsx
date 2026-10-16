import type { StoredComparison } from "@/lib/observation-schema";

const entities = {
  asset: "Saldo por activo",
  position: "Posición",
  order: "Orden normal",
  algo: "Orden condicional",
  instrument: "Instrumento / filtros",
};
const kinds = {
  observed: "Observado ahora",
  not_observed: "Ya no aparece · estado final desconocido",
  changed: "Cambió entre lecturas",
};

export function ObservationComparison({
  comparison,
}: {
  comparison: StoredComparison;
}) {
  const { report, storedAt } = comparison;
  return (
    <section aria-label="Comparación guardada de Binance">
      <p role="status">COMPARACIÓN GUARDADA · NO ESTADO ACTUAL</p>
      <p className="muted-copy">
        Guardada el {new Date(storedAt).toLocaleString("es")}. Recuperada de
        PostgreSQL sin consultar a Binance.
      </p>
      <p className="notice danger">
        Comparación de lecturas REST no atómicas. No confirma cancelaciones,
        fills, cierres, protección ni continuidad de eventos. No habilita
        ejecución.
      </p>
      <p>
        Lectura comparada:{" "}
        {new Date(report.currentStartedAt).toLocaleString("es")} —{" "}
        {new Date(report.currentCapturedAt).toLocaleString("es")}
      </p>
      {report.previousCapturedAt && (
        <p>
          Captura anterior:{" "}
          {new Date(report.previousCapturedAt).toLocaleString("es")}
        </p>
      )}
      {report.status === "initial_observation" ? (
        <p>
          Primera observación completa: no hay una lectura anterior para
          comparar.
        </p>
      ) : (
        <>
          {report.intervalRelation === "overlapping" && (
            <p className="notice danger">
              Los intervalos de lectura se solapan. Las diferencias no
              establecen una secuencia de eventos.
            </p>
          )}
          <p>
            {report.totalChanges} diferencias de registros observadas. Se
            muestran {report.changes.length}
            {report.truncated
              ? " · DETALLE LIMITADO: hay diferencias adicionales no mostradas"
              : ""}
            .
          </p>
          {report.totalChanges === 0 && (
            <p>
              No se detectaron diferencias en los campos comparados; esto no
              demuestra sincronización ni ausencia de actividad entre lecturas.
            </p>
          )}
          {report.changes.length > 0 && (
            <div className="table-scroll">
              <table>
                <caption>
                  Diferencias observadas · Importes y cantidades sin conversión
                  a float
                </caption>
                <thead>
                  <tr>
                    <th>Registro</th>
                    <th>Observación</th>
                    <th>Campo</th>
                    <th>Antes</th>
                    <th>Después</th>
                  </tr>
                </thead>
                <tbody>
                  {report.changes.flatMap((change) =>
                    change.fields.map((field, index) => (
                      <tr key={`${change.entity}:${change.key}:${field.field}`}>
                        <td>
                          {index === 0 ? (
                            <>
                              {entities[change.entity]}
                              <br />
                              {change.key}
                            </>
                          ) : null}
                        </td>
                        <td>{index === 0 ? kinds[change.kind] : null}</td>
                        <td>{field.field}</td>
                        <td>{field.before ?? "No observado"}</td>
                        <td>{field.after ?? "No observado"}</td>
                      </tr>
                    )),
                  )}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      <p className="muted-copy">
        Se comparan los campos normalizados de saldos, posiciones, órdenes y
        filtros. Cambios solo de formato decimal o de updateTime no se cuentan.
        Un registro ausente no se sustituye por una cantidad cero y el acumulado
        ejecutado no es un ledger de fills. Sigue faltando reconciliar eventos
        privados y períodos de desconexión.
      </p>
    </section>
  );
}
