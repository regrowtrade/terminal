# Reglas de la versión 0.1

## Unidades y presupuesto

Los contratos usan decimales como strings, nunca floats JSON. Entrada máxima: 16 dígitos enteros y 12 fraccionarios; las divisiones calculadas usan 32 fraccionarios. Los precios no están cuantizados a un instrumento real.

`riskUnit=amount`: el riesgo introducido es USDT. `riskUnit=percent`: riesgo monetario = referencia de cuenta × porcentaje / 100. La referencia positiva y su base `equity|wallet` son manuales y se devuelven en el resultado. No cambian después de calcular.

`costReserve` representa costes futuros estimados para el escenario completo; no es una comisión por orden ni un coste ya pagado. Debe ser no negativo y menor que el riesgo. El usuario debe estimarlo de nuevo si cambia la grilla. La versión no estima automáticamente comisiones ni deslizamiento.

## Entrada limpia

Una entrada. Cantidad Q, precio E, riesgo R, relación B y reserva C:

- LONG: SL = E − (R − C)/Q; TP = E + (R×B + C)/Q.
- SHORT: SL = E + (R − C)/Q; TP = E − (R×B + C)/Q.

Esto reserva C dentro de la pérdida prevista y añade C al objetivo de beneficio bruto. El apalancamiento únicamente modifica el margen estimado.

## Recompras

- Secuencia Q, Q, 2Q, 4Q…; entrada incluida en el contador.
- Distancia d: LONG P[n+1]=P[n]×(1−d/100); SHORT P[n+1]=P[n]×(1+d/100).
- Precio medio ponderado y cantidades acumuladas por cada candidato.
- SL y TP calculados con el presupuesto total de ese escenario.
- Se detiene al superar el margen manual, producir precios no positivos, cruzar el SL del candidato o alcanzar el máximo solicitado (hasta 11).

Replica la secuencia del caso de incremento del 100 % del Go antiguo, con reserva y límite de margen adicionales. No replica el incremento de volumen arbitrario del proyecto antiguo. El riesgo de liquidación y la transición del SL entre ejecuciones requieren un futuro motor de ejecución; **esta entrega no coloca una grilla real**.

## Cobertura

Distancia = abs(E_long−E_short)/max(E_long,E_short)×100.

- Cierre inicial 5 %: separación = distancia×1,50.
- Cierre inicial 10 %: separación = distancia×1,20.
- El porcentaje se aplica a las monedas del lado seleccionado en la fotografía inicial; secuencia Q, Q, 2Q, 4Q…
- SHORT: niveles descendentes; TP LONG por encima del promedio ponderado de cierres.
- LONG: niveles ascendentes; TP SHORT por debajo del promedio ponderado de cierres.
- Cantidad del TP = menor entre cierres acumulados y monedas contrarias restantes en el escenario. No representa seguimiento de TP ya ejecutados.

Resultado neto(P) = realizado − comisiones + funding + Q_long×(P−E_long) + Q_short×(E_short−P).

La fotografía incluye TODOS los cierres de la campaña. Flotante por sí solo no es recuperación neta.

Al activar: referencia = resultado neto(mark); umbral = referencia − adicional. Un nuevo cálculo utiliza la nueva fotografía; no acumula adicionales previos. Dentro de una gestión, mejorar el PnL no implica trailing automático del umbral.

Para cada escenario se actualiza realizado/exposición, y el SL resuelve resultado neto(SL)−reserva=umbral. Si no hay exposición direccional neta, se devuelve SL nulo, nunca una división por cero. Un escenario futuro incompatible se excluye; si el primero no permite protección, se rechaza el plan.

La función pura `Exposure.Close` admite parciales y deduce comisiones de esa ejecución. Está probada, pero no conectada todavía a eventos de exchange.

## Límites no negociables

Una vista previa no es una orden. Un ciclo de compensación no equivale a cerrar todo el activo. Un stop de una pierna no equivale a un cierre atómico de ambas. Un SL no garantiza una pérdida exacta. Los próximos adaptadores deben confirmar aceptación, ejecución, cantidades residuales y cancelación de órdenes normales y condicionales.
