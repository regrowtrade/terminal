# Entregas y criterios de avance

## 0.1 — Base y planificación

- [x] Interfaz privada responsive, navegación y estados honestos de simulación.
- [x] Planificador decimal de entrada limpia, recompras y desmonte de cobertura.
- [x] Presupuesto en dinero/porcentaje y referencia capturada.
- [x] Pruebas de reglas, seguridad, componentes y API; pruebas de navegador incluidas.
- [x] Contrato OpenAPI, diseño PostgreSQL y plantilla de imagen del motor.
- [ ] Persistencia efectiva y pruebas de tenant isolation en PostgreSQL.
- [ ] Conexiones a exchanges o ejecución real (deliberadamente ausentes).

## 0.2 — Datos reales, solo lectura

Infraestructura, sesiones y credenciales/lectura fueron revisadas en los PRs #1–#3, destinados a Railway Hobby, uso personal y Binance Futures USDT, y ya están consolidadas en `main` mediante el PR #4. El PR #5, también integrado en `main`, amplía la lectura con órdenes/filtros y persistencia de la última captura (ver [control de versiones](version-control.md) y [mapa de integración de los cuatro proyectos](integration-map.md)). No es todavía una entrega 0.2 completa ni un despliegue validado.

- [x] Definición Railway versionada, imágenes web/motor sin root y comunicación privada.
- [x] Runner administrativo de migraciones, checksums, bloqueo y comprobación de rol/esquema en el motor.
- [x] Pruebas PostgreSQL/RLS y builds de contenedor definidos como puertas de CI.
- [ ] Despliegue aprobado y comprobado en Railway; backups/restauración verificados.
- [ ] Resolver IP de salida fija antes de activar claves de trading (Hobby no incluye esta función).
- [x] Identidad personal persistida, sesiones revocables y auditoría de sesiones implementadas y revisadas en el PR #2, consolidadas en `main` con el PR #4 (despliegue pendiente).
- [x] En el PR #7 (`feat/connected-account-views`), actualizado con `main` tras los PRs #5/#6: Posiciones de cuenta reutiliza lecturas manuales/guardadas y comparaciones autenticadas sin fixtures ni cargas automáticas; filtros locales y paginación explícita. Ejemplos separados y opt-in. Revisión/integración pendientes; no acredita un monitor reconciliado ni validación con claves reales.
- [ ] Persistencia y consumo de planes propios; métricas de cuenta/ledger conciliados en dashboard/journal.
- [x] En el PR #3, consolidado en `main` con el PR #4: envelopes cifrados con rotación administrativa y RLS, control de permisos/IP y panel manual de saldos/posiciones Binance USDT. Desactivado por defecto, sin claves reales ni despliegue verificado; no equivale a sincronización continua.
- [x] En el PR #5 (`feat/binance-market-orders`), integrado en `main`: catálogo/filtros de perpetuos USDT y listas completas de órdenes abiertas normales/algo integrados en la lectura manual. Validación operativa pendiente; no confirma protección ni reconcilia fills.
- [x] En la misma rama: última lectura completa por cuenta persistida con RLS, protección contra sobrescritura fuera de orden, revalidación de sesión al guardar y recuperación histórica explícita sin contactar al exchange. Migración `0004_read_snapshots.sql`; no aplicada a Railway por el agente.
- [x] En el PR #6 (`feat/binance-observation-comparison`), integrado por el propietario en `main`: comparación decimal durable entre lecturas completas, baseline obtenido dentro de la transacción y reporte acotado guardado junto al snapshot; recuperación manual autenticada. Ausencia no implica fill/cancelación/cierre y la continuidad sigue sin establecerse. Migración 0005; no aplicada por el agente.
- [x] En `feat/binance-user-data-stream`: flujo de usuario de solo lectura (listenKey POST/PUT/DELETE + WebSocket fijo), validación de `ORDER_TRADE_UPDATE`/`ACCOUNT_UPDATE`, deduplicación durable, reconexión con backoff y keepalive. Journal append-only `binance_user_events` con RLS y recuperación acotada `POST /v1/connections/binance/events`. No reconcilia fills ni establece continuidad; Binance no reenvía eventos perdidos. Migración `0006_user_events.sql`; no aplicada por el agente. Ver [eventos privados](private-events.md).
- [x] En `feat/reconciliation-foundation`: continuidad durable del flujo (`binance_stream_state`, hueco marcado al desconectar y reiniciado por cada lectura REST) y reporte de reconciliación `POST /v1/connections/binance/reconcile` que superpone eventos deduplicados sobre la última lectura, con fills `TRADE` deduplicados por `(orderId, tradeId)`, acotados y `continuity=established|gapped`. Solo PostgreSQL, sin claves ni exchange; nunca `executionReady`. Migración `0007_stream_state.sql`; no aplicada por el agente. Ver [reconciliación](reconciliation.md).
- [x] En `feat/reconciliation-foundation`: buffering en memoria de eventos durante una lectura REST (`connections.SnapshotGate`): los eventos que llegan en vuelo se retienen y se vuelcan al journal al confirmar la lectura, sin pérdida. Primitiva de proceso; la durabilidad sigue en el journal.
- [ ] Monitor de cuenta sincronizado (UI) y cierre de la ventana por reloj del exchange: ordenación/antigüedad por evento frente al reloj del exchange (watermark de corte), consulta de estados ambiguos y deduplicación durable de fills con tradeId como clave primaria. La comparación del PR #6, las vistas REST, el journal y el reporte de reconciliación no sustituyen a un estado reconciliado con el exchange.

