# Binance Futures USDT: primera lectura segura

Esta entrega implementa credenciales cifradas y consultas REST manuales de saldos/posiciones, órdenes abiertas normales/condicionales y catálogo/filtros de perpetuos USDT. **Desactivada por defecto, sin despliegue ni validación con una cuenta real realizados por el agente.** No contiene envío/cancelación de órdenes, transferencias, retiros, reconciliación ni recuperación de ejecución. El flujo de usuario de solo lectura (listenKey/WebSocket) y su journal durable se documentan en [eventos privados](private-events.md); no completan todavía la versión 0.2.

## Antes de habilitarla

1. Integrar primero las entregas aprobadas en `main`; aplicar las migraciones hasta `0007_stream_state.sql` con el migrador administrativo y reaplicar `database/runtime-grants.sql`. No modificar migraciones anteriores ni aplicarlas al arrancar HTTP. El motor solo recibe SELECT sobre credenciales, SELECT/INSERT/UPDATE sobre la última lectura y su comparación, SELECT/INSERT sobre el journal de eventos, y SELECT/INSERT/UPDATE sobre el estado de continuidad del flujo; la herramienta administrativa realiza altas/rotaciones/bajas de credenciales.
2. Confirmar acceso permitido a Binance Futures en la región y una IP de salida fija aprobada. Railway Hobby no incluye Static Outbound IPs: resolverlo antes de introducir claves. `BINANCE_EGRESS_CONFIRMED=true` es una confirmación operativa del propietario, **no una prueba automática de que la IP sea fija o dedicada**. La consulta de permisos exige además restricción de IP activa y Binance debe aceptar la petición desde esa salida.
3. Crear una clave **HMAC**, con lectura y restricción de IP. Desactivar trading Futures, Spot/Margin, Margin, opciones/FIX/Portfolio, retiros y transferencias internas/universales. El adaptador rechaza los flags peligrosos y respuestas incompletas, no modifica permisos. Si Binance requiere un permiso combinado de trading para leer una cuenta concreta, esta fase rechazará la conexión: investigar/revisar el diseño, **no activar trading para saltarse el guard**. No admite claves RSA/Ed25519, testnet, Portfolio Margin ni exposiciones no USDT en esta entrega.
4. No introducir secretos en el chat, argumentos, repositorio, tickets o logs. Desactivar captura de variables, cuerpos, cabeceras y URLs firmadas en APM/proxies. No compartir `ENGINE_API_TOKEN` ni las credenciales PostgreSQL.

## Cifrado y secretos

Solo motor y consola administrativa reciben:

- `CREDENTIAL_ACTIVE_KEY_ID`: ID de versión no secreto, único para ese material.
- `CREDENTIAL_KEYS_JSON`: variable sellada con mapa `{ "<id>": "<KEK aleatoria de 32 bytes en base64>" }`. Generar con CSPRNG en una consola segura/gestor de secretos, nunca una contraseña de usuario. No reutilizar un ID con otros bytes. No hay una clave por defecto.
- `BINANCE_READ_ENABLED=true` y `BINANCE_EGRESS_CONFIRMED=true` únicamente después de aprobar los prerrequisitos. Sin habilitación no hay peticiones a Binance desde la UI.

El keyring permite hasta ocho versiones para una transición controlada. Cada registro usa una DEK aleatoria nueva de 256 bits; AES-256-GCM cifra las credenciales y envuelve la DEK con la KEK activa. Los datos autenticados ligan versión, propietario, cuenta, exchange, entorno y propósito (datos/wrapping); intercambiar filas o modificar ciphertext falla. En PostgreSQL solo hay un envelope cifrado, nonces y el ID de versión. **La KEK está en el gestor de secretos, no en un KMS administrado**; separar acceso a base y keyring y respaldarlos de forma independiente. El motor necesita descifrar en memoria durante la consulta; Go no ofrece garantía de borrado completo de strings/heap.

La web, el navegador y el migrador no reciben KEKs ni credenciales de exchange. No hay endpoint/formulario que acepte API keys. Los backups de base siguen siendo confidenciales; no destruir claves antiguas mientras existan registros/backups que las requieran.

