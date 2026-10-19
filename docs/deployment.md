# Despliegue objetivo: Railway

Decisión: uso personal, Binance Futures USDT y web + motor + PostgreSQL en Railway. Railway es una plataforma de servicios/contenedores, no un VPS administrado directamente. **No se ha creado infraestructura ni desplegado esta rama. No hay conexión a Binance ni ejecución.**

## Servicios y red

La definición versionada está en `.railway/railway.ts` (SDK fijado en el lockfile). Usa Infrastructure as Code; no crea recursos al evaluar el archivo. Las nuevas instalaciones no deben usar los archivos `railway.json`/`railway.toml` obsoletos.

| Servicio   | Fuente                                            | Acceso                                                      |
| ---------- | ------------------------------------------------- | ----------------------------------------------------------- |
| `web`      | `infra/web.Dockerfile`, Next.js standalone        | Único dominio público, HTTPS                                |
| `engine`   | `infra/engine.Dockerfile`, Go                     | Red privada, puerto 8080, token servidor-servidor           |
| `postgres` | Producto PostgreSQL de Railway                    | Red privada, sin TCP proxy público                          |
| `migrator` | Misma imagen Go, comando `/usr/local/bin/migrate` | Trabajo administrativo explícito, termina; no servidor HTTP |

Ambas imágenes usan la raíz del repositorio `terminal` como contexto y usuarios de runtime sin root. No necesitan secretos durante el build. El motor escucha en `[::]:8080` para admitir IPv6 y redes dual-stack. La web llama a `http://engine.railway.internal:8080`; Railway cifra el tráfico privado con Wireguard.

Una réplica por servicio y sin suspensión. Eso **no es** un bloqueo distribuido ni evita solapamientos de despliegue: el futuro ejecutor necesitará exclusión y fencing antes de habilitar órdenes. Seleccionar una región permitida por Binance Futures y medir latencia; no eludir restricciones regionales.

## Railway Hobby: límite pendiente

