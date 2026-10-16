# Binance Futures USDT: primera lectura segura

Esta entrega implementa credenciales cifradas y consultas REST manuales de saldos/posiciones. **Desactivada por defecto, sin despliegue ni validación con una cuenta real realizados por el agente.** No contiene envío/cancelación de órdenes, transferencias, retiros, WebSocket/listenKey, reconciliación ni recuperación de ejecución. No completa todavía la versión 0.2.

## Antes de habilitarla

1. Revisar/mergear primero infraestructura y sesiones; aplicar `0003_exchange_credentials.sql` con el migrador administrativo y reaplicar `database/runtime-grants.sql`. El motor solo recibe SELECT sobre credenciales; la herramienta administrativa realiza altas/rotaciones/bajas.
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

En la imagen está disponible `/usr/local/bin/credentials` con los mismos flags. Requiere TTY interactivo: solicita API key y secret sin eco. No acepta secretos por argumentos/pipes/archivos. Verifica permisos y acceso efectivo a saldos/posiciones mediante GET antes de guardar; no imprime el snapshot. Emite únicamente el UUID de cuenta. Si falla o el resultado es ambiguo, revisar el estado antes de repetir; no relaja permisos ni cambia nada en Binance.

Después, el propietario autenticado puede abrir **Configuración → Comprobar conexión → Consultar saldos y posiciones**. Comprobar conexión solo lista metadatos locales; no contacta a Binance. Cada snapshot vuelve a comprobar permisos. Las sesiones se validan junto con propiedad/RLS al leer el envelope; nunca se toma un ownerId del navegador.

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

- Solo GET a destinos fijos HTTPS: `fapi.binance.com/fapi/v1/time`, `/fapi/v3/account`, `/fapi/v3/positionRisk` y `api.binance.com/sapi/v1/account/apiRestrictions`. No URLs configurables desde clientes, redirects, proxies elegidos por variables de entorno ni reintentos a nivel aplicación.
- Firma HMAC SHA-256, `recvWindow=5000`, reloj obtenido de Binance con RTT acotado, timeouts y cuerpos acotados. Ante 418/429 se impone cooldown y se respeta `Retry-After` antes de nuevas consultas externas. Límite local adicional: un snapshot cada diez segundos por proceso; no escalar sin cuota compartida.
- Cantidades, precios y PnL se validan y transmiten como **strings decimales**. Se distinguen BOTH/LONG/SHORT; no se suma dinero de activos diferentes ni se infiere modo de margen/leverage a partir de campos ausentes. Posiciones no USDT, datos nulos, campos monetarios incompletos o valores inválidos provocan rechazo, sin mostrar un parcial como completo.
- La respuesta indica `mode=read_only`, `consistency=rest_non_atomic`, `executionReady=false` y hora de captura. Son varias consultas consecutivas, no un snapshot atómico ni un stream actualizado. La UI muestra tiempo transcurrido desde la recepción (reloj monotónico), marca antigüedad a los 30 s y elimina el resultado anterior al actualizar/fallar. No hay polling automático.
- No se consultan órdenes normales/algo, fills, comisiones/funding ni eventos privados. Un array de posiciones vacío **no** demuestra que no haya órdenes abiertas. El precio de liquidación devuelto no es un stop ni una garantía de pérdida máxima.
- Las otras vistas mantienen fixtures identificados y el planificador no consume estos datos automáticamente. No hay persistencia de snapshots ni sincronización/reconciliación continua aún.

## Pruebas sin exchanges

Unitarias del adaptador con RoundTripper local: firma/hosts/métodos, permisos, cooldown, redirecciones, errores sin secretos y datos parciales. Vault: cifrado aleatorio, AAD, manipulación y keyring incorrecto. CI PostgreSQL: ciphertext, RLS, sesión revocada, rol runtime sin escritura, rotación/baja y auditoría. La herramienta de alta **no se ejecuta con claves reales** en tests. El siguiente bloque será instrumentos/filtros, órdenes normales/algo y sincronización privada antes de sustituir fixtures o plantear testnet de ejecución.

Referencias oficiales consultadas: [permisos de clave](https://developers.binance.com/docs/wallet/account/api-key-permission), [cuenta V3](https://developers.binance.com/docs/derivatives/usds-margined-futures/account/rest-api/Account-Information-V3), [posiciones V3](https://developers.binance.com/docs/derivatives/usds-margined-futures/trade/rest-api/Position-Information-V3). Revalidar compatibilidad antes de habilitar una cuenta real; las pruebas offline no certifican disponibilidad regional ni permisos efectivos del proveedor.