## Alta administrativa (no ejecutar aún en Railway Hobby sin salida fija)

Desde `apps/engine`, con `MIGRATION_DATABASE_URL` administrativa y keyring inyectados de forma segura **solo en esa consola**, ejecutar:

```powershell
go run ./cmd/credentials -action provision -name "Binance personal · Lectura"
```

En la imagen está disponible `/usr/local/bin/credentials` con los mismos flags. Requiere TTY interactivo: solicita API key y secret sin eco. No acepta secretos por argumentos/pipes/archivos. Verifica permisos y acceso efectivo a todos los componentes de la lectura mediante GET antes de guardar; no imprime el snapshot. Emite únicamente el UUID de cuenta. Si falla o el resultado es ambiguo, revisar el estado antes de repetir; no relaja permisos ni cambia nada en Binance.

Después, el propietario autenticado puede abrir **Posiciones → Cuenta Binance · Solo lectura** o **Configuración**, y pulsar **Comprobar conexión → Consultar cuenta y órdenes**. Comprobar conexión solo lista metadatos locales; no contacta a Binance. Cada snapshot vuelve a comprobar permisos. Las sesiones se validan junto con propiedad/RLS al leer el envelope; nunca se toma un ownerId del navegador. Los [ejemplos quedan separados de la cuenta](account-views.md), sin fallback ni cargas automáticas.

## Rotación y retirada

1. Añadir la nueva KEK con **otro ID**, conservar la anterior y seleccionar la nueva como activa en consola/motor.
2. Por cada cuenta, ejecutar:

```powershell
go run ./cmd/credentials -action rotate -account <uuid-de-cuenta>
```

Esto rota el **cifrado local**, no la API key de Binance. Es una transacción auditada, con DEK/nonces nuevos y binding verificado; no hace peticiones a Binance. Verificar que todas las cuentas se descifran con la nueva versión antes de retirar las anteriores; conservar material requerido para backups históricos y probar restauración.

Para retirar la copia local, incluso si se perdió la KEK:

```powershell
go run ./cmd/credentials -action delete -account <uuid-de-cuenta>
```

La cuenta histórica permanece y el envelope se elimina transaccionalmente. **Revocar también la API key en Binance**: eliminar la copia local no la revoca, no termina consultas ya iniciadas, no cancela órdenes ni cierra posiciones. Para cambiar una API key comprometida, revocarla primero en Binance, provisionar otra de mínimos permisos y retirar la copia anterior. Rotar solo la KEK no arregla una clave de exchange filtrada.

## Semántica de los datos y límites