El plan contratado es Hobby. Según la [documentación de IP de salida estática](https://docs.railway.com/networking/static-outbound-ips), esa función requiere Pro. Una IP pública del dominio no equivale a una IP de salida fija. Antes de almacenar claves de trading o habilitar ejecución, habrá que pasar a un plan compatible o aprobar una salida fija segura. No relajar la lista de IP permitidas como solución. Confirmar también backups y presupuesto de servicios/volúmenes en el plan elegido.

## Preparación, después del merge revisado

1. Conectar GitHub a Railway y seleccionar un entorno privado del proyecto. No desplegar la rama de trabajo con capital ni conectar bots antiguos.
2. Instalar/autenticar Railway CLI en tu equipo. Desde `terminal`: `railway login`, `railway link`, `railway config plan`. Revisar servicios, región, costes y cualquier eliminación. **No ejecutar apply automáticamente.** Si el proyecto ya tiene recursos, importarlos sin incluir secretos antes de aplicar; un archivo de proyecto completo puede proponer eliminaciones de recursos omitidos.
3. Crear variables compartidas selladas, generadas de forma única: `ENGINE_API_TOKEN` (32+ caracteres), `SESSION_SECRET` (32+) y `TERMINAL_PASSWORD` (16+). No pegarlas en el chat ni en Git. La configuración referencia estas variables; no genera credenciales por defecto.
4. Tras aprobar el plan, aplicar interactivamente. La fuente declarada es `main`, no la rama del PR. Generar dominio HTTPS **solo para web** y establecer `APP_ORIGIN` al origen exacto, sin path. Dejar motor y PostgreSQL sin exposición pública.
5. Ejecutar/desplegar explícitamente `migrator` y comprobar su finalización satisfactoria. Solo ese trabajo recibe `MIGRATION_DATABASE_URL` administrativa. No configurar esta variable en motor ni web. No configurar migraciones como build ni como tarea del servidor HTTP.
6. En una consola segura de PostgreSQL, crear `terminal_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS` con contraseña única, sin membresía de roles propietarios/administrativos. Ejecutar `database/runtime-grants.sql` después de las migraciones. No utilizar el usuario `postgres` en el motor.
7. Configurar `engine.DATABASE_URL` con ese rol limitado, host privado y base de datos correctos. La variable se preserva en IaC y **no** referencia la URL administrativa del producto PostgreSQL. Redeploy del motor: `DATABASE_REQUIRED=true` bloquea un arranque sin base; el esquema y los privilegios se verifican al conectar y en `/readyz`. La migración `0002_personal_sessions.sql` provisiona una identidad personal de UUID aleatorio, estable en esa base; no introduce credenciales. Reaplicar `runtime-grants.sql` tras actualizar. La web usa `SESSION_BACKEND=postgres`; no puede iniciar sesión en producción sin registro persistido.
8. Desplegar web, iniciar sesión y verificar preview. Los datos ilustrativos siguen identificados como tales hasta que implementemos las vistas conectadas. No abrir registros públicos con el login actual.

## Migraciones y comprobaciones

`database/migrations` es la fuente de verdad. Las migraciones se aplican con el comando administrativo, nunca automáticamente en el motor. Se guardan nombre y SHA-256 (LF canónico) en `schema_migrations`; modificar una migración aplicada o ejecutar una versión anterior incompatible provoca rechazo. El bloqueo PostgreSQL serializa migradores y la transacción revierte DDL e historial si falla una migración. No hay rollback destructivo automático; hacer backup, probar restauración y añadir migraciones correctivas hacia delante.

Para una base local explícitamente elegida, desde `apps/engine`:

```powershell
# Inyecta MIGRATION_DATABASE_URL de forma segura en esta consola administrativa.
go run ./cmd/migrate
```

El motor no carga `.env`. `MIGRATIONS_DIR` por defecto es `../../database/migrations`; la imagen usa `/opt/terminal/migrations`. PostgreSQL externo a loopback/Railway requiere `sslmode=verify-full` y CA confiable. El endpoint `/healthz` comprueba liveness; `/readyz` verifica dependencias configuradas y no devuelve URLs ni errores del driver. Ninguno comprueba conexión a Binance o protección de posiciones.

Al introducir el corte `capturedAtMs`, las capturas anteriores requieren una [transición administrativa explícita](reconciliation.md#transición-de-snapshots-anteriores-al-corte-por-exchange) antes de volver a leer esa cuenta. La migración `0009_legacy_snapshot_archives.sql` solo crea el archivo: `/usr/local/bin/snapshot-transition` inspecciona por defecto y requiere `--apply --expected-sha256` para preservar y retirar un baseline antiguo. Detener escritores antiguos, verificar backup/restauración y no ejecutar automáticamente al desplegar. El runtime carece de permisos sobre el archivo y no recibe la URL administrativa.

La consulta manual de órdenes normales requiere además `0010_order_observations.sql` y reaplicar `database/runtime-grants.sql` antes del motor nuevo. Persiste evidencia append-only bajo RLS; no recupera trades ni habilita órdenes. [Límites y operación](reconciliation.md#consulta-explícita-de-una-orden-normal). Sin aplicar migraciones a Railway por el agente.

Las pruebas PostgreSQL se ejecutan en CI contra un contenedor desechable. Para ejecutarlas localmente, configurar `TEST_DATABASE_URL` apuntando **solo a un servidor de pruebas**, con permisos de crear bases/roles; crean y eliminan únicamente su base/rol de nombre aleatorio. Sin esa variable se omiten explícitamente. Nunca apuntarla a producción. `npm run test:standalone` usa esa misma variable (solo loopback) para probar el flujo completo de producción: PostgreSQL real, dos dispositivos, preview y rechazo de cookies copiadas tras logout individual/global. Sin base verifica que producción bloquea login, sin degradar a sesión local, y comprueba el preview directo del motor con entradas sintéticas.

## Operación de sesiones personales

- Las sesiones antiguas firmadas sin registro dejan de ser válidas: iniciar sesión de nuevo después de esta actualización. Las sesiones persistidas sobreviven a reinicios de web/motor si se conservan secretos y base.
- En Configuración, «Cerrar todas las sesiones» invalida las sesiones existentes, incluida la actual. Un login posterior con contraseña correcta sigue permitido. Límite de 20 sesiones activas; cerrar las existentes o esperar su expiración antes de abrir más.
- Cambiar `TERMINAL_PASSWORD` o `SESSION_SECRET` y redesplegar todas las instancias web invalida sus cookies anteriores. No se elimina historial de auditoría. Los registros revocados/expirados se depuran al iniciar una nueva sesión.
- Las sesiones invalidadas solo por rotación aún cuentan en el límite hasta su expiración; si no queda una cookie válida y se alcanzó el límite, un administrador debe revocar las filas de `owner_sessions` del propietario desde una consola segura (con el contexto RLS apropiado), o esperar ocho horas. No dar permisos administrativos al motor para resolverlo.
- Si la base cae, las peticiones protegidas se rechazan. Logout responde 503 cuando no puede confirmar la revocación: reintentar cuando el servicio vuelva. No asumir revocación solo por cerrar una pestaña.
- El login y preview conservan rate limiting local: mantener una réplica, acceso privado y no habilitar registros públicos. MFA y throttling compartido siguen pendientes antes de trading real.

## Siguientes puertas de seguridad

La entrega `feat/binance-readonly-vault` agrega la migración 0003, el CLI administrativo `/usr/local/bin/credentials` y un panel de lectura REST manual. `feat/binance-market-orders` amplía la lectura con órdenes normales/algo, instrumentos/filtros y la última captura persistida (migración 0004). Aplicar migraciones y reaplicar grants mediante el proceso administrativo antes de actualizar el motor; actualizar web y motor coordinadamente por el cambio de respuesta estricta. La lectura externa está desactivada por defecto; no copiar KEKs al servicio web/migrador ni introducir claves reales hasta resolver salida fija. La definición IaC preserva variables del motor, no las crea con secretos/defaults. Guía de habilitación, límites, recuperación histórica y rotación: [binance-readonly.md](binance-readonly.md).

Las entregas previas están consolidadas en `main` con los PRs #4 y #5; revisar después la comparación durable (`feat/binance-observation-comparison`). Esta última requiere aplicar `0005_observation_comparisons.sql` antes de actualizar todos los motores; no mezclar escritores antiguos/nuevos y comprobar readiness del historial. Los reportes antiguos quedan NULL hasta una nueva lectura completa: no se rellenan con evidencia inventada. Después: despliegue y comprobación de identidad/sesiones → credenciales cifradas y Binance solo lectura → eventos privados y reconciliación (una lectura guardada o comparación REST no los sustituye) → ejecución testnet con recuperación/idempotencia → revisión operativa y activación gradual. No se han conectado claves reales ni enviado/cancelado órdenes desde esta entrega.
