# Investigación de intenciones (solo lectura)

Entrega `feat/intent-order-observations`, sobre `main` con el PR #15 integrado. Vincula una **observación puntual**, no una ejecución, a una intención guardada. No envía, cancela ni reintenta órdenes y no cambia `status` ni `exchangeOrderId`. No establece continuidad ni confirma SL/TP.

## API y fuente de verdad

- `POST /v1/plans/intents/observe` recibe exclusivamente `tokenHash`, `accountId`, `intentId`. Su proxy autenticado `/api/plans/intents/observe` acepta solo los dos IDs y deriva el digest en el servidor. Rechaza símbolos, selectores, atributos financieros y propietarios del navegador.
- PostgreSQL verifica propiedad, plan/campaña y correspondencia con la descomposición determinista del snapshot: solo `regular` + `ENTRY`/`REBUY`, simulación `clean`/`dca` y cuenta Binance production del lector actual. Testnet, cobertura y TP/SL condicionales no son compatibles en esta entrega.
- El lector consulta por símbolo y **clientOrderId almacenados**: GET firmado, reloj/permisos/IP verificados, límites/cooldown compartidos y sin reintentos. Sigue rechazando claves con trading habilitado.
- Después del acceso externo, otra transacción revalida sesión, credenciales existentes e intención, incluida la huella SHA-256 del snapshot. Una decisión distinta no se guarda como la original: 409 si sigue siendo válida; 422 si dejó de ser compatible/válida. No mantiene una transacción durante el acceso externo.
- Evaluación y respuesta normalizada se guardan junto a auditoría `intent.observed`; solo se devuelve éxito después del commit. Un fallo no devuelve evidencia parcial ni hace fallback a historia.

## Interpretación

| `assessment`       | Significado                                                                                                                      |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| `attributes_match` | Coinciden LIMIT, BUY/LONG o SELL/SHORT, `positionSide`, cantidad, precio y `reduceOnly=false`. **No prueba envío ni propiedad.** |
| `mismatch`         | Diferencias explícitas en `mismatches`; no asocia automáticamente ni modifica la intención.                                      |
| `unresolved`       | Binance devolvió `-2013`. No implica que nunca existió, ni cancelación/ejecución; no permite reenviar.                           |

Comparación decimal sin floats ni tolerancias: `1` y `1.000` son iguales; diferencias decimales no se ocultan. IDs int64 como strings. `positionSide=BOTH` es discrepancia: falta diseñar la política one-way. `FILLED` describe esa orden, no un cierre de posición ni un ledger de fills.

`executionReady=false` y `ownershipConfirmed=false` siempre. Binance permite reutilizar IDs de cliente después de órdenes terminales; coincidir en atributos no demuestra identidad histórica ni pertenencia a la estrategia. Las intenciones actuales son de simulación y no se han enviado.

## Historia y despliegue

`POST /v1/plans/intents/observations` y su proxy reciben los mismos IDs. Recuperan explícitamente **las últimas 20 observaciones**, más recientes primero, no un historial completo. Recalculan cada evaluación al leerla. Funcionan con lectura desactivada y tras retirar credenciales, pero exigen sesión/propiedad. Intención propia sin observaciones: `[]`; ajena/inexistente: 404. La entrega original no añadió UI ni consultas automáticas al exchange.

## Consumo en Planes guardados

La entrega `feat/intent-observations-ui`, integrada por el propietario en `main` mediante el PR #20, añade este flujo en Configuración:

1. La carga inicial consulta cuentas provisionadas y planes guardados en PostgreSQL, nunca el exchange. **Ver intenciones** carga únicamente las del plan elegido; no hay una cascada de peticiones para todos los planes.
2. **Investigar ENTRY/REBUY** abre un único panel con plan, intención, símbolo, lado, cantidad/precio e ID de cliente guardados, sin editarlos. Solo se ofrece para intenciones normales de planes `clean`/`dca` válidos; TP/SL condicionales y cobertura muestran su limitación, sin una alternativa de ejecución.
3. **Consultar y guardar observación** contacta Binance explícitamente y requiere que el lector esté habilitado. Solo envía `accountId` e `intentId`; el servidor deriva la sesión y relee los criterios. No envía atributos financieros ni selectores desde el navegador.
4. **Ver últimas 20 observaciones** consulta solo PostgreSQL, incluso con el lector externo desactivado. Siempre se marca **HISTORIAL GUARDADO · NO ACTUAL**, acotado y no completo. Una lista vacía no demuestra que no hubiera órdenes ni ejecuciones.