- Solo GET a destinos fijos HTTPS: `fapi.binance.com/fapi/v1/time`, `/fapi/v1/exchangeInfo`, `/fapi/v3/account`, `/fapi/v3/positionRisk`, `/fapi/v1/openOrders`, `/fapi/v1/openAlgoOrders` y `api.binance.com/sapi/v1/account/apiRestrictions`. Tiempo/catálogo son públicos y nunca reciben API key ni firma. No URLs configurables desde clientes, redirects, proxies elegidos por variables de entorno ni reintentos a nivel aplicación.
- Firma HMAC SHA-256, `recvWindow=5000`, reloj obtenido de Binance con RTT acotado, timeout de cuatro segundos por petición y 18 segundos por lectura completa. Cuerpos limitados a 2 MiB por endpoint privado/tiempo y 8 MiB para catálogo; la API web también limita la respuesta del motor a 8 MiB. Ante 418/429 se impone cooldown y se respeta `Retry-After` antes de nuevas consultas externas. Límite local adicional: un inicio cada diez segundos por proceso y ninguna lectura simultánea en ese cliente. Órdenes normales y algo se consultan sin símbolo para no omitir órdenes de instrumentos sin posición: cada una cuesta peso 40 según la documentación consultada. No escalar sin cuota compartida.
- Cantidades, precios y PnL se validan y transmiten como **strings decimales**. Se distinguen BOTH/LONG/SHORT; no se suma dinero de activos diferentes ni se infiere modo de margen/leverage a partir de campos ausentes. Posiciones no USDT, datos nulos, campos monetarios incompletos o valores inválidos provocan rechazo, sin mostrar un parcial como completo.
- La respuesta indica `mode=read_only`, `consistency=rest_non_atomic`, `executionReady=false`, `startedAt` y `capturedAt`. Son siete consultas consecutivas, no un snapshot atómico ni un stream actualizado. La UI muestra inicio/fin y tiempo transcurrido desde la recepción (reloj monotónico), marca antigüedad a los 30 s y elimina el resultado anterior al actualizar/fallar. No hay polling automático. Si falla cualquier componente o falta un instrumento referenciado por posiciones/órdenes, no se devuelve un parcial.
- Órdenes normales y algo permanecen en listas separadas. IDs int64 se decodifican sin float y se envían al navegador como strings. Se validan duplicados por símbolo/ID, BUY/SELL y BOTH/LONG/SHORT, cantidades, precios, estados abiertos, indicadores reduce-only/cierre total y condiciones de trailing. `executedQty` informa el acumulado devuelto por Binance, no un ledger de fills reconciliado. `closePosition=true` se muestra sin inventar una cantidad fija. Una orden condicional no acredita cobertura suficiente ni garantiza ejecución/pérdida máxima.
- El catálogo público se limita explícitamente a perpetuos con cotización/margen USDT; una exposición privada fuera de ese alcance provoca rechazo, no filtrado silencioso. Se muestran PRICE_FILTER, LOT_SIZE, MARKET_LOT_SIZE, MIN_NOTIONAL, PERCENT_PRICE y límites de órdenes normales/algo. No se infiere tickSize/stepSize desde las precisiones; se preservan ceros y decimales. Los nombres de filtros desconocidos se muestran como no interpretados, nunca como validados. Son datos informativos: no se valida ni cuantiza una instrucción de trading.
- No se consultan fills ni comisiones/funding por REST. El flujo de usuario recibe eventos privados de solo lectura y los guarda en un journal durable, sin reconciliarlos ni convertirlos en un ledger de fills; ver [eventos privados](private-events.md). Un array de posiciones vacío **no** demuestra que no haya órdenes abiertas; incluso dos listas vacías solo describen consultas distintas, no una ausencia atómica ni un cierre confirmado. El precio de liquidación devuelto no es un stop ni una garantía de pérdida máxima.
- Posiciones reutiliza estas lecturas completas, con filtros locales por símbolo/lado y paginación explícita de 50 registros. Los ejemplos son opt-in y no se mezclan con la cuenta; el dashboard sigue ilustrativo y el planificador no consume estos datos automáticamente. No hay sincronización/reconciliación continua aún.

### Persistencia y recuperación

Cada lectura externa completa debe guardarse en PostgreSQL antes de devolver éxito. El guardado revalida sesión vigente y pertenencia de cuenta/credencial en una transacción corta, posterior a las peticiones externas; una revocación o retirada de credencial durante la consulta impide guardar/devolver el resultado. Si falla la consulta o el guardado, se responde con error sin devolver un parcial ni recuperar silenciosamente el resultado anterior. Un fallo de confirmación podría haber guardado una lectura: consultar explícitamente el estado, nunca inferir éxito.

`exchange_read_snapshots` conserva **solo la última captura por cuenta**, no un historial de fills. Una captura con `capturedAt` anterior o igual no sobrescribe una posterior; esto ordena observaciones según el reloj del motor, no eventos del exchange. No escalar sin resolver cuota compartida y sincronización/reconciliación. RLS forzada y FK compuesta ligan cuenta/propietario, tamaño máximo 8 MiB; fechas se normalizan a microsegundos para coincidir con PostgreSQL. La auditoría registra `account.read_saved` sin saldos, secretos ni payload. El runtime no puede borrar esta tabla.

**Mostrar última lectura guardada** llama a `/v1/connections/binance/latest` mediante la web autenticada. Revalida sesión y cuenta, pero no descifra claves ni llama a Binance; funciona aunque el lector externo esté desactivado. La UI muestra siempre **LECTURA GUARDADA · NO ACTUAL**, fecha de guardado y el intervalo original, aunque se acabe de descargar. No es protección verificada, un estado reconciliado ni una confirmación de que una posición siga abierta/cerrada. No se carga automáticamente al abrir la página.

