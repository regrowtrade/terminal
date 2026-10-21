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

`POST /v1/plans/intents/observations` y su proxy reciben los mismos IDs. Recuperan explícitamente **las últimas 20 observaciones**, más recientes primero, no un historial completo. Recalculan cada evaluación al leerla. Funcionan con lectura desactivada y tras retirar credenciales, pero exigen sesión/propiedad. Intención propia sin observaciones: `[]`; ajena/inexistente: 404. No hay UI nueva ni consultas automáticas.

Aplicar explícitamente `0011_intent_observations.sql` y reaplicar `database/runtime-grants.sql` antes del motor nuevo. `binance_intent_observations` es append-only para runtime, con RLS forzada y FK compuesta a intención/plan/cuenta/propietario. Sin claves, pero con evidencia financiera confidencial que requiere controles de acceso/backups. La migración no rellena historia ni cambia estados existentes.

No aplicado a Railway ni validado con claves reales. Tests sintéticos y PostgreSQL desechable; nunca exchanges.

## Pendientes

Consulta algo/condicional, recuperación REST de trades perdidos, estado reconciliado tras reinicios/desconexiones, relación demostrable con envíos durables, ejecutor testnet recuperable, protección confirmada y automatizaciones. Este PR no sustituye esos controles.
