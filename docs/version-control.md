# Control de versiones y revisión

- `main` contiene únicamente cambios aprobados por el propietario. El agente no hace merge.
- Cada entrega usa una rama `fix/<tema>` o `feat/<tema>` y commits pequeños que separan correcciones de funcionalidades.
- Publicar un PR con alcance, pruebas realmente ejecutadas, pendientes y riesgos. El propietario revisa el código y hace el merge.
- No desplegar automáticamente una rama de trabajo ni conectar claves de producción a entornos de PR.
- Mantener lockfiles, contratos, migraciones y documentación en el mismo PR que los cambios de runtime.
- Los tags SemVer (`v0.1.1`, `v0.2.0`…) se crean sobre commits aprobados de `main`, no sobre funcionalidades pendientes o incompletas. No etiquetar esta base como `v0.2.0` completa.
- Correcciones compatibles: patch. Nuevas capacidades compatibles: minor. Rupturas de contrato: revisión explícita y documentación; durante `0.x` seguir documentando cualquier incompatibilidad.
- Nunca incluir `.env`, tokens, contraseñas ni claves de exchange. Revisar el diff antes de publicar.
- No reescribir el historial compartido ni hacer force-push sin aprobación.

## Entregas de transición revisadas

`feat/railway-foundation`: commit separado para la corrección del puerto de preview, seguido de infraestructura Railway y base PostgreSQL. No incluye datos privados del exchange ni habilita trading. La versión `0.1.0` del contrato de preview se conserva: `/readyz` es una adición compatible y no representa una release de ejecución real.

`feat/persistent-owner-sessions`: identidad personal, sesiones persistidas/revocables y auditoría. El PR #2 fue revisado y mergeado en `feat/railway-foundation`, después de que el PR #1 integrara infraestructura en `main`.

`feat/binance-readonly-vault`: credenciales cifradas y Binance solo lectura. El PR #3 fue revisado y mergeado en `feat/persistent-owner-sessions`. Sin ejecución ni despliegue automático de ramas de trabajo.

## Consolidación hacia `main`

Los tres PRs revisados no quedaron inicialmente todos en `main`: los merges de PRs encadenados solo actualizan su rama base, no las ramas donde esa base se hubiera integrado anteriormente. El propietario ya integró el PR #4 desde `chore/consolidate-approved-deliveries` en `main`, reuniendo sesiones y credenciales previamente aprobadas. No añade capacidades de trading ni acredita un despliegue.

Los PRs #5 (`feat/binance-market-orders`) y #6 (`feat/binance-observation-comparison`) ya fueron integrados por el propietario directamente en `main`: órdenes/filtros, lecturas persistidas y comparación durable entre observaciones. No añaden eventos privados ni ejecución.

El PR #7 (`feat/connected-account-views`) se creó desde `9cccd25` y se actualizó mediante merge de `origin/main` (`bc1c474`) después de integrar el PR #6. Se resuelven los solapamientos conservando comparación, vistas de cuenta, filtros y controles de ciclo de vida, con ambas suites de pruebas. No se reescribe el historial compartido. Esta actualización integra `main` en la rama del PR; no hace merge del PR en `main`. El propietario revisa diff/CI y hace ese merge final.

Para futuras entregas dependientes, una vez integrado el PR anterior, cambiar la base del siguiente PR abierto a `main` y comprobar otra vez diff/CI **antes** de mergearlo. Crear las nuevas ramas desde `main` actualizado cuando todas sus dependencias estén allí. No considerar una entrega disponible en `main` solo porque GitHub marque su PR como mergeado.

El propietario ya integró los PRs #9 (base de reconciliación, incluida la corrección de CI PostgreSQL `b1136fb`) y #10 (`feat/snapshot-event-buffering`) en `main`. Buffering, fills durables y transición administrativa de snapshots antiguos están allí desde `dd6e99d`; no habilitan trading.

`feat/binance-order-observations` parte de ese `main` actualizado: consulta manual de órdenes normales por ID/cliente y evidencia durable. Mantiene separadas la observación y la recuperación automática todavía pendiente. Sin despliegue, migraciones remotas ni merge por el agente.

El propietario integró el PR #11 en `main` (`ce60308`). `feat/persist-strategy-plans` parte de ahí: persistencia de planes como decisión durable (`trade_campaigns` DRAFT + `strategy_plans`), ligada a cuenta provisionada y con consumo acotado. El esquema ya existía en `0001`; no añade migración. Intenciones de órdenes deterministas y ejecución siguen pendientes en entregas posteriores.

El propietario integró el PR #12 (`7e1d840`). `feat/order-intents` parte de ese `main`: descompone un plan `clean`/`dca` en intenciones deterministas (`order_intents`) con `clientOrderId` estable e idempotente; la cobertura falla cerrada. No envía órdenes. El ejecutor, la reconciliación/recuperación de ejecución y las automatizaciones siguen pendientes.

El propietario integró el PR #13 (`c794bdc`). `feat/plans-api` parte de ese `main`: expone por HTTP el guardado y listado de planes e intenciones con proxy web y OpenAPI; aún sin UI de consumo. Sin órdenes ni despliegue.

El propietario integró el PR #14 (`fbf35f6`). `feat/plans-ui` parte de ese `main`: botón de guardado en el planificador y panel «Planes guardados» en ajustes; sin ejecución.