Eliminar una credencial no elimina la evidencia financiera guardada ni la cuenta histórica. El endpoint de recuperación sigue exigiendo propietario y UUID de esa cuenta; el selector de conexión solo lista cuentas con credenciales provisionadas. Los datos financieros se almacenan como JSONB confidencial, no con el envelope de secretos: restringir acceso a PostgreSQL/backups y definir su eliminación administrativa según la política de retención. Nunca incluir estos payloads en logs/APM.

La última captura también guarda una **comparación durable de observaciones**, calculada frente al baseline de esa misma cuenta dentro de la transacción. **Ver cambios de la última lectura** recupera el reporte sin consultar Binance. Ausencias no se interpretan como cancelaciones/fills/cierres; formatos decimales equivalentes no crean actividad y los detalles se limitan explícitamente a 200 registros con conteo completo. No establece continuidad ni protección. Ver [alcance, límites y concurrencia](observation-comparison.md).

## Pruebas sin exchanges

Unitarias del adaptador con RoundTripper local: firma/hosts/métodos, separación pública/privada, permisos, cooldown, redirecciones, errores sin secretos, IDs grandes, filtros, trailing, duplicados y datos parciales. Flujo de usuario: firma/allowlist de listenKey, parseo y validación de eventos, deduplicación estable, reconexión por `listenKeyExpired`, eventos desconocidos ignorados y malformados rechazados, y parada ante clave insegura; ver [eventos privados](private-events.md). Vault: cifrado aleatorio, AAD, manipulación y keyring incorrecto. CI PostgreSQL: ciphertext, RLS, sesión revocada, runtime sin escritura de credenciales ni borrado del journal de eventos, rotación/baja, auditoría, deduplicación de eventos y recuperación durable tras reabrir el pool. Pruebas web/API y navegador con datos sintéticos para posiciones/órdenes/filtros, separación de fuentes, paginación, eliminación de resultados tras un fallo y recuperación etiquetada como histórica. La herramienta de alta **no se ejecuta con claves reales** en tests. Reconciliación y monitor sincronizado siguen pendientes antes de presentar datos conciliados o plantear testnet de ejecución.

### Compatibilidad de despliegue

La ampliación de la respuesta privada hace obligatorios `startedAt`, `capturedAtMs`, `openOrders`, `algoOrders` e `instruments` y añade los endpoints históricos `latest`, `comparison`, `events` y `reconcile`. Aplicar migraciones hasta `0007_stream_state.sql` y los grants antes de actualizar todos los motores; su readiness rechaza un historial de esquema incompleto. Web y motor usan schemas estrictos y deben actualizarse juntos; durante una mezcla de versiones la lectura falla de forma cerrada (503), no degrada a una vista parcial. No mezclar escritores antiguos/nuevos. No se han probado claves reales; compatibilidad efectiva de permisos/región y nuevas variantes de respuesta deben validarse antes de habilitarla.

Referencias oficiales consultadas: [permisos de clave](https://developers.binance.com/docs/wallet/account/api-key-permission), [cuenta V3](https://developers.binance.com/docs/derivatives/usds-margined-futures/account/rest-api/Account-Information-V3), [posiciones V3](https://developers.binance.com/docs/derivatives/usds-margined-futures/trade/rest-api/Position-Information-V3), [órdenes abiertas](https://developers.binance.com/docs/derivatives/usds-margined-futures/trade/rest-api/Current-All-Open-Orders), [órdenes algo abiertas](https://developers.binance.com/docs/derivatives/usds-margined-futures/trade/rest-api/Current-All-Algo-Open-Orders), [instrumentos](https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/Exchange-Information) y modelos del [conector oficial](https://github.com/binance/binance-connector-js/tree/master/clients/derivatives-trading-usds-futures/src/rest-api). Revalidar compatibilidad antes de habilitar una cuenta real; las pruebas offline no certifican disponibilidad regional ni permisos efectivos del proveedor.
