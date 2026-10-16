# Regrow Terminal

Terminal privada con Next.js y un motor de cálculo Go. La transición a uso real comienza con infraestructura Railway y base PostgreSQL; **el trading continúa bloqueado y el preview es exclusivamente de simulación**. Todo el código nuevo vive aquí; los cuatro proyectos de `../src` permanecen intactos.

## Qué funciona

- Acceso personal validado en el servidor; sesiones firmadas y revocables de 8 horas (PostgreSQL obligatorio en producción), cookies HttpOnly, protección de origen, límites de solicitudes y CSP con nonce.
- Interfaz responsive: dashboard, posiciones ilustrativas filtrables, calculadora, riesgo, journal y configuración.
- Vistas previas calculadas en Go, no resultados financieros inventados en el navegador.
- Entrada limpia con relación beneficio/riesgo configurable.
- Recompras `1, 1, 2, 4…`, separación compuesta, límite de margen y filtro por SL.
- Cobertura con cierre inicial del 5 % / 10 % y factores 1,50 / 1,20; TP ponderado del lado contrario.
- Presupuesto en USDT o porcentaje de una base manual explícita (saldo/equity).
- Nuevo resultado neto de referencia en cada cálculo: realizado + flotante de ambos lados − comisiones + funding.
- Cálculo de exposición tras cierres parciales probado unitariamente; el HTTP preview muestra escenarios por nivel completo.
- Base de conexión PostgreSQL, migrador administrativo con historial/checksums y readiness de rol/esquema. Identidad personal, sesiones revocables, auditoría, cuentas provisionadas y última lectura manual del exchange persistidas; planes todavía pendientes.
- Imágenes web/motor para Railway y definición IaC versionada; no desplegadas desde esta entrega.
- Credenciales cifradas por envolvente y CLI administrativo con entrada oculta; panel manual de saldos/posiciones, órdenes abiertas normales/condicionales y catálogo/filtros de perpetuos Binance USDT en Configuración, solo lectura y desactivado por defecto. IDs int64 como texto, precios/cantidades decimales e intervalo de observación explícito. [Habilitación, permisos y límites](docs/binance-readonly.md).
- Guardado de la última lectura completa por cuenta con RLS y revalidación de sesión tras la consulta; recuperación manual desde PostgreSQL marcada como **guardada, no actual**, incluso con el lector externo desactivado. No es un historial de fills ni un checkpoint de reconciliación.

## Qué NO funciona todavía

No hay órdenes reales, monitor continuo, reconciliación, journal persistido, Bybit, migración de usuarios antiguos, Telegram, facturación ni autenticación multiusuario. Binance REST manual está implementado pero desactivado y no validado con claves reales por el agente; no se ha conectado ninguna cuenta. PostgreSQL debe configurarse explícitamente; las migraciones se aplican mediante un trabajo administrativo separado, nunca automáticamente al arrancar HTTP. Las vistas ilustrativas conservan fixtures; solo el panel de lectura, después de habilitarlo, puede mostrar respuestas de una cuenta conectada.

La vista previa no valida liquidación, filtros de instrumentos, liquidez ni disponibilidad de margen del exchange. El catálogo del panel de lectura es informativo y no convierte la simulación en una orden validada. No debe utilizarse como una instrucción ejecutable. Los cálculos de margen son aproximaciones de nocional/apalancamiento.

## Arranque local en Windows

Requisitos: Node.js 22+ y Go 1.26.9+ (la imagen de web usa Node.js 24). Go puede descargar el toolchain requerido si tienes la selección automática habilitada.

Desde `terminal/`:

```powershell
npm ci
./scripts/dev.ps1
```

El script pide una contraseña de al menos 16 caracteres, genera secretos aleatorios efímeros, inicia el motor y la web y no escribe credenciales a disco. Abre **http://localhost:3000**. Al terminar, detiene el proceso de motor que inició. Los tokens y sesiones de esa ejecución dejan de ser válidos al volver a arrancar.

Desarrollo usa sesiones revocables en memoria (`SESSION_BACKEND=local`), no datos reales. En producción solo se permite `postgres`: aplicar migraciones/grants y configurar motor/base antes de iniciar sesión. Cerrar sesión invalida también una cookie copiada; Configuración permite cerrar todas las sesiones. Rotar contraseña o `SESSION_SECRET` invalida las anteriores. Ver [despliegue y operación](docs/deployment.md).

