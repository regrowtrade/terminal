# Consulta puntual de órdenes algo (solo lectura)

Entrega `feat/binance-algo-order-lookup`, creada desde `main` tras verificar el PR #20 (`8002eef`) y sus checks correctos. Añade API y proxy, **todavía sin UI**. No envía, cancela, reintenta ni sigue automáticamente una orden hija. No cambia snapshots, continuidad, fills ni intenciones; siempre `executionReady=false`.

## Protocolo y autenticación

- Motor: `POST /v1/connections/binance/algo`, autenticado con token privado y sesión/propiedad PostgreSQL. Acepta únicamente `tokenHash`, `accountId`, `symbol` y exactamente uno de `algoId`/`clientAlgoId`.
- Proxy: `POST /api/connections/binance/algo`, con sesión del servidor, mismo origen, PostgreSQL y límite de consultas compartido. Deriva `tokenHash` desde la cookie; rechaza IDs normales, propietarios, selectores vacíos/nulos o desconocidos. No expone secretos ni errores del exchange/driver. Cache desactivada y redirects rechazados.
- `symbol` es el símbolo USDT **esperado**, no un selector enviado al exchange. Binance `GET /fapi/v1/algoOrder` recibe solo `algoId` **o** `clientAlgoId`, además del reloj/recvWindow/firma. Se comprueba el símbolo devuelto y el identificador elegido; no se reutiliza el protocolo de `/fapi/v1/order`.
- Verifica reloj, permisos de lectura y restricción de IP antes de consultar. Sigue rechazando claves con trading/retiros/transferencias habilitados. Timeout de consulta de 18 segundos y límites/cooldown compartidos con las demás lecturas; no hay reintentos ni fallback a consultas normales.

