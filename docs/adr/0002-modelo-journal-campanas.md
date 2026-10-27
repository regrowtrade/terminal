# ADR 0002 — Modelo del journal: la unidad es la campaña

- **Estado:** Propuesta. Pendiente de revisión y aprobación del propietario en el PR.
- **Fecha:** 2026-10-27.
- **Relacionado:** REG-13 (piloto `trading-journal`), [ADR 0001](0001-arquitectura-servicios.md), [inventario](../inventario-src.md).

## Contexto

El piloto REG-13 muestra cuentas, operaciones y estadísticas en modo lectura. Antes de implementarlo hay que decidir qué representa una fila del journal. El ADR 0001 decidió la arquitectura, pero no el modelo de datos.

Lo que existe hoy en `database/migrations/0001_foundation.sql`:

- `trade_campaigns`: campaña por cuenta e instrumento, con estados `DRAFT`, `ACTIVE`, `PAUSED`, `RECOVERING`, `CLOSING`, `CLOSED` y `ERROR`. Solo una campaña activa por instrumento y cuenta.
- `strategy_plans`: plan versionado por campaña, con modo `clean`, `dca` o `hedge`.
- `order_intents`: intención de orden ligada a un plan, con propósito (`ENTRY`, `REBUY`, `TP`, `SL`, `HEDGE_CLOSE`, `EMERGENCY_CLOSE`).
- `executions`: fills de cada intención, con precio, cantidad, comisión y PnL realizado, todo en `numeric(38,18)`.
- `campaign_ledger`: apuntes de `FUNDING` y `ADJUSTMENT` por campaña.

El modelo plano de `src/trading-journal/db/schema.ts` (`trades` con entrada y salida) no tiene equivalente en terminal y no se migra.

**Hecho relevante:** hoy `executions` y `campaign_ledger` no se alimentan. Ninguna pantalla de terminal muestra resultados reales. El piloto trabaja con datos sintéticos.

## Decisión

1. **La fila principal del journal es una campaña** (`trade_campaigns`).
2. El detalle de una campaña muestra sus ejecuciones (`executions`, a través de `order_intents`) y sus apuntes del ledger (`campaign_ledger`) como desglose de solo lectura.
3. Las estadísticas por cuenta se calculan en `apps/engine` a partir de campañas en estado `CLOSED` y de sus ejecuciones, con aritmética decimal. Las campañas abiertas se muestran aparte y no entran en el resultado realizado.
4. El modelo plano del legacy no se crea en terminal ni se migra.
5. Este ADR no crea tablas nuevas. Usa el esquema ya versionado en `0001_foundation.sql`.

## Alternativas consideradas

| Opción                                    | Ventajas                                                                                                         | Costes y riesgos                                                 | Resultado                                                   |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ----------------------------------------------------------- |
| **A. Campaña como fila**                  | Encaja con planes, recompras, cobertura y el límite de una campaña activa por instrumento; el esquema ya existe. | Depende de la ingesta de ejecuciones, que aún no existe.         | **Elegida.**                                                |
| **B. Operación plana** en una tabla nueva | Simple de mostrar; similar al legacy.                                                                            | Duplica el modelo de campañas; obliga a una migración posterior. | Descartada.                                                 |
| **C. Ejecución individual** como fila     | Fiel a Binance.                                                                                                  | Tabla técnica, sin agrupar por estrategia.                       | Descartada para la vista principal; sí se usa como detalle. |

## Consecuencias

- Las pruebas de REG-13 usan campañas y ejecuciones sintéticas. No hay datos reales ni importación desde Neon o el legacy.
- Mientras no exista la ingesta de ejecuciones, el journal real mostrará estados vacíos. Esto debe quedar explícito en la interfaz.
- Las métricas exactas (por ejemplo, tasa de acierto o comisiones acumuladas) se definirán en el contrato de REG-13 y se probarán con fixtures; no se fijan aquí.
- La ingesta de ejecuciones y del ledger es un ticket propio y bloquea cualquier resultado real.

## Seguridad y límites

- El journal de REG-13 es **solo lectura**. No crea, modifica ni borra campañas, planes ni ejecuciones.
- La propiedad se valida en el servidor a partir de la sesión. Las tablas tienen `owner_id` y claves compuestas que lo obligan.
- Esta decisión no habilita trading ni ejecución.

## Aprobación

Pendiente de revisión del propietario en el PR. No se considera aprobado hasta que el propietario lo confirme.
