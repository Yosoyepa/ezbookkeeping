# T001 — Fork de ezBookkeeping: recibos, extractos e insights con los CLI vía MCP

Fecha: 2026-11-03 · Arquitecto: Opus 5.5 · Estado: **plan, pendiente de aprobación**

## 0. Decisión de arquitectura

La IA **no corre dentro de la app**. La app guarda los datos y las fotos, y los expone por MCP. El análisis lo hacen
los CLI del responsable (Claude Code, Antigravity CLI `agy`, Codex) con **su suscripción**, sin API keys de pago en el servidor.

```
Celular (PWA)                    Servidor ezBookkeeping (Docker)           PC (CLI con suscripción)
  foto del recibo ──subida──▶   bandeja de recibos (pendiente)  ◀──MCP──   "procesa mis recibos"
  extracto PDF/CSV ─subida──▶   bandeja de documentos           ◀──MCP──   "importa el extracto de Bancolombia"
                                transacciones, categorías       ◀──MCP──   "analiza mis gastos de octubre"
```

### Lo que ya existe upstream (verificado en el código, v2.1.1-45, commit ec33570 del 2026-11-04)
- PWA móvil con `<input capture=camera>` en `AIImageRecognitionSheet.vue` y en `transactions/EditPage.vue`.
- Fotos adjuntas a transacciones (`pkg/models/transaction_picture_info.go`), incluidas las subidas sin vincular.
- Servidor MCP por HTTP en `/mcp` con token propio (`ezbookkeeping user-data ... --type mcp`), lista de IPs
  permitidas y protocolo hasta 2025-06-18. Tiene 7 herramientas: `add_transaction` (con `dry_run`), `query_transactions`,
  `query_all_accounts`, `query_all_accounts_balance`, `query_all_transaction_categories`,
  `query_all_transaction_tags` y `query_latest_exchange_rates`.
- El tipo `MCPImageContent` ya está definido en `pkg/mcp/handler.go`, pero ninguna herramienta lo usa todavía.
- Detector de duplicados (`pkg/duplicatechecker`) e importadores (csv, excel, ofx, qif, camt, fireflyIII, etc.).
- Skill `skills/ezbookkeeping/` con `ebktools.sh` / `ebktools.ps1` (cliente de la API).

### Lo que falta y construimos
1. **Bandeja de recibos**: la foto se sube sin crear la transacción y queda `pendiente`.
2. **Herramientas MCP nuevas**: `list_pending_receipts`, `get_receipt_image` (devuelve la imagen), `resolve_receipt`
   (crea la transacción, la vincula a la foto y la marca como procesada), `add_transactions_batch` (con dry_run y
   deduplicación), `update_transaction` (recategorizar) y `get_spending_summary` (agregados por mes, categoría y
   comercio, para no enviar miles de filas al modelo).
3. **Bandeja de documentos** para extractos bancarios: el CLI lee el PDF y llama a `add_transactions_batch`. Así
   **no hace falta escribir un parser por banco**: el modelo hace de parser y el servidor valida y deduplica.
4. **Paquete `ai-clients/`**: configuración MCP y comandos listos para cada CLI (Claude Code: skill y comandos;
   `agy`: `~/.gemini/config/mcp_config.json` (global, compartido con el IDE y la app Antigravity 2.0) o
   `.agents/mcp_config.json` del workspace, con el campo `serverUrl` y el token en `headers`, más una skill o workflow;
   Codex: `config.toml` y prompts). Los comandos serían
   `/recibos`, `/extracto`, `/insights` y `/recategorizar`.
5. **Pantalla "Bandeja"** en la PWA: ver qué está pendiente y qué se procesó, y deshacer.

## 1. Ítems críticos

| # | Ítem | Por qué no puede salir mal |
|---|---|---|
| C1 | Los CLI pueden **ver la imagen** del recibo | Es la base de toda la idea. Claude Code lo soporta, y según el responsable Codex también. Falta confirmar `agy` con la versión instalada |
| C2 | **Nada se registra dos veces ni se inventa** | Un modelo puede alucinar montos o reimportar el mismo extracto. Las finanzas dejan de ser fiables |
| C3 | **Seguridad del token MCP y de la red** | El token da acceso de escritura a todas tus finanzas. El celular necesita llegar al servidor sin exponerlo a internet |
| C4 | **Fork mantenible** | Upstream está muy activo (commits diarios). Si tocamos mucho su código, cada rebase será un dolor |
| C5 | **Montos y fechas en COP y formato colombiano** | `1.234.567,89`, sin centavos en la práctica, fechas `dd/mm/aaaa`. Un error de separador multiplica por 100 |
| C6 | **Captura cómoda en el celular** | Si subir un recibo cuesta más de 3 toques, no se usa |
| C7 | **Funciona con los 3 CLI** (o se documenta cuál no) | La gracia es no depender de un proveedor |

## 2. Definición de hecho por ítem

**C1 — Imagen visible en el CLI (spike, fase 0)**
- [ ] Herramienta mínima `get_receipt_image` devuelve `MCPImageContent` (base64, `image/jpeg`).
- [ ] Con una foto de prueba (`testdata/receipts/exito_01.jpg`), cada CLI responde el total correcto:
      Claude Code ☐ · `agy` ☐ (anotar la versión) · Codex ☐ (anotar la versión).
- [ ] Si algún CLI no ve la imagen, se aplica el **plan B** documentado: la herramienta devuelve una URL firmada de corta
      duración, el comando del CLI la descarga a un archivo temporal y el CLI lee el archivo local. Se prueba igual.

**C2 — Integridad**
- [ ] `resolve_receipt` y `add_transactions_batch` exigen un `dry_run` previo: el CLI muestra la tabla y el
      responsable confirma. Los comandos de `ai-clients/` lo imponen en su texto.