El resultado distingue atributos coincidentes, discrepancias con sus campos y ausencia no resuelta. Coincidir **no confirma propiedad/envío**; una ausencia nunca autoriza reenviar. Los IDs y cantidades/precios observados se muestran como strings exactos. `FILLED` se refiere a esa orden, no al cierre de una posición o protección confirmada. La intención sigue siendo de simulación, sin cambios de estado ni asociación automática.

La evidencia se valida con el schema estricto y se liga al plan/intención/ID de cliente/símbolo/lado/propósito/cantidad/precio seleccionados; la comparación decimal no usa floats ni tolerancias. Escalas equivalentes son iguales, pero diferencias de 18 decimales no se ocultan. `decimal.js`, ya usado por el schema de evidencia, se declara como dependencia directa de runtime en vez de depender transitivamente de herramientas de pruebas.

Cambiar cuenta o intención, actualizar planes/intenciones, cerrar el panel o salir de la vista aborta peticiones y descarta respuestas tardías. La carga de planes/intenciones también tiene timeout, cache desactivada y errores explícitos, sin presentar una consulta fallida como lista vacía. Consultar una observación retira el historial anterior antes del I/O, nunca lo reutiliza como fallback. Una respuesta ambigua puede haberse guardado: comprobar historia antes de reintentar. Abortar el navegador no garantiza cancelar el guardado en el servidor. No hay polling ni reintentos automáticos; solo una operación de evidencia por panel.

La API permite consultar historia tras retirar credenciales, pero la UI usa la lista de cuentas provisionadas: tras retirar la credencial, una carga nueva ya no permite descubrir esa cuenta. No se añade un navegador de cuentas archivadas en esta entrega. No se aplicó la migración `0011` ni ningún cambio remoto; no hay nuevas rutas ni cambios de contrato HTTP.

Verificación local de esta entrega: **248 pruebas web** y **26 pruebas de navegador** correctas, además de `npm run lint`, `npm run typecheck`, `npm run build`, `go test ./...` y `go vet ./...`. Las pruebas usan decisiones y evidencia sintéticas, sin exchange ni claves. Cubren coincidencia/discrepancia/ausencia, límites de historia, asociaciones incorrectas, errores, diferencia decimal inferior al float, permisos UI, carga explícita, cambio de cuenta y cancelación; el navegador verifica escritorio/móvil. Los tests Go se ejecutaron sin configurar PostgreSQL, por lo que no se repitieron las pruebas de integración de base de datos. No prueban operación con una cuenta real ni un monitor sincronizado; CI remoto pendiente de comprobar.

Formato: los **128 archivos versionados/nuevos** cubiertos por el comando del repositorio pasan Prettier con finales LF normalizados en memoria. No se reformatearon los CRLF preexistentes del checkout Windows; CI Linux sigue siendo la comprobación autoritativa. `npm audit --omit=dev`: sin vulnerabilidades detectadas.

Aplicar explícitamente `0011_intent_observations.sql` y reaplicar `database/runtime-grants.sql` antes del motor nuevo. `binance_intent_observations` es append-only para runtime, con RLS forzada y FK compuesta a intención/plan/cuenta/propietario. Sin claves, pero con evidencia financiera confidencial que requiere controles de acceso/backups. La migración no rellena historia ni cambia estados existentes.

No aplicado a Railway ni validado con claves reales. Tests sintéticos y PostgreSQL desechable; nunca exchanges.

## Pendientes

Consumo UI algo e investigación de intenciones condicionales, estado reconciliado tras reinicios/desconexiones, relación demostrable con envíos durables, ejecutor testnet recuperable, protección confirmada y automatizaciones. La [consulta algo por API/proxy](algo-order-lookup.md) se añade en una entrega posterior pendiente de revisión, sin asociación a intenciones. La consulta explícita de una orden condicional y la recuperación REST manual de fills se añadieron en entregas posteriores de cuenta; no son investigación de intenciones TP/SL ni recuperación automática. Esta UI no sustituye esos controles.
