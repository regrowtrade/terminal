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
- Corte por reloj del exchange: la lectura guarda `capturedAtMs` (tiempo del exchange en el instante de `capturedAt`) y el reporte superpone solo los eventos con `event_time` estrictamente posterior; los anteriores ya están reflejados en la lectura y no se cuentan dos veces.
- Conteos de eventos (`totalEvents`, `orderUpdates`, `accountUpdates`), `lastEventTime` y si el journal consultado quedó truncado (`eventsTruncated`).
- `fills`: ejecuciones `TRADE` deduplicadas por `(orderId, tradeId)`, acotadas a 200 con `totalFills` completo y `fillsTruncated`. Precios/cantidades/comisiones son strings decimales originales.

Solo consulta PostgreSQL: no descifra claves ni contacta Binance, y funciona con el lector externo desactivado. Requiere sesión y propiedad. Una cuenta sin lectura guardada devuelve 404.

## Límites honestos

- El reporte consulta una ventana acotada del journal (200 eventos); `eventsTruncated` lo hace visible. No es un ledger de fills completo ni una historia de ejecuciones.
- `gapped` no intenta reconstruir lo perdido: la fuente de verdad sigue siendo la última lectura REST, no los eventos.
- No se infiere comisiones/funding ni PnL realizado a partir de un fill, ni cancelación/cierre a partir de una ausencia.

## Despliegue

Aplicar la migración `0007_stream_state.sql` y los grants antes de actualizar todos los motores; web y motor se actualizan juntos para el endpoint nuevo. Ver [eventos privados](private-events.md) y [roadmap](roadmap.md).

## Qué falta

- [x] Buffering en memoria de eventos durante una lectura: `connections.SnapshotGate` retiene los eventos que llegan mientras una snapshot REST está en vuelo y los vuelca al journal al confirmar la lectura, de modo que ninguno se pierde al cruzar `capturedAt`. Es una primitiva de proceso; la durabilidad sigue estando en el journal.
- [x] Watermark de corte por reloj del exchange: la snapshot guarda `capturedAtMs` (tiempo del exchange en `capturedAt`) y la reconciliación consulta solo eventos con `event_time` estrictamente posterior, de modo que lo ya reflejado no se superpone dos veces. El corte es una extrapolación lineal del reloj del exchange y no sustituye a una antigüedad exacta por evento.
- [ ] Consulta de estados ambiguos.
- [ ] Deduplicación durable de fills con tradeId como clave primaria.
- [ ] Monitor de cuenta sincronizado (UI).

La ejecución y las automatizaciones siguen bloqueadas.
