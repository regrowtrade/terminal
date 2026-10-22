# Revisión local para integrar la recuperación de fills y su UI

Fecha: 22 de octubre de 2026. Rama de entrega: `feat/account-reconciliation-delivery`, creada desde `origin/main` (`25fa298`) tras verificar el merge del PR #18 (`feat/binance-fill-recovery`, `094dd15`). Conserva la UI y las correcciones que seguían locales en `feat/account-reconciliation-ui`. Esa base ya está aprobada; **la UI y las correcciones de esta revisión todavía no están integradas en main**.

## Alcance

- Posiciones y Configuración consumen manualmente el reporte de reconciliación y la recuperación REST. Sin polling ni estado fundido de posiciones/órdenes; siempre `executionReady=false`.
- Se mantienen las APIs existentes y no se añade ninguna migración. La activación requiere las migraciones/grants existentes y los prerrequisitos operativos de lectura.
- Los repositorios de `../src` no se modifican ni se ejecutan.

## Hallazgos corregidos antes de integrar

1. **Protocolo REST incompatible:** la recuperación combinaba `fromId` con `startTime`/`endTime` y suponía una paginación hacia IDs anteriores. Ahora subdivide ventanas saturadas, sin asumir el orden de la respuesta. Mantiene 20 consultas de trades como máximo, inicio incluido/fin excluido y truncamiento visible si queda trabajo o se satura un milisegundo.
2. **Ventana vacía rechazada:** ahora una lista vacía válida devuelve cero después de revalidar sesión, cuenta y credencial, también tras I/O. No se confunde con ausencia de ejecuciones.
3. **Evidencia fuera de criterios:** adaptador y servicio rechazan otro símbolo o tiempos fuera de ventana; servicio y base repiten validación de IDs, decimales y reloj REST. No se aceptan redeliveries contradictorias ni resultados parciales después de un error.
4. **Respuesta web no ligada a la petición:** proxy e interfaz verifican símbolo e inicio/fin; el schema limita ventana/conteos y rechaza `executionReady=true` y campos desconocidos.
5. **Comisiones con signo:** el reporte web y OpenAPI admiten las comisiones REST con su signo original. No se redondean ni agregan activos ni se calcula PnL.
6. **Auditoría:** fills y `account.fills_recovered` (solo conteos) se confirman juntos; una auditoría fallida revierte los inserts y no devuelve conteos parciales.

## Verificación

- `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`.
- `go test ./...`, `go vet ./...` desde `apps/engine`.
- `npm run test:e2e`: escritorio/móvil, respuestas de cuenta sintéticas, sin claves ni exchange.
- Resultado local: **215 pruebas web** y **22 pruebas de navegador** correctas. Las comprobaciones Go y PostgreSQL descritas también pasaron; esto no acredita CI remoto ni operación con una cuenta real.
- `npm run format:check` en este checkout Windows informa finales CRLF en archivos existentes. Se corrigió el formato de la nueva prueba y se comprobaron los **125 archivos versionados** incluidos en el comando con finales LF normalizados en memoria: sin discrepancias de formato. No se reformateó el repositorio completo; el check de CI en Linux sigue siendo la puerta autoritativa.
- PostgreSQL **15 local desechable**, en loopback y puerto aleatorio, con credenciales efímeras: `TEST_DATABASE_URL` configurada solo para la ejecución de `go test -count=1 -v ./internal/database ./cmd/snapshot-transition` y `node scripts/test-standalone.mjs`. Cluster detenido y eliminado tras verificar.
- La ejecución PostgreSQL comprobó migraciones/RLS, recuperación vacía, deduplicación, credenciales retiradas, revocación, cuentas de otro propietario/entorno, conservación de snapshot/continuidad y rollback ante fallo de auditoría. El standalone de producción verificó login, preview y revocación individual/global con rechazo de cookies copiadas.

Las pruebas del adaptador usan transportes sintéticos que exigen GET firmado, host fijo, selectores compatibles y respuestas dentro de cada subventana. Cubren distintos órdenes de respuesta, límites inclusivo/exclusivo, IDs mayores que el entero seguro JS, comisiones con signo, duplicados, ventanas vacías y saturación. No validan el comportamiento operativo de una cuenta real ni sustituyen CI.

## Puertas pendientes

1. Revisar las correcciones REST/proxy y el consumo UI como cambios separados en la entrega. La dependencia inicial ya se verificó en `origin/main` tras el PR #18; volver a comprobar diff/CI antes de integrar. No hay merge a `main` ni force-push por el agente.
2. El propietario revisa y hace merge a `main`. Esta revisión no autoriza despliegue ni migraciones remotas.
3. Siguen pendientes monitor sincronizado, recuperación automática de estados ambiguos, consulta algo, ejecutor testnet recuperable y automatizaciones. El ledger/journal de resultados no se completa con estos fills.
4. Validación operativa, despliegue Railway aprobado, salida fija, backups y restauración siguen siendo puertas independientes. No se conectaron claves ni cuentas reales.
