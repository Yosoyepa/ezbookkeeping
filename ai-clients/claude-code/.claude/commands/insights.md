---
description: "Analiza los gastos de un periodo con agregados del servidor: top de categorías, variación, recurrentes, anomalías y recomendaciones"
argument-hint: "[periodo, p. ej. octubre 2026]"
---

Ejecuta el comando /insights siguiendo la sección `## /insights [periodo]` del bloque compartido, después de aplicar las `## Reglas comunes`. Periodo del usuario (vacío = mes en curso): $ARGUMENTS

<!-- BEGIN SHARED: shared/finanzas-workflows.md (copia generada por scripts/sync-workflows; no editar aquí) -->
# Finanzas: reglas y flujo de los comandos

Fuente única de las reglas de negocio para los tres CLI (Claude Code, Antigravity `agy` y Codex). Los archivos de cada CLI contienen una copia de este documento entre las marcas `BEGIN SHARED` y `END SHARED`. **No edites esas copias:** cambia este archivo y ejecuta `scripts/sync-workflows.ps1` o `scripts/sync-workflows.sh`.

El servidor de datos es ezBookkeeping. Sus herramientas MCP aparecen con el nombre del servidor `ebk`. La moneda por defecto es COP (pesos colombianos).

## Reglas comunes

1. **Primero el catálogo.** Antes de crear o cambiar una transacción, llama a `query_all_accounts` y a `query_all_transaction_categories`. Nunca inventes una cuenta ni una categoría. Si no hay una adecuada, pregunta al usuario cuál usar. Las cuentas y categorías nuevas se crean en la app, no desde aquí.
2. **Dos pasos siempre.** Las herramientas que escriben (`resolve_inbox_item`, `add_transactions_batch`, `update_transaction`) se llaman primero con `dry_run=true`. Muestra el resultado en una tabla y espera una confirmación explícita del usuario. Solo entonces repite la llamada con `dry_run=false`. Si el usuario pide cambios, vuelve a `dry_run=true`.
3. **Fechas.** El usuario escribe fechas como `dd/mm/aaaa`. Conviértelas a RFC 3339 con la zona horaria de Colombia (`-05:00`) y a mediodía, por ejemplo `15/10/2026` → `2026-10-15T12:00:00-05:00`. Si una fecha no se lee con certeza, pregunta.
4. **Montos en COP.** Pasa el monto a las herramientas tal como lo escribió el usuario, como texto. El servidor lo interpreta y devuelve `normalized_amount`, que es el valor que debes mostrar en la tabla. Esta es la referencia del formato colombiano:
   - `1.234.567` → 1234567
   - `1.234.567,89` → 1234567.89
   - `45.900` → 45900
   - `12.34` → 12.34
   - `12,34` → 12.34
   - `1,234` → 1234 (coma seguida de tres dígitos = miles)
   - `45,900.00` → 45900

   Si el monto no es claro, pregunta. Nunca lo adivines. Si el servidor responde un error de monto, pide al usuario que lo reenvíe sin separadores de miles.
5. **Origen trazable.** Toda transacción creada por estas herramientas lleva la etiqueta `ia` y un comentario que termina en `[origen: ...]`. No quites ni cambies esa etiqueta.
6. **Duplicados.** Una fila marcada como `duplicate` no se fuerza. Muéstrala al usuario y deja que decida.
7. **Lo que no se lee, se pregunta.** Si un campo es ilegible (fecha, total, comercio o cuenta), pregúntalo. Nunca lo rellenes con un valor plausible.
8. **Secretos.** Nunca muestres, copies ni guardes el token MCP. Las herramientas se llaman a través del servidor `ebk`, que ya tiene configurado el token.

## Herramientas MCP que usan los comandos

| Herramienta | Para qué |
|---|---|
| `query_all_accounts` | Catálogo de cuentas. Siempre antes de escribir. |
| `query_all_transaction_categories` | Catálogo de categorías. Siempre antes de escribir. |
| `query_transactions` | Buscar transacciones: `start_time`, `end_time` (RFC 3339), `type`, `category_name`, `account_name`, `keyword`, `count` (50 o menos) y `page`. |
| `list_inbox_items` | Ver la bandeja: `status` (pending), `kind` (1 = recibo, 2 = extracto) y `limit`. |
| `get_inbox_item` | Abrir un ítem. Recibo: imagen y metadatos. Extracto CSV, TXT, OFX o QIF: texto. Extracto PDF o XLSX: archivo incrustado y `download_url` válido 15 minutos. |
| `resolve_inbox_item` | Convertir un recibo en transacciones (1 a 50 filas) y marcarlo como procesado. |
| `discard_inbox_item` | Descartar un ítem con un `reason`. |
| `add_transactions_batch` | Crear transacciones en lote (1 a 500 filas por llamada) con `source_label`. |
| `update_transaction` | Cambiar `category_name`, `tags` (reemplaza todas), `comment`, `amount`, `time` o `account_name` de una transacción. |
| `get_spending_summary` | Totales agregados entre `start_time` y `end_time`, con `type` (expense o income), `group_by` (category, month, account, tag o comment_merchant) y `top`. |

Cada fila (`TxRow`) tiene `type` (expense, income o transfer), `time` (RFC 3339), `category_name`, `account_name`, `amount` y, si hace falta, `destination_account_name`, `destination_amount`, `tags` y `comment`.

