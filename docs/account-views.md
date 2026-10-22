# Vistas de cuenta y simulación

La ruta privada **Posiciones** abre por defecto **Cuenta Binance · Solo lectura**, sin inventar una exposición ni descargar datos automáticamente. **Ejemplos de simulación** es una opción explícita y separada. El dashboard sigue siendo ilustrativo y la calculadora sigue siendo un sandbox; ninguna lectura se convierte en un plan, una estrategia ni una orden.

## Flujo de lectura

1. **Comprobar conexión** consulta los metadatos locales de cuentas provisionadas. No contacta a Binance ni pide claves al navegador.
2. Elegir cuenta. El servidor valida sesión, pertenencia y RLS en cada operación; el cliente no decide el propietario.
3. **Consultar cuenta y órdenes** solo está disponible con el lector externo habilitado y sus prerrequisitos aprobados. Reutiliza la lectura completa del PR #5 y su persistencia obligatoria. No cambia permisos, límites de cuota ni destinos del adaptador.
4. **Mostrar última lectura guardada** recupera explícitamente PostgreSQL, incluso con lectura externa desactivada. Siempre muestra **LECTURA GUARDADA · NO ACTUAL**, fecha de guardado e intervalo original. No llama a Binance ni confirma la situación actual.
5. **Consultar reporte de reconciliación** consulta PostgreSQL, también con el lector externo desactivado. Muestra la lectura base, continuidad registrada, conteos, último evento y fills posteriores al corte con IDs/decimales exactos. Siempre es **REPORTE PUNTUAL · NO EN VIVO · EJECUCIÓN BLOQUEADA**: no combina el reporte con las posiciones como si fueran un estado sincronizado. Una cuenta sin baseline requiere primero una lectura de cuenta.
6. **Recuperar y guardar fills** sí contacta Binance, requiere el lector habilitado y un símbolo USDT con inicio/fin **UTC** (máximo siete días). Guarda evidencia deduplicada, informa fills nuevos/duplicados y truncamiento. Retira el reporte anterior antes de consultar, incluso si la respuesta falla: el servidor podría haber guardado evidencia. Consultar nuevamente el reporte es una acción explícita, nunca un reintento automático. La recuperación no restablece continuidad ni prueba ausencia de ejecuciones cuando devuelve cero.

El inicio de recuperación está incluido y el fin excluido. Se subdividen ventanas saturadas dentro de un presupuesto de 20 consultas, sin mezclar `fromId` y fechas. Una ventana de un solo milisegundo aún saturada también se marca truncada; no se saltan sus fills ni se presenta como completa. Una ventana vacía válida devuelve cero tras verificar sesión/propiedad/credencial, no un error. Los siete días limitan la consulta, no garantizan la retención de Binance.

Configuración y Posiciones reutilizan el mismo panel/controlador y las mismas tablas de cuenta. El PR #7 se actualizó con `main` tras integrar el PR #6 y conserva **Ver cambios de la última lectura** en ambas vistas. Recuperar esa comparación sigue siendo una acción explícita, histórica y sin contacto con Binance; no requiere habilitar el lector externo. Cambiar cuenta, fuente o iniciar otra consulta elimina también la comparación anterior. No se añaden endpoints ni migraciones distintos de los ya incorporados en `main` por el PR #6.

## Datos y filtros

- Cantidades firmadas, precios, saldos y PnL se muestran como strings decimales originales. No se convierten a float ni se agregan monedas diferentes.
- Los registros con cantidad cero se conservan: no se cuentan como prueba de posición abierta ni como cierre confirmado. Un registro ausente tampoco crea un fill/cancelación/cierre.
- Búsqueda por símbolo y selector BOTH/LONG/SHORT filtran **posiciones y las dos familias de órdenes**, solo en memoria. Se aplica `positionSide`, no BUY/SELL: una compra de cierre de SHORT no desaparece al seleccionar SHORT. BOTH identifica el lado unidireccional, no una cobertura.
- Los filtros no alteran el snapshot guardado, los saldos ni el catálogo de instrumentos. Cada tabla informa cuántos registros coinciden de su total original. Un resultado filtrado vacío no se presenta como cuenta vacía.
- Posiciones, órdenes normales y condicionales se paginan por separado a 50 registros. Los controles muestran rango/conteo y permiten recorrer el detalle completo; no hay truncado silencioso ni peticiones por paginar. Cambiar filtros/cuenta/lectura reinicia las páginas.
- No se deduce pertenencia a una estrategia, protección, comisiones ni PnL realizado. Las órdenes normales y algo permanecen separadas y los IDs se mantienen como texto exacto.
- Los fills del reporte muestran la comisión/activo declarados, preservando el signo de las comisiones REST, sin sumarlos ni calcular PnL. Se paginan localmente de 50 en 50 dentro de los hasta 200 devueltos; los avisos de eventos/fills truncados permanecen visibles y paginar no recupera evidencia omitida. El reporte no es un ledger completo ni reconstruye lo perdido durante un hueco.

## Ciclo de vida y seguridad

No hay polling, carga automática al navegar ni fallback a ejemplos cuando falla la cuenta. Listar de nuevo borra selección/metadatos anteriores; cambiar cuenta, iniciar otra consulta, fallar o cambiar de origen elimina los datos financieros mostrados. Una respuesta 401 elimina también la lista de cuentas del panel.

El control inicial de lectura permanece deshabilitado en el HTML de servidor hasta hidratar sus handlers, para no perder clics tempranos. Una referencia síncrona evita iniciar dos peticiones simultáneas desde el mismo panel. Salir de la vista aborta la petición del navegador y descarta respuestas tardías. El navegador tiene un timeout de 22 segundos, además de los límites existentes web/motor. Abortar el cliente **no revoca una clave ni garantiza cancelar una lectura/guardado ya iniciado en el servidor**. Recuperar explícitamente la última lectura si el resultado fue ambiguo.

Los datos solo viven en el estado de la vista, no en localStorage/sessionStorage ni en parámetros de URL. Volver desde los ejemplos exige comprobar la cuenta de nuevo. Esto no sustituye las comprobaciones de sesión del servidor ni elimina la necesidad de proteger equipo, base y backups.

El panel de reconciliación se desmonta al cambiar de cuenta/fuente o iniciar una operación del controlador de lecturas/comparaciones: aborta sus peticiones y descarta resultados tardíos. Reporte y recuperación no pueden solicitarse simultáneamente desde ese panel. Editar la ventana retira el resultado de recuperación previo; los errores no muestran mensajes privados del exchange. La respuesta de recuperación debe coincidir con símbolo y ventana solicitados. Los controles de consulta de orden siguen separados.

## Alcance pendiente

Hay un reporte manual de eventos/fills deduplicados y recuperación REST acotada, no un monitor continuo ni un estado completamente reconciliado. Siguen pendientes automatizaciones TP/recompras/cobertura, recuperación automática de estados ambiguos y journal sincronizado. Bybit no está implementado. Esta vista no prueba que una cuenta real esté provisionada ni completa la versión 0.2: la activación operativa sigue requiriendo despliegue aprobado, permisos mínimos y salida fija. Ver [seguridad de lectura](binance-readonly.md), [plan](roadmap.md) y [mapa de los cuatro proyectos](integration-map.md).

Las pruebas de componentes y navegador usan respuestas sintéticas, incluyendo LONG/SHORT/BOTH, decimales de 18 posiciones, IDs superiores al entero seguro de JavaScript, filtros/paginación, huecos/truncamiento, ventanas UTC, respuestas discrepantes, errores, cambio de cuenta y cancelación del cliente. No contactan exchanges ni usan claves reales.