Referencia contrastada el 22 de octubre de 2026: [Query Algo Order](https://developers.binance.com/en/docs/catalog/core-trading-derivatives-trading-usd-s-m-futures/api/rest-api/trade#query-algo-order). La página devolvió HTTP 202 sin contenido en este entorno; se contrastaron los parámetros y campos con el SDK oficial de Binance, [método query_algo_order](https://github.com/binance/binance-connector-python/blob/6f3ed1ca6726cbf6b9bac2ce006d928a297cb3bf/clients/derivatives_trading_usds_futures/src/binance_sdk_derivatives_trading_usds_futures/rest_api/api/trade_api.py) y [modelo de respuesta](https://github.com/binance/binance-connector-python/blob/6f3ed1ca6726cbf6b9bac2ce006d928a297cb3bf/clients/derivatives_trading_usds_futures/src/binance_sdk_derivatives_trading_usds_futures/rest_api/models/query_algo_order_response.py). No se contactó ningún endpoint operativo de Binance.

## Evidencia y límites

La respuesta usa `family="algo"`, query con IDs algo y un objeto `order` independiente del de órdenes normales. Conserva int64 como strings y decimales exactos, sin floats. Solo admite `algoType=CONDITIONAL`, tipos STOP/TAKE_PROFIT y variantes MARKET/TRAILING con atributos completos y coherentes. Trailing requiere `activatePrice` positivo y `callbackRate` en `[0.1,10]`; una variante del exchange que no los devuelva falla cerrada, no inventa esos datos.

Estados admitidos: `NEW`, `CANCELED`, `TRIGGERED`, `FINISHED`, `REJECTED`, `EXPIRED`. Un estado desconocido falla cerrado. `FINISHED` **no equivale a FILLED**, cierre de posición, protección confirmada o pertenencia a una estrategia. No se vincula esta observación a una intención TP/SL ni se confirma su propiedad.

`actualOrderId`, `actualPrice`, `actualType`/`actualQty` opcionales y `triggerTime` son evidencia declarada por el exchange. El ID hijo puede estar vacío antes del disparo; una referencia no vacía exige ID válido y tiempo positivo. Sin hijo no se aceptan precio/cantidad ejecutada positivos ni tipo hijo. `NEW` no puede llevar hijo o disparo. La cantidad declarada debe ser no negativa y no exceder la cantidad solicitada salvo `closePosition`; una cantidad positiva exige precio positivo. Close-all permite cantidad cero sin deducirla de posiciones. La referencia hija **no se consulta automáticamente**, y estas cantidades no crean filas en el ledger de fills.

Solo HTTP 400 con código `-2013` se conserva como `outcome=unresolved`, `reason=exchange_not_found`, `order=null`. Otros códigos, errores, JSON incompleto, discrepancias de query o tipos no compatibles devuelven error sin evidencia parcial. La ausencia puede deberse al ID o a retención del exchange: el SDK advierte de órdenes canceladas/expiradas sin trades tras tres días, y de órdenes de más de 90 días. No prueba que nunca existieran, ni autoriza reenvío.

## Persistencia y operación

Tras I/O externo se revalidan sesión y existencia/propiedad de la credencial Binance production. La observación append-only y la auditoría `account.algo_order_observed` (solo `outcome`) se guardan en la misma transacción; error de guardado/auditoría no devuelve éxito ni resultado parcial.

Reutiliza `binance_order_observations` de la migración `0010`: su envelope JSON y restricciones admiten esta familia sin modificar migraciones históricas. RLS forzada, FK compuesta y permisos append-only existentes. La evidencia normal no cambia de contrato ni recibe campos algo. El helper de guardado compartido conserva revalidación, tiempos UTC a microsegundos y rollback para ambas familias.

Requiere las migraciones/grants existentes y los prerrequisitos operativos del lector. No se aplicó ninguna migración remota ni se desplegó. No hay API de listado/historia algo nueva, monitor sincronizado, recuperación automática ni ejecución testnet. El siguiente consumo UI deberá mostrar la familia y distinguir referencia hija, estado algo y estado de orden normal.

## Verificación

Durante esta entrega, Next.js `16.4.0` bloqueó el arranque del navegador por un aviso de seguridad confirmado con `npm audit`. Se aplica el parche **16.4.2** y su configuración ESLint alineada en un commit independiente; [detalle y límites](security.md#dependencias). No se desactivó el control ni se amplió la actualización a otras familias de dependencias.

- Transportes sintéticos verifican GET firmado, host fijo, parámetros exactos (sin symbol en Binance), IDs/decimales exactos, estados, close-all, rechazo de evidencia incompleta, ausencia acotada, permisos y cooldown; nunca llaman exchanges.
- Servicio: sesión/propiedad antes de descifrar, binding de cuenta/exchange/entorno, respuesta ligada a la query, guardado durable y ausencia de respuestas parciales.
- HTTP/proxy: autenticación, mismo origen, sesiones PostgreSQL, selectores estrictos, límites, errores saneados y separación de familias.
- PostgreSQL 15 local desechable: migraciones/RLS, round-trip exacto, auditoría, rollback, revocación/expiración y credencial retirada; observaciones normales y algo conviven sin modificar snapshot/continuidad/fills. Cluster detenido y eliminado tras las pruebas.

Resultados locales tras instalar Next.js **16.4.2**:

- **292 pruebas web** correctas con `npm run test --workspace @regrow/web -- --maxWorkers=1`. `npm test` y otra ejecución con dos procesos agotaron los cinco segundos de una prueba preexistente de paginación de Posiciones en este equipo; no se modificó esa prueba ni se aumentaron sus timeouts. Antes del parche, la suite completa también pasó con dos procesos. CI Linux debe confirmar el comando predeterminado.
- **26 pruebas de navegador** correctas con `npm run test:e2e`, escritorio y móvil, incluidos el rechazo sin sesión y el bloqueo algo con sesiones locales. El intento anterior con 16.4.0 no pudo arrancar por el aviso de seguridad; no se ignoró ese fallo.
- `npm run lint`, `npm run typecheck`, `npm run build`, `go test ./...`, `go vet ./...` y `npm audit --omit=dev`: correctos; audit de runtime con cero vulnerabilidades.
- Integración PostgreSQL con `TEST_DATABASE_URL` efímera: `go test -count=1 -v ./internal/database` correcto, incluido `algo_observations`. `npm run test:standalone` con otro cluster desechable validó el build de producción 16.4.2: login, preview privado, revocación individual/global y rechazo de cookies copiadas. Ambos clusters se detuvieron y eliminaron.
- **133 archivos** cubiertos por Prettier y **94 archivos Go** de `cmd/internal` pasan sus formatters normalizando LF en memoria. Se conservan los CRLF preexistentes de Windows; CI Linux es autoritativo. El YAML OpenAPI se parseó sin errores ni referencias locales sin resolver; no sustituye un validador integral del contrato.

No acredita operación con claves reales, compatibilidad de cada variante de respuesta del exchange ni despliegue. CI remoto y revisión del propietario son puertas independientes.
