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

## Primera rama de transición

`feat/railway-foundation`: commit separado para la corrección del puerto de preview, seguido de infraestructura Railway y base PostgreSQL. No incluye datos privados del exchange ni habilita trading. La versión `0.1.0` del contrato de preview se conserva: `/readyz` es una adición compatible y no representa una release de ejecución real.

`feat/persistent-owner-sessions`: entrega dependiente con identidad personal, sesiones persistidas/revocables y auditoría. Mientras infraestructura esté pendiente, su PR apunta a `feat/railway-foundation` para mostrar solo el nuevo alcance. Revisar/mergear primero infraestructura; después cambiar la base del segundo PR a `main` y comprobar diff/CI. No mergear la segunda rama dentro de la primera para saltar la revisión por entrega. Sin tags de release antes de aprobación.