- [ ] La deduplicación por (cuenta, fecha ±1 día, monto, descripción normalizada) rechaza un reimport.
      Prueba: `go test ./pkg/mcp/... -run Batch` importa el mismo extracto dos veces y la segunda vez crea **0** transacciones.
- [ ] Cada transacción creada por IA lleva la etiqueta `ia` y un comentario con el origen (id del recibo o del extracto):
      se puede filtrar y revertir.
- [ ] Un recibo procesado guarda la foto vinculada a la transacción: verificado en la PWA.

**C3 — Seguridad** *(gate antes de exponer el servidor fuera de la PC)*
- [ ] `enable_mcp = true` con `mcp_allowed_remote_ips` limitado (localhost y la IP de Tailscale de la PC).
- [ ] El token MCP vive en el almacén de secretos de cada CLI o en variables de entorno, **nunca** en el repo.
      Comprobación: `git grep -nE 'ebk_|EBKTOOL_TOKEN=.+'` devuelve 0 coincidencias.
- [ ] Acceso desde el celular solo por Tailscale (o la VPN que elija el responsable): sin puertos abiertos en el router.

**C4 — Mantenibilidad**
- [ ] El código nuevo vive en archivos nuevos (`pkg/mcp/*_receipt_*`, `pkg/receipts/`, `src/views/mobile/inbox/`, etc.). Los cambios
      a archivos de upstream se limitan al registro de herramientas, rutas, migraciones y menú: `git diff upstream/main --stat`
      muestra **menos de 15 archivos de upstream modificados**.
- [ ] `go test ./...` y `npm run test` pasan.
- [ ] Un rebase de prueba sobre `upstream/main` del día del cierre aplica sin conflictos, o con conflictos documentados.

**C5 — Formato colombiano**
- [ ] Pruebas con montos `1.234.567`, `1.234.567,89`, `$ 45.900` y `45,900.00` → el valor en COP es correcto (tabla de casos en
      `pkg/receipts/amount_parse_test.go` y en el texto de los comandos).
- [ ] Extracto de prueba de 2 bancos (Bancolombia, Nequi o Davivienda; los elige el responsable) importado con el 100 % de las filas
      cuadrando con el saldo del extracto.

**C6 — Captura móvil**
- [ ] Desde la pantalla de inicio de la PWA: botón "Recibo" → cámara → subir = **2 toques** + disparo.
- [ ] Varias fotos seguidas (recibo largo o varios recibos) en una sola sesión.
- [ ] Probado en el celular real del responsable (Android/iOS: por confirmar) a través de la VPN.

**C7 — Tres CLI**
- [ ] `ai-clients/README.md` con la instalación en ≤ 5 pasos por CLI.
- [ ] Prueba ciega: un worker Haiku sin contexto, solo con el README, procesa 3 recibos y 1 extracto con Claude Code y
      escribe un apartado de "Fricción".

## 3. Plan de delegación

| Fase | Qué | Quién | Modelo / esfuerzo | Entrega | Rutas que posee |
|---|---|---|---|---|---|
| 0 | Fork, upstream remoto y build local en Docker | Arquitecto | Opus 5.5 | Repo listo y `docker compose up` | raíz |
| 0 | **Spike C1**: `get_receipt_image` mínimo y prueba en los 3 CLI | `w-spike-mcp` | Sonnet 5.5 · high | Informe con la matriz CLI × resultado | `pkg/mcp/get_receipt_image*` |
| 1 | Bandeja de recibos (modelo, migración, API y herramientas MCP de recibos) | `w-receipts-backend` | Sonnet 5.5 · high | Código y pruebas Go | `pkg/receipts/`, `pkg/mcp/*receipt*`, migraciones |
| 1 | Pantalla de bandeja y botón de captura en la PWA | `w-inbox-pwa` | Haiku 5.5 · max (→ Sonnet si se atasca) | Vue y pruebas | `src/views/mobile/inbox/`, `src/components/mobile/Receipt*` |
| 2 | `add_transactions_batch`, deduplicación, `update_transaction` y `get_spending_summary` | `w-mcp-batch` | Sonnet 5.5 · high | Código y pruebas Go | `pkg/mcp/*batch*`, `*summary*`, `*update*` |
| 2 | Paquete `ai-clients/` (configuración y comandos de los 3 CLI) y casos COP | `w-ai-clients` | Haiku 5.5 · max | Archivos y README | `ai-clients/` |
| 3 | Revisión independiente del diff | Codex `gpt-6.1-sol` · xhigh (Orca visible) | — | Hallazgos | solo lectura |
| 3 | Prueba ciega C7 | `w-blind` | Haiku 5.5 · max | Informe de fricción | solo lectura |
| 3 | Correcciones | código → Sonnet, documentación → arquitecto | — | Commits | según el hallazgo |

Las fases 1 y 2 empiezan **solo si el spike C1 pasa** (o queda el plan B aprobado).

## 4. Decisiones abiertas del responsable
1. ~~Fork público o privado~~ → **público**: https://github.com/Yosoyepa/ezbookkeeping (decidido el 2026-11-03).
2. **Dónde corre el servidor**: en tu PC con Docker (lo más simple), en un mini-PC/NAS o en un VPS.
3. **Red del celular**: Tailscale (recomendado) u otra opción.
4. **Bancos para el importador de prueba (C5)** y si el celular es Android o iOS.
5. ~~Carpeta local~~ → `Documents/Proyectos Personales/Finanzas/`.

## Cierre
_(se rellena al terminar: por ítem, cumplido / no cumplido / aceptado por alcance, con la evidencia)_
