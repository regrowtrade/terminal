# Integración de los cuatro proyectos

El destino único es `terminal/`. `../src/rebuymaster`, `../src/hedge-management`, `../src/automatic-tp` y `../src/trading-journal` permanecen como referencias, sin cambios ni ejecución desde estas entregas. No se está fusionando ciegamente sus repositorios, autenticación, credenciales o bases de datos.

| Origen             | Incorporado en la terminal                                                                                                                                                                       | Todavía pendiente                                                                                                                                                                                                           |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rebuymaster`      | Parte de las reglas de recompras en `apps/engine/internal/planner`, modo `dca`: secuencia Q/Q/2Q/4Q, separación compuesta, presupuesto y límites. Preview web de simulación.                     | Paridad funcional completa (incluido incremento de volumen arbitrario), planes persistidos, envío/recuperación de grillas, coordinación por fills y gestión continua del SL/TP.                                             |
| `hedge-management` | Preview decimal de desmonte de cobertura, referencias de PnL neto y cálculo puro de exposición/cierres parciales; reglas y pruebas en `docs/strategy-rules.md` y `apps/engine/internal/planner`. | Campañas persistidas, asociación a posiciones reales, reconciliación de ejecuciones, ajustes de TP/SL y recuperación tras desconexión.                                                                                      |
| `automatic-tp`     | El preview calcula objetivos estáticos de TP. **No se ha incorporado todavía su controlador automático**.                                                                                        | Seguimiento por cantidad/precio medio reconciliados, coordinación con recompras/cobertura, confirmación de órdenes y recuperación idempotente. No arrancar el script antiguo en paralelo al futuro ejecutor.                |
| `trading-journal`  | Ruta privada `/journal` con estado pendiente explícito y esquema de base para campañas, ejecuciones, ledger y notas.                                                                             | Persistencia funcional del journal, importación/deduplicación de fills, comisiones/funding, anotaciones, resultados conciliados y migración de datos antiguos. Las tablas diseñadas no equivalen a funciones implementadas. |

## Base transversal que comparten

- Web Next.js y motor Go con aritmética decimal y contrato privado.
- Identidad validada en servidor, sesiones revocables, RLS y credenciales cifradas, sin aceptar userId como autoridad del navegador.
- Lectura Binance USDT manual de cuentas, posiciones, órdenes normales/algo e instrumentos y última observación persistida, incorporadas en `main` con los PRs #4/#5. Sin validación operativa con una cuenta real por el agente.
- Comparación durable entre observaciones incorporada en `main` con el PR #6: no reemplaza eventos ni reconciliación.
- En el PR #7 (`feat/connected-account-views`), actualizado con `main` después del PR #6: ruta Posiciones de cuenta sin fixtures, con ejemplos opt-in, filtros locales y paginación; conserva la recuperación explícita de comparaciones históricas. Revisión/integración pendientes.

## Orden y puertas del plan

1. Terminar datos propios de cuenta/planes y reconciliación de snapshots + eventos privados, incluyendo pérdida/reordenación de eventos y reinicios. Las lecturas o diferencias REST no bastan.
2. Ejecución controlada **en testnet**, con intenciones durables, IDs deterministas, estados ambiguos y un solo controlador por cuenta/instrumento. No convivir con los bots antiguos.
3. Automatizaciones coordinadas de TP, recompras y cobertura sobre esa ejecución recuperable, no cuatro procesos independientes compitiendo por la cuenta.
4. Journal de resultados/costes conciliados y notas. La migración de usuarios/datos requiere diseño y aprobación explícitos; no copiar secretos ni identidades confiadas al cliente.
5. Expansión posterior (Bybit/multiusuario/Telegram) y revisión operativa antes de capital real.

No se marca un proyecto como integrado solo por haber trasladado una fórmula, creado una pantalla o mergeado un PR. Los criterios y las limitaciones de cada fase siguen en [roadmap](roadmap.md). Despliegue Railway, salida fija, backups/restauración y validación real son puertas operativas separadas; no se ejecutan automáticamente al terminar código.
