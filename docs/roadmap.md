# Entregas y criterios de avance

## 0.1 — Base y planificación

- [x] Interfaz privada responsive, navegación y estados honestos de simulación.
- [x] Planificador decimal de entrada limpia, recompras y desmonte de cobertura.
- [x] Presupuesto en dinero/porcentaje y referencia capturada.
- [x] Pruebas de reglas, seguridad, componentes y API; pruebas de navegador incluidas.
- [x] Contrato OpenAPI, diseño PostgreSQL y plantilla de imagen del motor.
- [ ] Persistencia efectiva y pruebas de tenant isolation en PostgreSQL.
- [ ] Conexiones a exchanges o ejecución real (deliberadamente ausentes).

## 0.2 — Datos reales, solo lectura

Primera rama en revisión: `feat/railway-foundation`, destinada a Railway Hobby, uso personal y Binance Futures USDT. No es todavía una entrega 0.2 completa.

- [x] Definición Railway versionada, imágenes web/motor sin root y comunicación privada.
- [x] Runner administrativo de migraciones, checksums, bloqueo y comprobación de rol/esquema en el motor.
- [x] Pruebas PostgreSQL/RLS y builds de contenedor definidos como puertas de CI.
- [ ] Despliegue aprobado y comprobado en Railway; backups/restauración verificados.
- [ ] Resolver IP de salida fija antes de activar claves de trading (Hobby no incluye esta función).
- [x] Identidad personal persistida, sesiones revocables y auditoría de sesiones implementadas en `feat/persistent-owner-sessions` (PR dependiente de infraestructura; despliegue pendiente).
- [ ] Consumo efectivo de datos de cuentas/planes propios en las vistas.
- [x] En `feat/binance-readonly-vault`: envelopes cifrados con rotación administrativa y RLS, control de permisos/IP y panel manual de saldos/posiciones Binance USDT. Desactivado por defecto, sin claves reales ni despliegue verificado; no equivale a sincronización continua.
- [ ] Instrumentos/filtros, órdenes normales/algo, snapshots persistidos y eventos privados reconciliados; reemplazo de fixtures en las vistas conectadas.

1. Integrar identidad y sesiones revocables; no pasar el userId desde el cliente como autoridad.
2. Conectar PostgreSQL con migraciones versionadas y rol limitado; runner y readiness implementados, pendientes despliegue y consumo por los módulos.
3. Implementar almacenamiento cifrado de credenciales, restricción de permisos e IP.
4. Adaptador Binance de instrumentos, cuentas, órdenes normales/algo y posiciones.
5. Snapshots REST + eventos privados; deduplicación, antigüedad de datos y reconexión.
6. Eliminar fixtures de las vistas conectadas. Mantener sandbox explícitamente separado.

Criterio: posiciones/órdenes coinciden con el exchange tras reinicio y desconexión, y un usuario no puede acceder a una cuenta ajena.

## 0.3 — Ejecución controlada en testnet

Intenciones persistidas, clientOrderId determinista, propiedad por estrategia, filtros de instrumentos, protección confirmada, reconciliación antes de reanudar y cierre residual verificable. Un solo ejecutor por cuenta/instrumento; ninguna convivencia activa con bots antiguos.

Criterio: pruebas de timeout con orden aceptada, duplicados, grilla parcialmente enviada, SL rechazado, fills parciales, eventos fuera de orden y cancelaciones manuales. Nunca mostrar ACK como fill ni EXPIRED como cierre confirmado.

## 0.4 — Automatizaciones y journal

TP por cantidad/precio medio, coordinación de recompras y cobertura, presupuesto actualizado por fills reales, política explícita al detectar hedge, ledger de comisiones/funding, campañas y anotaciones. Telegram llama al mismo backend autorizado.

Criterio: resultados conciliados sin duplicación y protección documentada ante caída del motor; una compensación parcial no se presenta como cierre total.

## 0.5 — Bybit y preparación comercial

Adaptador Bybit con sus propias capacidades (no traducción ciega de Binance), MFA, roles y límites por usuario, throttling compartido, backups verificados, monitorización, gestión de suscripciones y revisión contractual/legal.

Antes de producción con capital: evaluación gradual con tamaños pequeños, kill switches y alertas operativas. No garantizar resultados ni pérdida monetaria exacta.
