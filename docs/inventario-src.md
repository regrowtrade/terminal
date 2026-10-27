# Inventario de proyectos legacy — REG-6

Destino: `terminal`. Este inventario propone decisiones; no autoriza trading real,
copias de credenciales, migraciones de datos ni llamadas a exchanges. Los proyectos
legacy se revisaron leyendo código y configuración, sin ejecutar bots, sin abrir
archivos `.env` y sin copiar valores de secretos.

## 1. Matriz por proyecto

| Proyecto           | Funcionalidades                                                                                                                  | Runtime y dependencias                                                                   | Entradas                                                           | Salidas                                                               | Persistencia y datos                                                                                  | Integraciones                                             | Pruebas propias                                                                          |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `automatic-tp`     | Calcula y coloca un TP LIMIT para una posición abierta en Binance Futures; cancela el TP previo al cambiar de precio de entrada. | Python; `python-binance`, `python-dotenv`. Lazo `while True` en `script.py`.             | Ticker y porcentaje de TP por consola; posición en Binance.        | Órdenes LIMIT de TP y cancelaciones en Binance; mensajes por consola. | Sin persistencia: símbolo, TP y `order_id` viven en memoria.                                          | Binance Futures REST (posiciones, órdenes, órdenes algo). | No se encontraron pruebas propias.                                                       |
| `hedge-management` | Bot de Telegram que abre, monitoriza y desmonta coberturas; reanuda monitores tras reinicio.                                     | Python; `python-binance`, `python-telegram-bot`, `python-dotenv`. Procfile con `worker`. | Comandos y conversación de Telegram; datos de posición en Binance. | Mensajes de Telegram; creación y cancelación de órdenes en Binance.   | SQLite `hedge_state.db`, tabla `active_hedges` (clave `symbol`, precios `REAL`, `order_ids` en JSON). | Binance Futures; Telegram Bot API.                        | No se encontraron pruebas propias.                                                       |
| `rebuymaster`      | Gestión de órdenes y recompras (buybacks) con simulación de niveles, margen y SL; envío de órdenes a Binance y Bybit.            | Go 1.23; Buffalo, Pop v6, `go-binance` v2, cliente Bybit v2, `lib/pq`.                   | Formularios y peticiones HTTP; credenciales por entorno.           | HTML de plantillas Plush; órdenes en Binance/Bybit.                   | PostgreSQL vía Pop; modelos `Order` y `Buyback` con `float64`.                                        | Binance Futures; Bybit V5; PostgreSQL.                    | `models_test.go`, `buybacks_test.go`, `actions_test.go`. No ejecutadas en esta revisión. |
| `trading-journal`  | Journal de operaciones por cuenta: CRUD de trades, estadísticas, calendario, estrategia y cobertura, perfil y exportación.       | Next.js 16, React 19, Drizzle ORM, Neon Serverless, Tailwind 4, `exceljs`.               | Formularios y Server Actions; `userId` enviado desde el cliente.   | UI, exportación CSV/Excel y cambios en la base.                       | Neon Postgres: `accounts`, `trades`, `user_profiles`; `userId` textual, sin FK a usuario.             | Neon Auth; Neon Postgres; exportación local.              | Ninguna; no hay runner de tests ni script `typecheck`.                                   |

## 2. Endpoints, Server Actions y vistas

### `rebuymaster` — rutas HTTP (`app/routes.go`)

| Método | Ruta                            | Uso                          |
| ------ | ------------------------------- | ---------------------------- |
| GET    | `/` y `/orders/`                | Listado de órdenes           |
| GET    | `/orders/new`                   | Formulario de alta           |
| GET    | `/orders/{id}/details`          | Detalle                      |
| GET    | `/orders/{id}/edit`             | Edición                      |
| PUT    | `/orders/{id}/update`           | Actualización                |
| POST   | `/orders/create`                | Creación                     |
| DELETE | `/orders/{id}/delete`           | Borrado                      |
| POST   | `/orders/{id}/set-order`        | Envío a Bybit                |
| POST   | `/orders/{id}/create-order`     | Envío a Binance              |
| GET    | `/buybacks/{id}`                | Listado de recompras         |
| POST   | `/buybacks/{id}/create`         | Generación de recompras      |
| DELETE | `/buybacks/{id}/delete`         | Borrado de recompras         |
| POST   | `/buybacks/{id}/binance/create` | Envío de recompras a Binance |
| POST   | `/buybacks/{id}/bybit/create`   | Envío de recompras a Bybit   |

