# SPEC T001 — Contratos de la bandeja, las herramientas MCP y la PWA

Este documento es la fuente de verdad que comparten los workers. Si algo del código upstream lo contradice, gana el
upstream: el worker lo anota en su informe y se adapta, sin romper el contrato hacia los demás.
Contexto: `docs/tasks/T001_fork-ezbookkeeping-mcp.md`.

## 1. Modelo `InboxItem` (`pkg/models/inbox_item.go`)

```go
type InboxItemKind byte   // 1 = receipt (imagen), 2 = statement (pdf/csv/xlsx/txt)
type InboxItemStatus byte // 1 = pending, 2 = processed, 3 = discarded

type InboxItem struct {
    Uid                  int64  `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
    Deleted              bool   `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
    Status               InboxItemStatus `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created) NOT NULL"`
    ItemId               int64  `xorm:"PK"`
    Kind                 InboxItemKind `xorm:"NOT NULL"`
    PictureId            int64  // receipts: picture de TransactionPictureService (TransactionId = 0 hasta resolverse)
    FileExtension        string `xorm:"VARCHAR(10) NOT NULL"`
    OriginalFileName     string `xorm:"VARCHAR(255)"`
    Note                 string `xorm:"VARCHAR(255)"`
    ResultTransactionIds string `xorm:"VARCHAR(2048)"` // ids separados por coma
    ProcessedNote        string `xorm:"VARCHAR(1024)"` // resumen o motivo del descarte
    CreatedIp            string `xorm:"VARCHAR(39)"`
    CreatedUnixTime      int64  `xorm:"INDEX(IDX_inbox_item_uid_deleted_status_created)"`
    UpdatedUnixTime      int64
    ProcessedUnixTime    int64
    DeletedUnixTime      int64
}
```
- Tipo de UUID nuevo: `UUID_TYPE_INBOX_ITEM UuidType = 13` en `pkg/uuid/uuid_type.go`.
- Sincronización: `cmd/database.go` → `UserDataStore.SyncStructs(new(models.InboxItem))`.
- **Recibos**: la imagen se guarda con `TransactionPictureService.UploadPicture` (TransactionId = 0). El ítem
  guarda `PictureId`. Al resolverse, la foto se vincula a la **primera** transacción creada.
  La PWA **nunca** llama a `remove_unused` sobre una foto que pertenece a un ítem de la bandeja.
- **Extractos**: se guardan con el `ObjectStorage` de upstream en `inbox_documents/<uid>/<itemId>.<ext>`.
  Máximo 20 MB, con extensiones `pdf, csv, xlsx, xls, txt, ofx, qif`.

## 2. API REST para la PWA (sesión normal, prefijo `/api/v1`)

Respuestas con el envoltorio estándar de upstream `{success, result}`. Los ids son strings (`json:",string"`).

| Método y ruta | Entrada | Resultado |
|---|---|---|
| `POST /inbox/items/upload.json` | multipart: `file`, `kind` (1/2), `note?` | `InboxItemInfoResponse` |
| `GET /inbox/items/list.json` | `status?` (0 = todos), `kind?`, `count` (≤ 50), `page` (≥ 1) | `{items: InboxItemInfoResponse[], totalCount}` |
| `GET /inbox/items/get.json` | `id` | `InboxItemInfoResponse` |
| `GET /inbox/items/stats.json` | — | `{pendingReceipts, pendingStatements}` |
| `POST /inbox/items/discard.json` | `{id, note?}` | `true` |
| `POST /inbox/items/reopen.json` | `{id}` | `true` (vuelve a pending; **no** borra transacciones ya creadas) |
| `POST /inbox/items/delete.json` | `{id}` | `true` (solo si está pending o discarded; borra la foto o el archivo) |
| `GET /inbox/items/file/:id` | token de sesión como en las fotos de upstream | bytes del archivo (para descargar el extracto) |

```ts
interface InboxItemInfoResponse {
  id: string; kind: 1 | 2; status: 1 | 2 | 3;
  pictureId?: string; pictureUrl?: string;   // receipts: misma URL que una foto de transacción
  fileExtension: string; originalFileName?: string; note?: string;
  resultTransactionIds: string[]; processedNote?: string;
  createdTime: number; processedTime?: number; // unix
}
```

## 3. Herramientas MCP nuevas (registradas como las de upstream)

Reglas comunes:
- Los montos llegan como **string** y se interpretan con `ParseLocalizedAmount` (§4). Se responde siempre con
  el monto normalizado (`"45900.00"`) para que el CLI lo muestre.
- Toda transacción creada por estas herramientas lleva la etiqueta **`ia`** (se crea si no existe) y un comentario
  terminado en ` [origen: <source>]`, donde `<source>` es `inbox:<id>` o el `source_label` del lote.
- Todas las herramientas que escriben aceptan **`dry_run`**. Su descripción dice: *"Call first with dry_run=true, show the
  result to the user and only call again with dry_run=false after explicit confirmation."*

| Herramienta | Entrada | Salida |
|---|---|---|
| `list_inbox_items` | `status?` = pending, `kind?`, `limit?` = 20 | `[{id, kind, status, created_time, note, original_file_name}]` |
| `get_inbox_item` | `id` | receipt → `MCPImageContent` + texto con los metadatos. statement csv/txt/ofx/qif → texto. pdf/xlsx → `MCPEmbeddedResource` (blob base64) **y** `download_url` de corta duración (≤ 15 min) |
| `resolve_inbox_item` | `id`, `transactions: TxRow[]` (1..50), `note?`, `dry_run` | resultado por fila, como en el lote. Si no es dry_run y se creó ≥ 1 transacción: el ítem pasa a processed, la foto queda vinculada y se guardan los ids |
| `discard_inbox_item` | `id`, `reason` | ok |
| `add_transactions_batch` | `transactions: TxRow[]` (1..500), `source_label` (obligatorio, p. ej. `"bancolombia-2026-10"`), `skip_duplicates?` = true, `inbox_item_id?` (si viene y no es dry_run: el extracto de la bandeja pasa a processed con los ids creados y un resumen en `processed_note`), `dry_run` | `{created, duplicates, errors, rows:[{index, status: created\|would_create\|duplicate\|error, transaction_id?, duplicate_of?, message?, normalized_amount}]}` |
| `update_transaction` | `id`, `category_name?`, `tags?` (reemplaza), `comment?`, `amount?`, `time?`, `account_name?`, `dry_run` | transacción antes y después |
| `get_spending_summary` | `start_time`, `end_time` (RFC 3339), `type` (expense\|income), `group_by` (category\|month\|account\|tag\|comment_merchant), `currency?`, `top?` = 20 | `[{key, total, count, share}]` + `{grand_total, currency, period}` |

`TxRow` = los mismos campos que `MCPAddTransactionRequest` de upstream (`type, time, category_name, account_name, amount,
destination_account_name?, destination_amount?, tags?, comment?`), con dos cambios: no lleva `dry_run` y `amount` se interpreta con §4.

**Duplicados** (`pkg/aiimport/dedupe.go`): se considera duplicada una fila con el mismo uid, cuenta, tipo y monto, a ≤ 1 día de una
transacción existente y con comentario normalizado igual o contenido en el otro. Para normalizar: minúsculas, sin tildes,
solo `[a-z0-9]` y sin el sufijo `[origen: …]`. Las filas repetidas dentro del mismo lote también cuentan como duplicadas.

`comment_merchant` agrupa por el comentario normalizado, sin el sufijo de origen y truncado a 40 caracteres.

## 4. `ParseLocalizedAmount` (`pkg/aiimport/amount.go`)

Acepta `$`, `COP`, espacios y signo. Reglas:
- `1.234.567` → 1234567. `1.234.567,89` → 1234567.89. `45.900` → 45900. `45,900.00` → 45900.
- `12.34` → 12.34 (un solo punto seguido de 2 dígitos = decimal). `12,34` → 12.34. `1,234` → 1234 (coma seguida de 3 dígitos = miles).
- Si no está claro, devuelve un error con un mensaje que pide reenviar el monto sin separadores de miles.
Pruebas con tabla de casos en `pkg/aiimport/amount_test.go` (≥ 20 casos).

## 5. PWA (Vue + Framework7 móvil; Vuetify escritorio)

- **Inicio móvil** (`src/views/mobile/HomePage.vue`): botón **"Recibo"** que abre la cámara (`capture=camera`,
  `multiple`), sube cada foto como receipt y muestra un toast "N recibos en la bandeja". Al lado, un acceso a la
  **Bandeja** con un badge que muestra `pendingReceipts + pendingStatements`.
- **Página `/inbox`** móvil (`src/views/mobile/inbox/ListPage.vue`): pestañas Pendientes / Procesados / Descartados,
  miniatura (recibos) o icono y nombre (extractos), nota, fecha, y acciones descartar, reabrir y borrar. Si el ítem ya se procesó, enlace a
  sus transacciones. Botón "Subir extracto" (input file sin capture).
- **Escritorio**: página `/inbox` equivalente, más sencilla (tabla), y entrada en el menú lateral.
- Servicios en `src/lib/services.ts`, tipos en `src/models/inbox.ts`, store `src/stores/inbox.ts`.
- Textos en `en.json` y `es.json` (los demás idiomas caen al inglés).
- Pantalla vacía de Pendientes: "Procesa tus recibos desde tu CLI con /recibos".

## 6. Paquete `ai-clients/`

```
ai-clients/
  README.md                        (español: requisitos, token MCP, Tailscale, instalación por CLI en ≤ 5 pasos)
  shared/finanzas-workflows.md     (fuente única: reglas de negocio y flujo de los 4 comandos)
  claude-code/.mcp.json.example    (+ skill finanzas/ con SKILL.md + commands/recibos.md, extracto.md, insights.md, recategorizar.md)
  antigravity/mcp_config.json.example (+ .agents/skills|workflows equivalentes)
  codex/config.toml.example        (+ prompts/*.md)
  scripts/check-mcp.ps1 / .sh      (initialize + tools/list contra el servidor; sale ≠ 0 si falta alguna herramienta)
```
Reglas que imponen los 4 comandos:
1. Primero `query_all_accounts` y `query_all_transaction_categories`. Nunca se inventan cuentas ni categorías: si no hay una
   adecuada, se le pregunta al usuario.
2. `dry_run=true` → tabla → confirmación explícita → `dry_run=false`.
3. Fechas `dd/mm/aaaa` y montos COP según §4. Si un campo no se lee, se pregunta; nunca se adivina.
4. `/extracto <ruta|inbox>`: el CLI lee el archivo local (o `get_inbox_item`), extrae las filas y usa `add_transactions_batch` con
   `source_label`. Al final, la suma de las filas debe cuadrar con el total o saldo del extracto, o se avisa la diferencia.
5. `/insights [periodo]`: `get_spending_summary` (no se descargan todas las transacciones). Entrega el top de categorías, la variación
   frente al periodo anterior, gastos recurrentes o suscripciones, anomalías y 3 recomendaciones accionables.
6. `/recategorizar [filtro]`: `query_transactions` → propuesta → `update_transaction` con dry_run.
