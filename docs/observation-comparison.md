# Comparación durable de observaciones Binance

Es una base de reconciliación **solo lectura**, no reconciliación del exchange terminada. No abre conexiones nuevas ni usa eventos privados, y nunca habilita trading. Cada reporte declara `mode=rest_observation_comparison`, `continuity=not_established` y `executionReady=false`.

## Qué se compara

- Saldos por activo, sin sumar monedas distintas.
- Posiciones por símbolo y BOTH/LONG/SHORT, conservando cantidad firmada.
- Órdenes normales por símbolo/ID y algo por símbolo/ID en familias separadas; IDs int64 como texto, no float.
- Instrumentos y filtros normalizados del alcance USDT perpetuo.

Se validan los datos normalizados completos de ambas lecturas antes de compararlos. Los importes se comparan con decimales exactos: `1.0` y `1.000` no crean actividad; una diferencia de `0.000000000000000001` sí se conserva. Se muestran los strings originales antes/después. Cambios solo de `updateTime` se excluyen, al igual que campos del exchange que el adaptador no modela; no es un diff de todo el JSON upstream. Los nombres de filtros desconocidos se comparan como un conjunto ordenado, no como reglas de ejecución interpretadas.

Estados de registro:

- `observed`: aparece en la lectura nueva, no en la anterior. **No** significa orden recién creada ni propiedad de una estrategia.
- `not_observed`: ya no aparece en la consulta nueva. Sus valores posteriores son `null`, no ceros ni estados inventados. **No** demuestra fill, cancelación, expiración, cierre o protección perdida/confirmada.
- `changed`: difieren campos observados. El acumulado `executedQty` no es un ledger de fills ni permite deducir comisiones o PnL realizado.

La primera captura sin baseline genera `initial_observation`, sin diferencias inventadas. Una captura posterior genera `compared`; incluso con cero cambios no demuestra ausencia de actividad entre lecturas ni continuidad de eventos. Se informa si los intervalos se solapan. Intervalos disjuntos tampoco garantizan secuencia completa ni ausencia de desconexiones.

## Persistencia y concurrencia

La migración administrativa `0005_observation_comparisons.sql` añade un reporte nullable a la fila de la última lectura. No cambia migraciones anteriores, crea datos falsos ni modifica permisos de exchange. Las filas anteriores quedan sin comparación hasta la siguiente captura completa; la consulta del reporte devuelve 404 mientras falte.

Después de la red, el guardado:

1. Revalida sesión, propiedad y existencia de credencial.
2. Dentro de la transacción serializada del propietario, lee el baseline durable de **esa cuenta**, no uno del navegador ni uno obtenido antes de las peticiones externas.
3. Rechaza baselines corruptos y capturas con fecha anterior/igual a la existente.
4. Calcula y guarda snapshot, reporte y auditoría en una sola transacción. No mantiene locks mientras consulta Binance.

El reporte queda ligado al inicio/fin exactos y a `storedAt` de su snapshot. RLS forzada y FK compuesta se heredan de la tabla; retirar una clave no elimina evidencia histórica ni concede acceso ajeno. Un error de validación o persistencia no devuelve una comparación parcial, sobrescribe evidencia corrupta ni hace fallback a datos antiguos. Un error de confirmación puede ser ambiguo: recuperar explícitamente el estado, no inferir éxito.

No se establece una secuencia de eventos con relojes del motor. No escalar lectores sin cuota compartida, coordinación y reconciliación reales.

## Límites y privacidad

Se calcula el conteo completo de **registros** diferentes; el detalle guarda como máximo 200 registros, en orden determinista por familia/clave/campo. `totalChanges`, `detailLimit=200` y `truncated` hacen visible cualquier recorte. Si se supera el límite de tamaño de reporte (2 MiB), el guardado falla, no lo presenta como completo. Los snapshots siguen limitados a 8 MiB y a su alcance documentado.

Solo se conserva el último reporte, no una historia completa ni un checkpoint de reconciliación. Sus campos antes/después son datos financieros confidenciales en JSONB, sin envelope de credenciales: proteger base/backups y no registrar payloads en logs/APM. La auditoría conserva metadatos vacíos, no importes ni cambios. No hay endpoint que acepte un reporte o baseline del cliente.

## Recuperación e interfaz

**Configuración → Comprobar conexión → Ver cambios de la última lectura** llama a la API privada `POST /v1/connections/binance/comparison` por medio de la web autenticada. Solo admite `accountId` desde el navegador; el digest de sesión se deriva en el servidor.

La recuperación solo consulta PostgreSQL: no descifra credenciales, consume cuota Binance ni refresca la cuenta. Funciona con el lector externo desactivado, siempre que existan base/sesión y una cuenta seleccionable. Muestra **COMPARACIÓN GUARDADA · NO ESTADO ACTUAL**, fechas originales y advertencias sobre ausencias/continuidad. Cambiar cuenta, actualizar o fallar elimina el reporte anterior de pantalla. No hay polling ni carga automática.

Aplicar migración 0005 mediante el trabajo administrativo antes de actualizar todos los motores. No mezclar escritores antiguos/nuevos; comprobar readiness del manifiesto. No se ha aplicado esta migración a Railway ni se ha validado con cuentas reales por el agente.

## Qué falta para reconciliar

Transporte/eventos privados autorizados con permisos mínimos, buffering durante snapshots, deduplicación durable de fills, ordenación/antigüedad por evento, consulta de estados ambiguos, recuperación tras desconexión y pruebas de pérdida/reordenación. No relajar permisos de clave ni introducir `listenKey`/escrituras de exchange para saltar las puertas actuales. Ejecución y recuperación de órdenes siguen bloqueadas.
