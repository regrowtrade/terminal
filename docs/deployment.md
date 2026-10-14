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
7. Configurar `engine.DATABASE_URL` con ese rol limitado, host privado y base de datos correctos. La variable se preserva en IaC y **no** referencia la URL administrativa del producto PostgreSQL. Redeploy del motor: `DATABASE_REQUIRED=true` bloquea un arranque sin base; el esquema y los privilegios se verifican al conectar y en `/readyz`.
8. Desplegar web, iniciar sesión y verificar preview. Los datos ilustrativos siguen identificados como tales hasta que implementemos las vistas conectadas. No abrir registros públicos con el login actual.

## Migraciones y comprobaciones

`database/migrations` es la fuente de verdad. Las migraciones se aplican con el comando administrativo, nunca automáticamente en el motor. Se guardan nombre y SHA-256 (LF canónico) en `schema_migrations`; modificar una migración aplicada o ejecutar una versión anterior incompatible provoca rechazo. El bloqueo PostgreSQL serializa migradores y la transacción revierte DDL e historial si falla una migración. No hay rollback destructivo automático; hacer backup, probar restauración y añadir migraciones correctivas hacia delante.

Para una base local explícitamente elegida, desde `apps/engine`:

```powershell
# Inyecta MIGRATION_DATABASE_URL de forma segura en esta consola administrativa.
go run ./cmd/migrate
```

El motor no carga `.env`. `MIGRATIONS_DIR` por defecto es `../../database/migrations`; la imagen usa `/opt/terminal/migrations`. PostgreSQL externo a loopback/Railway requiere `sslmode=verify-full` y CA confiable. El endpoint `/healthz` comprueba liveness; `/readyz` verifica dependencias configuradas y no devuelve URLs ni errores del driver. Ninguno comprueba conexión a Binance o protección de posiciones.

Las pruebas PostgreSQL se ejecutan en CI contra un contenedor desechable. Para ejecutarlas localmente, configurar `TEST_DATABASE_URL` apuntando **solo a un servidor de pruebas**, con permisos de crear bases/roles; crean y eliminan únicamente su base/rol de nombre aleatorio. Sin esa variable se omiten explícitamente. Nunca apuntarla a producción.

## Siguientes puertas de seguridad

Identidad personal persistida y sesiones revocables → credenciales cifradas y Binance solo lectura → snapshots/eventos reconciliados → ejecución testnet con recuperación/idempotencia → revisión operativa y activación gradual. No hay claves de exchange, órdenes ni retiros en esta entrega.
