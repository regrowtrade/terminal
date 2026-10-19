# Eventos privados de Binance (flujo de usuario, solo lectura)

Esta entrega añade un **flujo de datos de usuario de solo lectura** y un **journal durable** de eventos privados, sin reconciliación terminada. Solo recibe `ORDER_TRADE_UPDATE` y `ACCOUNT_UPDATE`; nunca envía, cancela, modifica órdenes ni infiere fills/continuidad/ejecución. Es la materia prima para el siguiente paso de reconciliación.

## Transporte

El adaptador añade **únicamente** tres llamadas no-GET, todas al mismo destino fijo:

- `POST /fapi/v1/listenKey` — crea un `listenKey` de solo lectura (peso 1).
- `PUT /fapi/v1/listenKey` — keepalive antes de que expire (≈60 min; aquí cada 30 min).
- `DELETE /fapi/v1/listenKey` — cierre al salir, best-effort.

La firma es la misma HMAC SHA-256 con `recvWindow=5000`; no hay cuerpo y la respuesta se limita. El WebSocket conecta a `wss://fstream.binance.com/ws/<listenKey>`, sin proxies elegidos por variables de entorno y sin redirects. Se reutiliza la comprobación de permisos/IP existente (`Verify`) antes de abrir el flujo, y el cooldown compartido ante 418/429.

El flujo se activa con `BINANCE_READ_ENABLED=true` (misma puerta que las lecturas REST, tras `BINANCE_EGRESS_CONFIRMED=true`). El agente no lo habilita ni valida claves reales.

## Eventos y validación

- `ORDER_TRADE_UPDATE`: símbolo/ID/`clientOrderId`, lado y `positionSide`, tipo, cantidades y precios como strings decimales, `executionType`/`orderStatus`, `tradeId` y `realizedProfit`. Se aceptan estados terminales (FILLED/CANCELED/EXPIRED/REJECTED) porque son transiciones observadas, no deducciones.
- `ACCOUNT_UPDATE`: motivo (ORDER/FUND), saldos por activo y posiciones por símbolo/lado, con decimales firmados.
- Tipos desconocidos (MARGIN_CALL, etc.) se ignoran sin tratarlos como operaciones. Un mensaje reconocido pero malformado **falla cerrado**: no se persiste ni se presenta un parcial. Las variantes de campos no documentadas deben validarse al activar una cuenta real, igual que el resto del adaptador.

Los tiempos `E`/`T` se conservan como enteros milisegundo; `T` puede ser menor o igual que `E` (el evento se emite después de la transacción), por lo que no se impone un orden falso entre ambos.

## Deduplicación, reconexión y antigüedad

- `DedupKey` es un SHA-256 de la identidad estable del evento (tipo, `E`/`T`, símbolo, IDs, `executionType`, `tradeId` para órdenes; tipo, `E`/`T`, motivo para cuenta). La clave primaria `(account_id, dedup_key)` hace idempotente la ingesta: una redelivery at-least-once no duplica.
- La reconexión usa backoff acotado (1 s → 30 s), vuelve a pedir `listenKey` y refresca el keepalive. `listenKeyExpired` reconecta de inmediato. Una clave revocada/permiso inválido detiene el flujo sin reintentar.
- Un deadline de lectura acotado detecta conexiones muertas en silencio; un fallo de base/red no detiene el motor.
- **Binance no reenvía eventos perdidos durante una desconexión.** El journal no es continuo ni total: la reconciliación con snapshots REST (siguiente entrega) cubre esos huecos. Un evento ausente nunca demuestra fill/cancelación/cierre.

## Persistencia

La migración administrativa `0006_user_events.sql` crea `binance_user_events`: journal append-only con FK compuesta a la cuenta, RLS forzada, tamaño acotado (64 KiB por evento) y columna `dedup_key` de 64 caracteres. El runtime solo recibe `SELECT`/`INSERT` (actualizar `runtime-grants.sql`); no puede borrar ni sobrescribir. No se escribe auditoría por evento (sería ruido); el propio journal es el registro y los payloads son confidenciales.

El monitor del motor enumera las cuentas del propietario bajo la identidad personal (sin sesión de navegador), descifra cada credencial con el keyring y guarda cada evento validado. Un fallo de guardado se registra y se continúa; los huecos se recuperan luego por snapshots.

## Recuperación

`POST /v1/connections/binance/events` (web autenticada) devuelve el journal acotado en orden ascendente por `eventTime`, con `limit=200` y `truncated`. Solo consulta PostgreSQL: no descifra claves ni contacta Binance, y funciona con el lector externo desactivado. Requiere sesión y propiedad. No hay UI todavía; queda para el monitor/reconciliación sincronizado.

## Despliegue y pendientes

Aplicar las migraciones `0006_user_events.sql`, `0007_stream_state.sql` y `0008_fills.sql` y los grants antes de habilitar el flujo; web y motor se actualizan juntos. Mientras haya mezcla de versiones, la lectura falla cerrada. No escalar sin cuota compartida y reconciliación reales.

Falta: consulta de estados ambiguos, recuperación tras reinicio con pérdida/reordenación de eventos, y sustituir el dashboard ilustrativo por métricas conciliadas. Ya implementado: buffering en memoria durante una lectura (`connections.SnapshotGate`), corte `capturedAtMs` por reloj del exchange para superponer solo eventos posteriores, y deduplicación durable de fills en `binance_fills` por `(orderId, tradeId)`. Ejecución y automatizaciones siguen bloqueadas.