Si el puerto predeterminado `8080` está ocupado (por ejemplo, por Apache), el script selecciona un puerto libre y configura la web para usarlo, sin detener el otro servicio. Comprueba que el motor de simulación esté listo antes de iniciar la web. Si indicas `-EnginePort`, ese puerto debe estar libre.

Para elegirlo explícitamente: `./scripts/dev.ps1 -EnginePort 8082`.

Si tu política de PowerShell bloquea scripts, revisa la política de tu equipo; no es necesario desactivar globalmente las protecciones. También puedes arrancar ambos procesos manualmente.

### Arranque manual

1. Copia `apps/web/.env.example` a `apps/web/.env.local` y sustituye TODOS los valores `replace-with-…` por secretos únicos. No uses los ejemplos como credenciales.
2. Configura `APP_ORIGIN=http://localhost:3000` y `ENGINE_URL=http://127.0.0.1:8080`.
3. En una consola situada en `apps/engine`, establece `ENGINE_API_TOKEN` al mismo token que en la web y ejecuta:

```powershell
$env:ENGINE_API_TOKEN = '<tu token aleatorio de 32 caracteres o más>'
go run ./cmd/engine
```

4. En otra consola, desde `terminal/`, ejecuta `npm run dev`.

El motor **no carga archivos .env automáticamente**. `ENGINE_ADDRESS` por defecto es `127.0.0.1:8080`. Los secretos web nunca utilizan prefijos `NEXT_PUBLIC_`.

Si la vista previa indica que la dirección no ofrece el motor, comprueba que `ENGINE_URL` apunte al puerto del proceso Go, no a Apache u otro servicio. En el arranque manual, cambia tanto `ENGINE_ADDRESS` como `ENGINE_URL` al mismo puerto libre y reinicia ambos procesos. Si la conexión privada es rechazada, comprueba que ambos usen el mismo `ENGINE_API_TOKEN`, sin compartirlo ni mostrarlo en logs.

## Verificación

Desde `terminal/`:

```powershell
npm run lint
npm run typecheck
npm test
npm run build
npm run test:standalone
npm audit --omit=dev
```

Desde `apps/engine/`:

```powershell
go test ./...
go vet ./...
```

Pruebas de navegador desde `terminal/`:

```powershell
npm exec --workspace @regrow/web -- playwright install chromium
npm run test:e2e
```

Las pruebas E2E inician servicios independientes en los puertos 3100/8081 con credenciales únicamente de prueba. No llaman a exchanges. El informe y las capturas quedan en directorios ignorados por Git.

`test:standalone` verifica producción bloqueada sin base; con `TEST_DATABASE_URL` de un PostgreSQL desechable en loopback prueba login, preview y revocación persistida completa (CI lo configura). Crea y elimina solo su base/rol aleatorios. Nunca usar una base de producción para estas pruebas.

Si la descarga de Chromium no está disponible, puedes usar un Chrome instalado: `$env:PLAYWRIGHT_CHANNEL = 'chrome'` antes de `npm run test:e2e`.

## Reglas financieras

Consulta [docs/strategy-rules.md](docs/strategy-rules.md). El motor utiliza decimales, transmite cantidades como strings y redondea divisiones a 32 decimales. La UI usa números binarios solo para **formatear visualmente** resultados; la sugerencia de riesgo usa enteros escalados y el motor vuelve a calcular el presupuesto de forma independiente.

Cambiar un campo invalida la vista previa previa. No se persisten planes ni se autoriza trading al calcular.

## Arquitectura y próximos pasos

```text
apps/web       → API privada de preview/sesiones → apps/engine → PostgreSQL
contracts      → OpenAPI del motor
database       → migraciones administrativas versionadas y grants limitados
docs           → reglas, decisiones, seguridad
infra          → imágenes y plantilla de despliegue
```

Orden siguiente: consolidar en `main` las entregas ya revisadas de sesiones y credenciales/lectura (los PRs #2 y #3 se mergearon en ramas dependientes; ver [control de versiones](docs/version-control.md)) → verificar despliegue y prerrequisitos operativos de lectura → instrumentos/filtros, órdenes normales/algo y reconciliación → ejecución idempotente en testnet → protección y automatizaciones → journal sincronizado → multiusuario / Bybit / Telegram.

No habilitar órdenes reales hasta completar los controles de [docs/security.md](docs/security.md). El despliegue acordado es web + motor + PostgreSQL en Railway: [docs/deployment.md](docs/deployment.md). Railway Hobby tiene pendiente la solución de IP de salida fija antes de trading. Para ramas, PRs revisados por el propietario y tags de versiones aprobadas, ver [docs/version-control.md](docs/version-control.md).