Vistas (`app/templates`): `orders/` (index, show, new, edit, form, actions, order_overview, order_trade_info), `buybacks/index`, `application`, y parciales de paginación, cabecera, estado vacío y migas de pan.

### `trading-journal` — Server Actions (`app/actions.ts`)

`getAccounts`, `createAccount`, `getTrades`, `createTrade`, `updateTrade`, `deleteTrade`, `getStats`, `updateInitialBalance`, `getCalendarData`, `getStrategyStats`, `getHedgeStats`, `getUserProfile`, `updateUserProfile`.

Vistas: `app/page.tsx`, `app/login/page.tsx`, `app/(main)/dashboard/page.tsx`, `app/(main)/trades/page.tsx`, `app/(main)/profile/page.tsx`.

### `hedge-management` — comandos de Telegram (`script.py`, `bot_handlers.py`)

Comandos y entradas: `/start`, `/help`, `/status`, `/posiciones`, `/abortar` y `/cancel`; la conversación se inicia con `/cobertura` (símbolo, lado, porcentaje, precio de inicio, pérdida máxima y confirmación). No expone HTTP.

### `automatic-tp` — entradas de consola

No expone endpoints ni vistas. Su única entrada es la consola: ticker y porcentaje de TP.

## 3. Decisiones por módulo

| Proyecto           | Propuesta                                                                                 | Justificación                                                              | Duplicidades y brechas en `terminal`                                                                                                                                                   |
| ------------------ | ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `automatic-tp`     | **Reescribir** el controlador; conservar sus reglas como referencia.                      | Crea y cancela órdenes reales con `float` y precisión fija por símbolo.    | Duplicidad: el TP estático ya existe en el planificador. Brecha: falta controlador, confirmación de órdenes y recuperación.                                                            |
| `hedge-management` | **Reescribir** la automatización; contrastar sus reglas puras con el planificador Go.     | Gestiona órdenes reales y recupera estado tras reinicio desde SQLite.      | Duplicidad: la cobertura en simulación y la exposición ya existen en Go. Brecha: reconciliación, ejecución recuperable y persistencia de campañas.                                     |
| `rebuymaster`      | **Reescribir** ejecución e integración; conservar reglas como referencia.                 | Envía órdenes a Binance y Bybit sin los controles de la terminal.          | Duplicidad: la secuencia de recompras y los planes persistidos ya existen. Brechas: ejecutor, Bybit y gestión de grillas.                                                              |
| `trading-journal`  | **Reescribir** persistencia y autorización; evaluar portar componentes y reglas aisladas. | Acepta `userId` del cliente en Server Actions: no trasladar esa autoridad. | Duplicidad: existen una ruta `/journal` y tablas de campañas, ejecuciones y ledger en `terminal`, pero sin journal funcional. Brechas: persistencia, importación de fills y migración. |

**Descartar como implementación de producción** (no como referencia): ejecución directa de los bots legacy, autenticación confiada al cliente y precisión de precios codificada por símbolo. No se descartan datos de usuario sin aprobación.

## 4. Riesgos de secretos

- Los cuatro proyectos leen credenciales desde el entorno o `.env`. Se registran solo nombres de archivo y su tipo; no se incluyen valores ni nombres de variables con su contenido.
- No copiar `.env`, tokens, claves ni bases locales (`hedge_state.db`). Auditar exposición y rotar credenciales antes de cualquier migración (REG-9 y REG-10).

## 5. Piloto propuesto (pendiente de validación del propietario)

**Propuesta:** `trading-journal`, en modo lectura: consulta de cuentas, operaciones y estadísticas dentro de `terminal`.

- **Valor:** alto para el objetivo de unificar el producto; da visibilidad de resultados sin ejecutar nada.
- **Riesgo:** bajo; no envía órdenes ni toca exchanges. El riesgo principal es la autorización, que debe resolverse en servidor con propiedad de recursos.
- **Tamaño:** medio; requiere un esquema de lectura y pruebas de propiedad de datos.
- **Pendientes:** diseño de tablas en PostgreSQL (`terminal` aún no tiene journal persistido), decisión sobre la migración de datos de Neon y revisión de cómo se valida la identidad.

Esta propuesta no está aprobada. Queda pendiente la validación del propietario antes de elegir el piloto.
