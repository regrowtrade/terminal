# Seguridad y preparación para trading real

## Controles implementados

- Acceso privado sin credenciales por defecto. Contraseña de 16+ caracteres y secreto de 32+; los placeholders se rechazan.
- Comparación de contraseña mediante scrypt y timingSafeEqual; token del motor comparado en tiempo constante.
- Sesiones HMAC con expiración, emisión, nonce y rol verificados; cookies HttpOnly / SameSite=Strict y Secure / __Host en producción.
- Páginas y API validan sesión independientemente; el proxy no se usa como autorización.
- Mutaciones POST exigen Origin exacto configurado. No hay CORS público del motor.
- Throttling de login y preview en memoria. Lectura de cuerpos acotada. Go rechaza campos desconocidos y JSON concatenado; web valida Zod estricto.
- Tokens del motor exclusivamente server-side; errores sanitizados, no logging de cuerpos o secretos.
- CSP por petición con nonce de scripts; estilos inline permitidos por la UI. Sin unsafe-inline para scripts de producción. Frame denial, nosniff, restricciones de permisos y no-store en sesiones/API.
- Motor privado por defecto, timeouts HTTP y cierre controlado.
- No existe endpoint de ejecución ni formulario de API keys.
- Base PostgreSQL opcional en local y obligatoria en Railway: pool acotado, rechazo de roles administrativos/propietarios y comprobación de historial del esquema. `SET LOCAL app.user_id` se limita a la transacción; aún no hay endpoints que persistan identidades o planes.
- Migraciones en proceso administrativo separado, sin credenciales de administración en servicios HTTP. Checksums, bloqueo y aplicación transaccional; no rollback destructivo automático.

## Límites actuales

Es acceso personal, no un sistema de identidad comercial. La limitación en memoria no es distribuida y se reinicia al desplegar; Vercel puede ejecutar múltiples instancias. No abrir el producto al público con este mecanismo. Logout borra la cookie pero no revoca una copia robada; rotar SESSION_SECRET revoca todas las sesiones. Cambiar solo la contraseña no revoca sesiones anteriores. No hay MFA ni recovery.

Hay conexión y verificación PostgreSQL en el motor cuando se configura, pero no se guardan todavía identidades, sesiones, planes ni auditoría desde HTTP. Las tablas y el helper transaccional incorporan FKs/RLS y tienen pruebas de integración definidas en CI; eso no sustituye la autorización de futuros endpoints. No hay cifrado de credenciales de exchange ni monitor de posiciones. El despliegue y las pruebas contra Railway están pendientes.

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