1. Integrar identidad y sesiones revocables; no pasar el userId desde el cliente como autoridad.
2. Conectar PostgreSQL con migraciones versionadas y rol limitado; runner y readiness implementados, pendientes despliegue y consumo por los módulos.
3. Implementar almacenamiento cifrado de credenciales, restricción de permisos e IP.
4. Adaptador Binance de instrumentos, cuentas, órdenes normales/algo y posiciones.
5. Snapshots REST + eventos privados (flujo de usuario de solo lectura) con deduplicación, antigüedad de datos y reconexión.
6. Reconciliar eventos privados con snapshots (continuidad + reporte de fills); cerrar la ventana lectura/eventos y eliminar fixtures de las vistas conectadas manteniendo el sandbox explícitamente separado.

Criterio: posiciones/órdenes coinciden con el exchange tras reinicio y desconexión, y un usuario no puede acceder a una cuenta ajena.

## 0.3 — Ejecución controlada en testnet

Intenciones persistidas, clientOrderId determinista, propiedad por estrategia, filtros de instrumentos, protección confirmada, reconciliación antes de reanudar y cierre residual verificable. Un solo ejecutor por cuenta/instrumento; ninguna convivencia activa con bots antiguos.

Criterio: pruebas de timeout con orden aceptada, duplicados, grilla parcialmente enviada, SL rechazado, fills parciales, eventos fuera de orden y cancelaciones manuales. Nunca mostrar ACK como fill ni EXPIRED como cierre confirmado.

## 0.4 — Automatizaciones y journal

TP por cantidad/precio medio, coordinación de recompras y cobertura, presupuesto actualizado por fills reales, política explícita al detectar hedge, ledger de comisiones/funding, campañas y anotaciones. Telegram llama al mismo backend autorizado.

Criterio: resultados conciliados sin duplicación y protección documentada ante caída del motor; una compensación parcial no se presenta como cierre total.

## 0.5 — Bybit y preparación comercial

Adaptador Bybit con sus propias capacidades (no traducción ciega de Binance), MFA, roles y límites por usuario, throttling compartido, backups verificados, monitorización, gestión de suscripciones y revisión contractual/legal.

Antes de producción con capital: evaluación gradual con tamaños pequeños, kill switches y alertas operativas. No garantizar resultados ni pérdida monetaria exacta.
