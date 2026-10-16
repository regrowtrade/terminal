# Seguridad y preparación para trading real

## Controles implementados

- Acceso privado sin credenciales por defecto. Contraseña de 16+ caracteres y secreto de 32+; los placeholders se rechazan.
- Comparación de contraseña mediante scrypt y timingSafeEqual; token del motor comparado en tiempo constante.
- Sesiones HMAC con expiración, emisión, nonce y rol verificados; cookies HttpOnly / SameSite=Strict y Secure / __Host en producción.
- En producción, cada sesión debe estar registrada en PostgreSQL: digest HMAC ligado a cookie + contraseña, nunca cookie ni contraseña en la base. Cada petición protegida consulta vigencia/revocación sin caché. La identidad personal se crea por migración administrativa; el runtime no puede reasignarla ni aceptar un userId del navegador.
- Logout revoca una copia de la cookie; «Cerrar todas las sesiones» requiere una sesión vigente. Crear/revocar se serializa en PostgreSQL. Auditoría de altas y revocaciones sin tokens, hashes ni contraseñas; duración máxima de ocho horas y 20 sesiones activas. Cambiar contraseña o SESSION_SECRET invalida también las cookies anteriores (reiniciar/redeploy de todas las instancias web).
- Páginas y API validan sesión independientemente; el proxy no se usa como autorización.
- Mutaciones POST exigen Origin exacto configurado. No hay CORS público del motor.
- Login/logout redirigen al `APP_ORIGIN` canónico, no al host interno ni a cabeceras forwarded del proxy. HTTPS obligatorio salvo loopback local; una configuración de origen inválida se rechaza.
- Throttling de login y preview en memoria. Lectura de cuerpos acotada. Go rechaza campos desconocidos y JSON concatenado; web valida Zod estricto.
- Tokens del motor exclusivamente server-side; errores sanitizados, no logging de cuerpos o secretos.
- CSP por petición con nonce de scripts; estilos inline permitidos por la UI. Sin unsafe-inline para scripts de producción. Frame denial, nosniff, restricciones de permisos y no-store en sesiones/API.
- Motor privado por defecto, timeouts HTTP y cierre controlado.
- No existe endpoint de ejecución ni formulario de API keys. La herramienta administrativa provisiona credenciales HMAC por entrada oculta, cifradas por envolvente AES-256-GCM y ligadas a propietario/cuenta/entorno; el runtime solo tiene SELECT sobre el envelope. KEKs solo en motor/consola, no web/base. Ver [Binance solo lectura](binance-readonly.md).
- La lectura manual incluye órdenes normales/algo e instrumentos. Solo GET a una allowlist fija, sin credenciales en endpoints públicos; IDs int64 como strings, intervalos y resultados no atómicos explícitos. Rechazo completo de componentes ausentes/inválidos, límite de tamaño/tiempo y exclusión de consultas simultáneas por cliente. Publicar filtros o una orden condicional no valida un plan ni acredita protección.
- Al guardar, se revalidan también los datos financieros normalizados de la captura anterior y actual. La comparación se calcula dentro de la transacción serializada del propietario y se guarda junto a su snapshot exacto. Errores de baseline, concurrencia fuera de orden o confirmación no devuelven un reporte parcial. El endpoint de comparación deriva identidad del servidor y no acepta baselines del navegador; siempre declara continuidad no establecida y ejecución no habilitada.
- Base PostgreSQL opcional en desarrollo local y obligatoria para acceso en producción: pool acotado, rechazo de roles administrativos/propietarios y comprobación de historial del esquema. `SET LOCAL app.user_id` se limita a la transacción; sesiones con RLS, sin endpoints de persistencia de planes aún.
- Migraciones en proceso administrativo separado, sin credenciales de administración en servicios HTTP. Checksums, bloqueo y aplicación transaccional; no rollback destructivo automático.

## Límites actuales