## /recibos

Procesa los recibos pendientes de la bandeja. Cada recibo es una foto (`kind` 1).

1. Llama a `query_all_accounts` y a `query_all_transaction_categories`.
2. Llama a `list_inbox_items` con `status` pending y `kind` 1. Si el usuario indicó un id, usa `get_inbox_item` con ese id. Si no hay ningún recibo pendiente, dilo y termina.
3. Por cada recibo, llama a `get_inbox_item` y lee la imagen: fecha, comercio, total, y los impuestos o la propina si aparecen.
4. Propón las filas: `type` expense, `time`, la cuenta (pregúntala si no es clara), la categoría del catálogo, `amount` tal como aparece en el total, y `comment` con el comercio.
5. Si el recibo tiene partes con categorías distintas, propón varias filas. La foto queda vinculada a la primera transacción creada.
6. Llama a `resolve_inbox_item` con `dry_run=true` y muestra una tabla con fila, fecha, comercio, categoría, cuenta y monto normalizado. Pide confirmación.
7. Con la confirmación, repite la llamada con `dry_run=false`.
8. Si el recibo no sirve (no es un gasto, es ilegible o es un duplicado evidente), propón `discard_inbox_item` con un motivo y espera confirmación.
9. Cierra con un resumen: recibos procesados, transacciones creadas, descartados y pendientes que quedan.

## /extracto <ruta|inbox>

Importa las filas de un extracto bancario. El modelo hace de analizador, así que no hace falta un parser por banco. Las cifras tienen que cuadrar.

1. **Origen.** Si el argumento es una ruta local, lee el archivo con las herramientas del propio CLI. Si es `inbox` o un id, usa `list_inbox_items` con `kind` 2 y luego `get_inbox_item`.
2. Llama a `query_all_accounts` y a `query_all_transaction_categories`.
3. Extrae todas las filas: fecha, descripción, monto con su signo o su columna de débito o crédito, y el saldo inicial y final o el total que trae el extracto. No inventes filas. Si una fila no se lee, pregúntala.
4. Pregunta lo que falte: la cuenta destino y el `source_label`, por ejemplo `bancolombia-2026-10`. Si el origen es la bandeja, el `source_label` es `inbox:<id>`.
5. Asigna una categoría del catálogo a cada fila. Si varias filas no tienen categoría clara, pregunta por grupos y no fila por fila.
6. Llama a `add_transactions_batch` con `dry_run=true`. Muestra el resumen (creadas, duplicadas y errores) y la tabla por fila. Si hay más de 500 filas, divídelas en lotes con el mismo `source_label`.
7. **Cuadre.** La suma firmada de las filas (ingresos positivos y gastos negativos) debe igualar la variación de saldo del extracto, es decir, saldo final menos saldo inicial, o el total que indique el documento. Si no cuadra, muestra la diferencia y no confirmes hasta que el usuario decida.
8. Con la confirmación, repite la llamada con `dry_run=false`.
9. Esta herramienta no marca el ítem de la bandeja como procesado ni como descartado. Deja el ítem como está y avísale al usuario.
10. Cierra con filas creadas, duplicadas omitidas, errores y la diferencia de cuadre, si la hay.

## /insights [periodo]

Analiza los gastos con agregados. No descargues todas las transacciones.

1. **Periodo.** Sin argumento, usa el mes en curso. Con argumento, interpreta el periodo, por ejemplo `octubre 2026` → del 01/10/2026 al 31/10/2026. Calcula también el periodo anterior de la misma duración.
2. Llama a `get_spending_summary` con `type` expense: con `group_by` category para cada periodo, con `group_by` month para la tendencia y con `group_by` comment_merchant para los comercios recurrentes.
3. Entrega, en este orden:
   - a. Top de categorías: total, número de transacciones y participación.
   - b. Variación frente al periodo anterior, en COP y en porcentaje.
   - c. Gastos recurrentes o suscripciones: comercios que aparecen en varios meses con montos parecidos.
   - d. Anomalías: categorías o comercios que suben mucho frente a su promedio.
   - e. Tres recomendaciones accionables, cada una con su cifra.
4. Si necesitas transacciones concretas para justificar una anomalía, usa `query_transactions` con `count` de 50 o menos.
5. Cada cifra sale de una herramienta. Indica el periodo y que la moneda es COP. Si un dato no está disponible, dilo.

## /recategorizar [filtro]

Corrige categorías en lote, con confirmación.

1. Llama a `query_all_transaction_categories` para conocer las categorías válidas.
2. Busca con `query_transactions` según el filtro del usuario: periodo, cuenta, palabra clave o categoría actual. Usa `count` de 50 o menos y pagina si hace falta. Si el filtro es demasiado amplio, pide que lo acoten.
3. Propón una categoría por transacción, usando solo el catálogo. Si ninguna encaja, pregunta.
4. Muestra la propuesta en una tabla con fecha, comentario, monto, categoría actual y categoría propuesta.
5. Por cada cambio aprobado, llama a `update_transaction` con `dry_run=true`. Cambia solo `category_name`, salvo que el usuario pida otra cosa. `tags` reemplaza todas las etiquetas, así que no lo uses sin leer antes las actuales.
6. Con la confirmación final, repite cada llamada con `dry_run=false`.
7. Cierra con cuántas transacciones cambiaron y cuáles quedaron sin tocar.
<!-- END SHARED -->
