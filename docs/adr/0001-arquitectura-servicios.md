# ADR 0001 — Arquitectura de servicios: un runtime de dominio en Go

- **Estado:** Propuesta. Pendiente de revisión y aprobación del propietario (REG-7).
- **Fecha:** 2026-10-27.
- **Relacionado:** REG-5 (épica), REG-6 (inventario aprobado), REG-13 (piloto `trading-journal`), [inventario](../inventario-src.md), [despliegue](../deployment.md).

## Contexto

Los cuatro proyectos legacy de `../src` se van a unificar en `terminal`. Tres de ellos contienen lógica de trading en Python o Go y uno contiene un journal en Next.js. El inventario (REG-6) concluyó que las reglas de los bots deben reescribirse, no ejecutarse tal cual.

Hoy `terminal` tiene:

- `apps/web`: Next.js 16 con CSS propio. No tiene credenciales de base de datos (`DATABASE_URL` no está en su configuración de Railway). Persiste sus sesiones llamando al motor (`/v1/auth/sessions/*`).
- `apps/engine`: Go. Expone una API privada con token de servidor, calcula planes en decimal y gestiona la persistencia en PostgreSQL, incluida la lectura de Binance en modo solo lectura.
- PostgreSQL, con migraciones versionadas y RLS, y un servicio `migrator` administrativo.

La regla operativa es que un solo controlador actúe sobre cada cuenta e instrumento, y que no convivan bots antiguos con el futuro ejecutor (ver [roadmap](../roadmap.md)).

## Decisión

1. **Un único runtime de dominio: `apps/engine` en Go.** Las reglas de los bots (TP, cobertura, recompras) se reescriben como módulos de estrategia dentro del motor, con las pruebas del planificador existente.
2. **Python no se despliega como servicio en `terminal`.** Los scripts Python de `../src` quedan solo como referencia de reglas y de casos de prueba.
3. **La web no accede a datos directamente.** Hace de capa de presentación y de proxy autenticado hacia el motor. Solo el motor conoce las credenciales de PostgreSQL.
4. **No se crea ningún servicio nuevo** para el piloto ni para el futuro ejecutor. El ejecutor será un módulo del motor con exclusión por cuenta e instrumento.

## Alternativas consideradas

| Opción                                               | Ventajas                                                                                                                              | Costes y riesgos                                                                                                                                                                                               | Resultado    |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ |
| **A. Servicio Python separado** que ejecuta los bots | Reutiliza código existente; menos trabajo inicial.                                                                                    | Un quinto servicio en Railway; contrato adicional web/motor/Python; dos motores de aritmética (`float` en Python, decimal en Go); dos procesos que pueden competir por la misma cuenta; pruebas en dos stacks. | Descartada.  |
| **B. Portar las reglas a Go** dentro del motor       | Una sola aritmética decimal; un solo controlador por cuenta; pruebas en un único stack; el planificador ya tiene reglas equivalentes. | Esfuerzo de reescritura; paridad funcional a verificar con pruebas.                                                                                                                                            | **Elegida.** |
| **C. Mantener los bots legacy en paralelo**          | Sin trabajo de migración.                                                                                                             | Incumple la regla de no convivir con bots antiguos; riesgo de órdenes duplicadas.                                                                                                                              | Descartada.  |

## Consecuencias por módulo

| Módulo legacy      | Destino                                                                                                     | Notas                                                                                                                                 |
| ------------------ | ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `automatic-tp`     | Reescritura en `apps/engine` como módulo de TP.                                                             | Requiere reconciliación de fills y recuperación idempotente antes de ejecutar (ver [reconciliación](../reconciliation.md)).           |
| `hedge-management` | Reescritura del desmonte de cobertura; contrastar con el planificador Go.                                   | Las notificaciones de Telegram no se portan en esta fase; si se quieren, serán un cliente del motor, no un proceso con lógica propia. |
| `rebuymaster`      | Reglas de recompras al planificador Go (modo `dca`); ejecución fuera de alcance hasta el gate de ejecución. | No trasladar rutas HTTP ni clientes de exchange sin controles.                                                                        |
| `trading-journal`  | Piloto en modo lectura (REG-13).                                                                            | Sin `userId` del cliente como autoridad; la propiedad se valida en el servidor.                                                       |

## Responsabilidades

| Componente      | Responsabilidad                                                                                                                         | No hace                                                                             |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `web` (Next.js) | Interfaz, validación de entrada con esquemas zod, cookie de sesión HttpOnly, proxy autenticado al motor.                                | Cálculos financieros; acceso a PostgreSQL; exponer el token del motor al navegador. |
| `engine` (Go)   | Cálculo de planes en decimal, persistencia, lectura de Binance (solo lectura), reglas de estrategia y, en su día, ejecución controlada. | Servir HTML; confiar en `userId` del cliente.                                       |
| `postgres`      | Datos de usuario y evidencia, con RLS.                                                                                                  | Lógica de negocio.                                                                  |
| `migrator`      | Aplicar migraciones versionadas.                                                                                                        | Servir peticiones HTTP; arrancar automáticamente con el motor.                      |

## Contratos, errores y configuración

- **Contrato:** `contracts/openapi.yaml` es la especificación del motor. Los DTO de Go y los esquemas zod de la web deben mantenerse alineados con él.
- **Errores:** respuestas con códigos acotados y sin URLs, trazas ni mensajes del driver (ya aplicado en `/readyz`).
- **Configuración:** el motor no lee `.env`; recibe variables del entorno. Los secretos nunca usan prefijos `NEXT_PUBLIC_` y no se incluyen en la imagen.
- **Local:** `scripts/dev.ps1` arranca motor y web con sesiones en memoria y sin datos reales.
- **CI:** el workflow `Terminal quality gates` ejecuta los jobs `postgres`, `containers`, `engine` y `web`.

## Propiedad de datos

- El motor es el único dueño del esquema y de los datos.
- Cada consulta se filtra por el usuario de la sesión validada en el servidor, y PostgreSQL aplica RLS.
- Las migraciones solo las aplica el trabajo `migrator`, nunca el motor al arrancar.

## Flujo trazable del piloto (REG-13)

1. El navegador solicita `/journal` con su cookie de sesión.
2. La web valida la sesión llamando al motor y, si es válida, pide los datos al motor con el token privado (nunca visible en el navegador).
3. El motor valida la propiedad, consulta PostgreSQL con el rol limitado y devuelve cantidades como texto decimal.
4. La UI muestra los datos con la etiqueta de solo lectura y sin acciones de ejecución.

El endpoint de lectura del journal no existe todavía; lo implementará REG-13.

## Seguridad y límites

- Esta decisión **no habilita trading real**. Los gates futuros son: reconciliación con el exchange, almacenamiento de credenciales con permisos e IP, ejecución recuperable y pruebas de fallo. Cualquier cambio que habilite órdenes requiere su propio ADR y aprobación.
- Railway Hobby no incluye IP de salida fija; esto bloquea cualquier clave de trading hasta resolverlo ([despliegue](../deployment.md)).
- Las pruebas no llaman a exchanges. Los clientes de Binance se prueban con transportes sintéticos.

## Pendientes

- Confirmar región, coste y plan de Railway antes de cualquier despliegue (ver [despliegue](../deployment.md)).
- Diseñar el contrato OpenAPI del journal de lectura (REG-13).
- Definir la estrategia de paridad funcional para cada módulo migrado, con pruebas antes de retirar el legacy.

## Aprobación

Pendiente de revisión del propietario. Este documento no se considera aprobado hasta que el propietario lo confirme en REG-7.