Es acceso personal, no un sistema de identidad comercial. La contraseña sigue en el gestor de secretos del servidor; no hay registro, hash de contraseña administrado desde UI, MFA ni recovery. El rate limit de login/preview aún está en memoria y se reinicia al desplegar: no escalar ni abrir registros hasta incorporar limitación compartida. La verificación de contraseña vive en la web; el token privado del motor es una credencial de confianza capaz de registrar sesiones, debe permanecer protegido.

`SESSION_BACKEND=local` solo está permitido fuera de producción: sesiones revocables en memoria, perdidas al reiniciar, sin auditoría durable. Producción usa `postgres` por defecto y rechaza explícitamente `local` o modos desconocidos. Una caída de base/motor bloquea acceso; nunca degrada a cookies stateless. Si logout no puede confirmar revocación devuelve 503 y conserva la cookie para reintentar, sin afirmar éxito. Solicitudes ya autorizadas pueden terminar tras una revocación; futuras operaciones financieras deberán verificar autorización junto a su transacción durable.

Se persisten identidad personal, sesiones, eventos de sesión, envelopes de credenciales provisionadas administrativamente y la última lectura manual completa por cuenta. No se guardan aún planes desde HTTP. La lectura se guarda solo después de revalidar sesión/cuenta; no se mantiene una transacción durante el acceso externo. RLS/FK compuesta aíslan snapshots; un resultado anterior no sobrescribe una captura posterior y un fallo no devuelve una lectura antigua automáticamente. La recuperación explícita consulta solo PostgreSQL y se etiqueta como guardada/no actual. Los snapshots contienen datos financieros confidenciales en JSONB (no API keys); no se cifran por envolvente y requieren los controles de acceso, disco y backups de la base. No son un ledger ni una autorización para ejecutar. El adaptador Binance REST es solo lectura, manual, desactivado por defecto y sin validación real desde esta entrega; rechaza claves con trading/retiros/transferencias o sin IP restringida. No hay monitor continuo, reconciliación ni KMS administrado. El despliegue y las pruebas contra Railway están pendientes.

## Dependencias

Usar lockfiles, `npm ci`, `npm audit --omit=dev`, `go vet` y análisis de vulnerabilidades periódico. Revisar el informe completo, sin downgrades automáticos forzados.

La rama de infraestructura requiere Go 1.26.9+ y fija pgx 5.9.2, x/crypto 0.58.0 y x/text 0.43.0 tras detectar vulnerabilidades en versiones anteriores con govulncheck. CI ejecuta el scanner fijado a v1.9.0 además de tests y vet; no considerar una versión segura indefinidamente sin volver a comprobarla.

En la instalación inicial, npm audit informa GHSA-vfj7-8cjw-p6xm en `braces` 3.0.3 y dependencias de ESLint/Next lint (cinco entradas de severidad alta). El registry consultado no ofrece una versión reparada de braces. No pertenece al runtime de producción y recibe globs locales confiables; no aceptar patrones no confiables en esas herramientas. No se declara solucionado. Revalidar y actualizar cuando exista un parche compatible.

## Antes de habilitar ejecución real

1. Identidad robusta, MFA, autorización por usuario/cuenta y throttling compartido.
2. Credenciales cifradas por envolvente/KMS, rotación, API keys sin retiros y con restricción de IP. Nunca almacenar claves en el navegador o logs.
3. Filtros de instrumentos, modos de posición/margen, margen disponible y liquidación verificados.
4. Órdenes normales/condicionales reconciliadas, fills deduplicados y seguimiento de cambios externos.
5. Intención durable antes de enviar; clientOrderId determinista; recuperación por consulta ante timeout; no reintentar ciegamente.
6. Un controlador por cuenta/instrumento con exclusión y fencing antes de múltiples workers.
7. Protección confirmada antes de aumentar exposición; política explícita cuando falta SL, conexión o datos.
8. Reconciliación antes de reanudar tras reinicio. Pruebas de parciales, rechazo de protección y cierre residual.
9. Separación inequívoca entre pausar, cancelar y cerrar; alcance explícito sobre órdenes manuales.
10. Testnet, límites conservadores y migración de un único controlador por activo. Nunca competir con los bots antiguos.

La revisión legal y contractual precede a la comercialización. No presentar resultados o límites de ejecución como garantizados.
