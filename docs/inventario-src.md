# Inventario preliminar de proyectos legacy — REG-6

Destino: `terminal`. Este inventario propone decisiones; no autoriza trading real,
copias de credenciales ni migraciones de datos. Los proyectos legacy se consultaron
sin ejecutar sus bots ni abrir archivos `.env`.

| Origen             | Función y dependencias principales                                                                      | Datos y entrada                                                                       | Propuesta                                                                                 | Motivo                                                                                                                                                        |
| ------------------ | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `automatic-tp`     | TP automático en Binance Futures; Python, `python-binance` y `python-dotenv`.                           | `script.py`, entrada interactiva y estado en memoria; credenciales por entorno.       | **Reescribir** el controlador; conservar las reglas como referencia.                      | Crea y cancela órdenes, usa `float` y precisión fija por símbolo. Terminal solo ofrece TP de simulación; exige recuperación y coordinación antes de ejecutar. |
| `hedge-management` | Bot Telegram para cobertura Binance; Python, `python-binance`, `python-telegram-bot` y `python-dotenv`. | `script.py`, comandos Telegram y SQLite para coberturas; secretos por entorno.        | **Reescribir** la automatización; contrastar las reglas puras con el planificador Go.     | El preview decimal ya existe; el bot gestiona órdenes reales y recupera estado tras reinicios. Faltan reconciliación y ejecución recuperable.                 |
| `rebuymaster`      | Órdenes y recompras Binance/Bybit; Go 1.23, Buffalo, Pop y clientes de exchange.                        | `cmd/rebuymaster`, rutas HTTP y PostgreSQL; configuración y credenciales por entorno. | **Reescribir** ejecución e integración; conservar reglas como referencia.                 | Terminal ya simula recompras y guarda planes; no tiene ejecutor ni Bybit. No trasladar rutas o clientes de órdenes sin controles.                             |
| `trading-journal`  | Journal Next.js 16, React, Drizzle, Neon, Tailwind y exportación.                                       | App Router y acciones de servidor; cuentas y operaciones en PostgreSQL/Neon.          | **Reescribir** persistencia y autorización; evaluar portar componentes y reglas aisladas. | Terminal aún no tiene journal persistido. El legacy acepta `userId` del cliente en acciones: no trasladar esa autoridad.                                      |

## Modelos de datos y paridad por verificar

- `automatic-tp`: el script conserva símbolo, precio y orden en memoria. No se
  observó persistencia en ese archivo; un reinicio no acredita recuperación.
- `hedge-management`: `active_hedges` en SQLite usa símbolo como clave, precios
  `REAL` y una lista JSON de órdenes. Una migración requiere identificar la
  cuenta, preservar precisión y verificar el estado real de cada orden.
- `rebuymaster`: `Order` y `Buyback` usan `float64`; el generador admite un
  incremento de volumen configurable. Comparar resultados con el planificador
  decimal antes de afirmar paridad o convertir registros existentes.
- `trading-journal`: cuentas, operaciones y perfiles usan IDs propios y `userId`
  textual. Las tablas de campañas, ejecuciones, ledger y notas de terminal no
  constituyen por sí solas una migración ni un journal funcional.

## Decisiones transversales y pendientes

- **Descartar como implementación de producción**, no como referencia: ejecución
  directa de los bots legacy, autenticación confiada al cliente y precisión de
  precios codificada por símbolo. No descartar datos de usuario sin aprobación.
- No copiar `.env`, tokens, claves ni bases locales; auditar exposición y rotar
  credenciales si procede antes de cualquier migración (REG-9 y REG-10).
- Definir el runtime y los límites de servicios en REG-7; esta tabla no decide
  definitivamente entre Python y Go. Revisar modelos de datos, pruebas y paridad
  funcional antes de seleccionar el módulo piloto.
- El detalle de funcionalidades ya presentes y sus límites está en
  [integration-map.md](integration-map.md). Los planes e intenciones de simulación
  están implementados con PostgreSQL; no equivalen a órdenes enviadas.
