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

El PR #5, `feat/binance-market-orders`, ya fue integrado por el propietario directamente en `main` e incorpora el nuevo alcance de órdenes/filtros y lecturas persistidas sobre esa consolidación. La rama `feat/binance-observation-comparison` parte de ese `main` actualizado y añade solo la comparación durable entre observaciones, no eventos privados ni ejecución. El propietario revisa diff/CI y hace el merge; el agente no lo hace.

Para futuras entregas dependientes, una vez integrado el PR anterior, cambiar la base del siguiente PR abierto a `main` y comprobar otra vez diff/CI **antes** de mergearlo. Crear las nuevas ramas desde `main` actualizado cuando todas sus dependencias estén allí. No considerar una entrega disponible en `main` solo porque GitHub marque su PR como mergeado.
