# Reconciliación (base de solo lectura)

Este paso añade la **base de reconciliación**: continuidad durable del flujo y un reporte que superpone los eventos deduplicados sobre la última lectura REST. Sigue siendo **solo lectura** y no produce un estado "en vivo" fundido, ni infiere fills/cierres a partir de ausencias, ni habilita ejecución.

## Continuidad

`binance_stream_state` guarda, por cuenta y bajo RLS:

- `connected`: si el flujo está conectado ahora.
- `last_event_time`: el mayor `eventTime` observado (nunca retrocede).
- `gapped_since_snapshot`: si hubo una desconexión **desde la última lectura REST**.

El monitor marca `gapped_since_snapshot=true` en cada desconexión y `connected` en cada reconexión. Guardar una lectura REST completa **reinicia** el hueco: la snapshot es el nuevo baseline autoritativo y solo las desconexiones posteriores vuelven a abrir un hueco. Un evento puede perder de detectarse si la desconexión ocurre justo mientras se guarda una snapshot; el orden se documenta y se corrige con la siguiente lectura.

`continuity=established` solo cuando `connected` y sin hueco; en cualquier otro caso es `gapped`. Una desconexión, aunque sea breve, deja `gapped`: Binance no reenvía eventos y no se puede asegurar que no se perdió ninguno.

## Reporte de reconciliación

`POST /v1/connections/binance/reconcile` (web autenticada) superpone el journal de eventos sobre la última lectura guardada y devuelve:

- `continuity`, `executionReady=false` y `snapshotCapturedAt` del baseline.
- Corte aproximado por reloj del exchange: la lectura guarda `capturedAtMs` (tiempo extrapolado del exchange en el instante de `capturedAt`) y el reporte superpone solo los eventos con `event_time` estrictamente posterior. REST no es atómico: este corte no demuestra que todos los eventos anteriores estén reflejados en cada campo de la lectura.
- Conteos de eventos (`totalEvents`, `orderUpdates`, `accountUpdates`), `lastEventTime` y si el journal consultado quedó truncado (`eventsTruncated`).
- `fills`: ejecuciones `TRADE` deduplicadas **durablemente** en `binance_fills` por la clave primaria `(accountId, orderId, tradeId)`, acotadas a 200 con `totalFills` completo y `fillsTruncated`. Precios/cantidades/comisiones son strings decimales originales. La extracción ocurre al ingestar el journal (`SaveUserEvents`), en la misma transacción.

Solo consulta PostgreSQL: no descifra claves ni contacta Binance, y funciona con el lector externo desactivado. Requiere sesión y propiedad. Una cuenta sin lectura guardada devuelve 404.

## Límites honestos

- El reporte consulta una ventana acotada del journal (200 eventos); `eventsTruncated` lo hace visible. No es un ledger de fills completo ni una historia de ejecuciones.
- `gapped` no intenta reconstruir lo perdido: la fuente de verdad sigue siendo la última lectura REST, no los eventos.
- El buffer es volátil: una caída del proceso o un fallo al guardar puede perder eventos pendientes. Se vacía en lotes de hasta 1000 eventos, compatibles con el límite transaccional del journal; no ofrece replay ni recuperación de fills perdidos.
- No se infiere comisiones/funding ni PnL realizado a partir de un fill, ni cancelación/cierre a partir de una ausencia.

## Despliegue

Aplicar las migraciones hasta `0009_legacy_snapshot_archives.sql` y los grants antes de actualizar todos los motores; web y motor se actualizan juntos para el endpoint nuevo. Ver [eventos privados](private-events.md) y [roadmap](roadmap.md).

Las lecturas antiguas sin `capturedAtMs` siguen rechazándose como baseline: no se inventa un reloj del exchange a partir del reloj local. Su transición es explícita y preserva el registro original; ver el procedimiento siguiente. La migración `0008` crea fills y `0009` crea el archivo, pero ninguna mueve datos ni rellena historia automáticamente.

## Transición de snapshots anteriores al corte por exchange

Herramienta administrativa `snapshot-transition`, incluida en la imagen del motor. Nunca se ejecuta desde HTTP ni con el rol del motor; no necesita claves del exchange ni KEKs. Solo admite cuentas Binance production del propietario personal de esa base, sin recibir un `ownerId` del operador.

1. Hacer backup y comprobar restauración. Detener todos los escritores/monitores antiguos y las lecturas manuales; no mezclar versiones ni dejar buffers en vuelo durante la transición. La herramienta también toma el mismo bloqueo por propietario que los escritores actuales.
2. Aplicar explícitamente migraciones hasta `0009` y reaplicar `database/runtime-grants.sql`. Inyectar `MIGRATION_DATABASE_URL` solo en la consola administrativa, nunca como argumento ni variable del servicio HTTP.
3. Inspeccionar **una cuenta**. Desde `apps/engine`:

   ```powershell
   go run ./cmd/snapshot-transition --account $account
   ```

   Por defecto no modifica datos. Devuelve solo estado, fecha y huella SHA-256, nunca el snapshot. `legacy` significa que falta la propiedad `capturedAtMs`; `current` significa que la propiedad existe, **no** que su contenido haya sido validado. Una propiedad nula/inválida requiere investigación y no se retira como legacy. `missing` significa que no hay baseline activo, no que se haya comprobado la existencia de un archivo.

4. Revisar cuenta, fecha y huella; después confirmar explícitamente la misma observación:

   ```powershell
   go run ./cmd/snapshot-transition --account $account --apply --expected-sha256 "HUELLA_SHA256_REVISADA"
   ```

   En la imagen, sustituir `go run ./cmd/snapshot-transition` por `/usr/local/bin/snapshot-transition`. La huella cubre propietario, cuenta, timestamps, snapshot y comparación completa. Si cualquiera cambió desde la inspección, se rechaza sin cambios; volver a inspeccionar, no reintentar a ciegas.

5. Verificar `status=archived` y su `archiveId`. Una sola transacción copia el JSON original, comparación (incluido NULL), fechas y propietario a `exchange_read_snapshot_archives`; retira únicamente ese baseline activo; marca el hueco sin cambiar conexión ni watermark; registra `account.read_archived` sin payload financiero. Un error revierte todo. Repetir sobre una cuenta sin baseline devuelve `missing`, sin duplicar el archivo ni la auditoría. Una captura nueva nunca se retira con una huella anterior.
6. Comprobar los metadatos del archivo desde la consola administrativa privada y conservarlo junto a los backups. El runtime no tiene SELECT/INSERT/UPDATE/DELETE sobre el archivo, además de RLS forzada; **no hay recuperación del archivo desde la UI ni API**, ni restauración automática al baseline. No eliminar ni reescribir esa evidencia. Si el resultado del comando es ambiguo, inspeccionar el baseline y el archivo antes de reintentar.
7. Arrancar solo las versiones nuevas y solicitar una lectura manual completa. Hasta entonces `latest`/`comparison`/`reconcile` no tienen baseline y devuelven ausencia explícita (404). La primera lectura nueva produce `initial_observation`, sin comparar contra el archivo ni rellenar fills históricos. Solo guardar esa lectura reinicia el hueco; no implica ejecución habilitada ni una captura REST atómica.

Probado con PostgreSQL desechable: preservación exacta, comparación NULL, evidencia financiera corrupta sin reinterpretación, huellas obsoletas, confirmaciones concurrentes, rollback si falla la auditoría, permisos/RLS y lectura nueva. No se ha ejecutado sobre Railway ni validado operativamente con claves reales.

## Qué falta

- [x] Buffering en memoria de eventos durante una lectura: `connections.SnapshotGate` retiene los eventos que llegan mientras una snapshot REST está en vuelo y los vuelca al journal al confirmar la lectura, de modo que ninguno se pierde al cruzar `capturedAt`. Es una primitiva de proceso; la durabilidad sigue estando en el journal.
- [x] Watermark de corte por reloj del exchange: la snapshot guarda `capturedAtMs` y la reconciliación consulta solo eventos con `event_time` estrictamente posterior. El corte es una extrapolación lineal del reloj del exchange; no sustituye a una antigüedad exacta por evento ni a una captura REST atómica.
- [ ] Consulta de estados ambiguos.
- [x] Deduplicación durable de fills: `binance_fills` con clave primaria `(accountId, orderId, tradeId)`; la extracción de `TRADE` ocurre al ingestar el journal (`SaveUserEvents`), de modo que las redeliveries at-least-once nunca duplican un fill. No se rellena retroactivamente: los eventos journalizados antes de la migración `0008` no generan filas de fills.
- [ ] Monitor de cuenta sincronizado (UI).

La ejecución y las automatizaciones siguen bloqueadas.
