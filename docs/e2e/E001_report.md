# E001 — Prueba E2E del producto completo (servidor real + CLI reales + PWA)

Fecha de la corrida: 2026-11-03 (la bitácora del servidor está en UTC y cruza al 2026-11-04). Worker: `w-e2e`.
Rama probada: `Yosoyepa/w-e2e` @ `e6a1f06a` (base `feat/finanzas-ia`). Servidor: ezBookkeeping construido desde un clon temporal en `%TEMP%\ebk-e2e-src`, publicado en `127.0.0.1:8080`.
Evidencia: `docs/e2e/evidence/` (salidas sin tokens, capturas PNG). Todas las cifras de este informe salen de la API o de las salidas guardadas ahí.

## 1. Resumen

El flujo principal funciona de punta a punta con los tres CLI: recibos, extractos PDF y CSV, insights y recategorización. Los montos quedan exactos, los cuadres dan diferencia 0, la reimportación crea 0 transacciones, cada transacción lleva la etiqueta `ia` y su origen, y la foto queda vinculada al recibo.

No está listo para uso real por dos defectos bloqueantes:

1. **La PWA sube la misma foto en bucle** y deja la hoja abierta. En 28 s creó 722 recibos duplicados (BUG-01).
2. **El CSV de la bandeja no llega a Claude Code**, así que `/extracto <id>` no puede leerlo (BUG-02).

Además, C3 (seguridad) y C4 (mantenibilidad) no cumplen los criterios tal como están escritos. C6 falla por el bucle de la PWA.

## 2. Entorno

| Componente | Versión / detalle |
|---|---|
| Host | Windows 11 Pro 10.0.26300, PowerShell 5.1 |
| Docker Desktop (servidor) | 28.1.1 |
| Imagen | `ezbookkeeping-ai:local`, construida desde el clon de `e6a1f06a` |
| Go / Node (contenedores) | `golang:1.27.3-alpine3.24` (pruebas del build), `node:26.13.0-alpine3.24` (`npm test`) |
| Claude Code | 2.1.306, modelo `claude-haiku-5-5` |
| Antigravity CLI (`agy`) | 1.4.12 (el README probó 1.4.4) |
| Codex CLI | 0.163.0, modelo `gpt-6-luna` (la cuenta no admite `gpt-6.1-sol`) |
| Python | 3.12.2 con PIL 10.2.0 y reportlab 4.4.5, siempre con `python -I` |
| Navegador de la PWA | Google Chrome del sistema con `playwright-core`; viewport 390×844, `isMobile`, `es-CO` |

Puerto 8080 libre en el momento del arranque. El servidor se publica solo en `127.0.0.1` (`compose_ps_y_puertos.txt`).

## 3. Criterios C1–C7

| Criterio | Veredicto | Evidencia (comando y salida recortada) |
|---|---|---|
| **C1** Los CLI ven la imagen | **Pasa** | Claude: `get_inbox_item` devuelve la imagen y el modelo lee 128.350 / 45.900 / 1.234.567 (`recibos_run2_ok.txt`). `agy` abre `media_0.jpg` y propone 23.400 (`agy_recibos_run13.txt`). Codex lee "Ferretería La Tuerca … $87.200" (`codex_recibos_run15_ok.txt`). No hizo falta el plan B. |
| **C2** Integridad | **Falla (parcial)** | Pasa en transacciones: `dry_run` antes de cada escritura en las tres CLI; reimportación PDF = 12/12 duplicadas, 0 creadas (`reimport_pdf_run9.txt`); CSV = 8/8, 0 creadas (`reimport_csv_run10.txt`); `ia` en 25 de 25 transacciones de IA; origen `[origen: …]` en 25 de 25; foto vinculada en 5 de 5 recibos (`verificacion_api_final.txt`). **Falla** en la bandeja: el bucle de la PWA creó 722 recibos duplicados (BUG-01). |
| **C3** Seguridad | **Falla (parcial)** | `curl /mcp` sin token → **400** `token is empty`, no 401/403 (BUG-07). Token inválido → 401. Usuario 2: `list_inbox_items` = 0 ítems, `query_all_accounts` = {}, `get_inbox_item` del ítem del usuario 1 = `inbox item not found` (`seguridad_c3.txt`). Solo `127.0.0.1` publicado (`compose_ps_y_puertos.txt`). **No cumple:** `mcp_allowed_remote_ips` quedó con la lista por defecto (`127.0.0.1,172.*.*.*,192.168.65.*`), no limitada a localhost y a la IP de Tailscale. Acceso por Tailscale y celular: **no probado**. Secretos en el repo: ver §7. |
| **C4** Mantenibilidad | **Falla** | Archivos de upstream modificados: **18** (criterio: menos de 15). Lista: `cmd/database.go`, `cmd/webserver.go`, `pkg/api/model_context_protocols.go`, `pkg/core/mcp_tool.go`, `pkg/mcp/handler.go`, `pkg/mcp/mcp_container.go`, `pkg/mcp/model_context_protocol.go`, `pkg/mcp/query_transactions_tool_handler.go`, `pkg/uuid/uuid_type.go`, `src/components/desktop/MainPageLayout.vue`, `src/consts/api.ts`, `src/consts/mcp.ts`, `src/lib/services.ts`, `src/locales/en.json`, `src/locales/es.json`, `src/router/desktop.ts`, `src/router/mobile.ts`, `src/views/mobile/HomePage.vue`. Pasa: `go test` (43 paquetes `ok`, 0 `FAIL` en el build) y `npm run test` (15 archivos, 38.689 pruebas) (`build_y_pruebas_resumen.txt`). **No probado:** rebase de prueba sobre upstream. |
| **C5** Formato colombiano | **Pasa** | Recibos con `$ 45.900`, `$128.350` y `1.234.567,00` → `normalized_amount` 45900.00 / 128350.00 / 1234567.00 (`recibos_run2_ok.txt`). Extracto PDF: 12 filas con `3.200.000,00`, `187.450,00`… y neto firmado 3.162.650 = saldo final − inicial (5.512.650 − 2.350.000), diferencia 0. Extracto CSV: 8 filas, neto −175.700 = 174.300 − 350.000, diferencia 0 (`verificacion_api_final.txt`). Dos bancos (Bancolombia Ahorros y Nequi). Tabla de montos en `pkg/aiimport/amount_test.go` (~39 casos), dentro de `ok` en el build. |
| **C6** Captura móvil | **Falla** | Toques para capturar: **Recibo + Subir = 2 toques + disparo**, meta cumplida (`pwa_playwright_log.txt`). **Falla:** tras una subida correcta la hoja no se cierra y reenvía la misma foto (BUG-01, `pwa_bug_hoja_tras_subida.png`). Teléfono real y VPN: **no probado**. Varias fotos seguidas en una sesión: **no probado** (la intercepción del test lo impidió). |
| **C7** Tres CLI | **Pasa** (con fricción) | Claude Code: 3 recibos, extracto PDF, insights y recategorizar. CSV desde la bandeja: falla (BUG-02) y se completó pegando el contenido. `agy`: 1/1 recibo. Codex: 1/1 recibo, con la respuesta de la cuenta. La prueba ciega (C7 del SPEC) la hace `w-blind`: **no probada aquí**. |

## 4. Pasos ejecutados

### 4.1 Build y arranque (README §1)
- `git clone --branch Yosoyepa/w-e2e <worktree> %TEMP%\ebk-e2e-src`, luego `ai-clients/.env` con `EBK_SECURITY_SECRET_KEY` aleatoria (64 hex) y `EBK_SERVER_DOMAIN=localhost`.
- `docker compose -f ai-clients/docker-compose.yml up -d --build` desde el clon. Terminó con código 0; el build ejecutó las pruebas Go de upstream (43 paquetes `ok`) y el lint de frontend.
- Sin problema de CRLF: `.gitattributes` fija LF en `*.sh` (el fallo que documentó S001 ya no ocurre).

### 4.2 Usuario, categorías, cuentas y token MCP (README §2)
- `userdata user-add -n e2etester -m e2e@example.com -i E2ETester -p … -c COP` → exit 0 (ver fricción F1).
- Categorías: 13 de primer nivel creadas primero, borradas después y sustituidas por una jerarquía de dos niveles (ver BUG-04). Cuentas: Efectivo (efectivo), Bancolombia Ahorros (ahorros), Nequi (virtual), todas en COP.
- Token: `userdata user-session-new -n e2etester -t mcp -e 31536000` → longitud 114. Se guardó en un archivo temporal y en `$env:EBK_MCP_TOKEN` del proceso (no en el registro del usuario, ver D2).
- `ai-clients/scripts/check-mcp.ps1` → `initialize` OK, `tools/list` 15 herramientas, las 10 de Finanzas OK, **exit 0** (`check_mcp.txt`).

### 4.3 Recibos con Claude Code (3 recibos sintéticos)
- Generados con PIL (`python -I`): `e2e_01_supermercado.jpg` (128.350, efectivo), `e2e_02_drogueria.jpg` (45.900, tarjeta débito ****4412), `e2e_03_restaurante.jpg` (1.234.567,00 con propina sugerida 112.233, Nequi).
- Subida por `POST /api/v1/inbox/items/upload.json` (kind 1) → 3 ítems pendientes.
- **Corrida 1** (`recibos_run1_catalogo_vacio.txt`): sin escrituras. El catálogo llegó vacío (BUG-04) y el modelo se detuvo a pedir categorías.
- **Corrida 2** (`recibos_run2_ok.txt`): con la jerarquía corregida. `dry_run=true` con las tres filas (`would_create`), luego `dry_run=false` → 3 creadas, 3 ítems `processed`. Al modelo se le dio la respuesta de la cuenta de la débito (ver D4).
- Verificación por API: montos exactos 128350.00 / 45900.00 / 1234567.00; etiqueta `ia`; comentario `… [origen: inbox:<id>]`; foto vinculada con `get_receipt_image` (`transaction_id` coincide en los tres).

### 4.4 Extractos (`/extracto`)
- **PDF Bancolombia** (12 movimientos, saldo inicial 2.350.000,00 y final 5.512.650,00, generado con reportlab y etiquetado como sintético):
  - Corrida 3 (`extracto_pdf_run3_solo_dry_run.txt`): solo `dry_run` (12 `would_create`, 0 duplicados, 0 errores); pidió confirmación.
  - Corrida 4 (`extracto_pdf_run4_sin_aplicar.txt`): otra vez solo `dry_run`, con confirmación explícita en el prompt; tampoco aplicó.
  - Corrida 5 (`extracto_pdf_run5_dos_turnos_ok.txt`): conductor de dos turnos en una sola sesión (ver D3). Aplicó con `dry_run=false` → **12 creadas**, 0 duplicadas, 0 errores.
- **CSV Nequi** (8 movimientos, saldo inicial 350.000,00 y final 174.300,00), subido a la bandeja como `kind 2` (`3852054252100780032`):
  - Corrida 6 (`extracto_csv_run6_contenido_no_llega.txt`): el CSV no llegó al modelo (BUG-02).
  - Corrida 7 (`extracto_csv_run7_webfetch_bloqueado.txt`): permitido `WebFetch` sobre `download_url`. Rechazado porque WebFetch no admite `localhost`, y el respaldo con `curl` pidió permiso que el modo `-p` no tiene.
  - Corrida 8 (`extracto_csv_run8_contenido_pegado_ok.txt`): el contenido del archivo se pegó en el prompt. `dry_run` → `dry_run=false` con `inbox_item_id` → **8 creadas**; el ítem quedó `processed`.
- **Cuadres**: PDF 3.162.650 = 5.512.650 − 2.350.000 (diferencia 0). CSV −175.700 = 174.300 − 350.000 (diferencia 0).
- **Reimportación** (criterio: 0 nuevas):
  - PDF (`reimport_pdf_run9.txt`): 12 duplicadas, 0 creadas, 0 errores.
  - CSV desde ruta local (`reimport_csv_run10.txt`): 8 duplicadas, 0 creadas, 0 errores. (La reimportación se hizo desde el archivo local, no desde la bandeja, porque la lectura desde la bandeja está bloqueada: BUG-02.)
- Total de octubre 2026 después de los extractos: 24 transacciones, luego 26 con `agy` y Codex.

### 4.5 Insights y recategorización
- `/insights octubre 2026` (`insights_run11.txt`): total **2.366.167** en 21 gastos, top de categorías Restaurantes 1.347.167 (4). Comparado con `get_spending_summary` directamente: mismo total y mismo conteo, verificado a mano desde las filas. La variación frente a septiembre se marca como "no disponible" porque septiembre no tiene gastos (correcto).
- `/recategorizar droguería de octubre 2026` (`recategorizar_run12.txt`): sobre una transacción **mal categorizada a propósito** (14.300, efectivo, creada con categoría Restaurantes). Propuso Medicamentos, `update_transaction` con `dry_run=true` (changed true), confirmación y `dry_run=false`. Verificación por API: antes Restaurantes, después Medicamentos; total de octubre sin cambios.

### 4.6 `agy` (C7)
- Configuración **temporal**: `.agents/mcp_config.json` en un workspace de `%TEMP%` con `serverUrl` y `Authorization` (token en texto plano, como advierte el README). No se tocó `~/.gemini/config/mcp_config.json`.
- `agy -p "/recibos …" --dangerously-skip-permissions --output-format stream-json --print-timeout 240s` → `status=SUCCESS` (`agy_recibos_run13.txt`).
- Lectura del recibo `e2e_04_agy_panaderia.jpg` como archivo `media_0.jpg`; `dry_run=true`, luego `dry_run=false`.
- API: transacción de 23.400 en Efectivo, categoría Panadería y cafetería, `ia`, origen `inbox:3852055008015024128`, foto vinculada, ítem `processed`.

### 4.7 Codex (C7)
- Configuración por `-c` (sin tocar `~/.codex/config.toml`): `mcp_servers.ebk.url`, `bearer_token_env_var`, `default_tools_approval_mode="approve"`, `--skip-git-repo-check`, `-s read-only`, `codex exec -m gpt-6-luna`.
- `codex debug prompt-input "hola"` en el workspace lista las cuatro skills (`recibos`, `extracto`, `insights`, `recategorizar`): cumple la verificación del README §5.
- Mención `$recibos`: funciona, la skill se cargó y se usó.
- **Corrida 14** (`codex_recibos_run14_sin_cuenta.txt`): leyó el recibo de la ferretería, pero como el prompt no traía la cuenta de la débito se detuvo a preguntar. No escribió nada (no hubo `dry_run`). Fue un error mío al preparar el prompt.
- **Corrida 15** (`codex_recibos_run15_ok.txt`): relanzado con la respuesta del usuario. El primer intento falló por el wrapper `codex.ps1` de Windows, que rompe los prompts con comillas dobles (ver F7). Con el prompt sin comillas: `dry_run` → `dry_run=false` → transacción de 87.200 en Bancolombia Ahorros, categoría Ferretería, `ia`, origen `inbox:3852055766076751872`, foto vinculada, ítem `processed`. Pendientes: 0.

### 4.8 PWA móvil (C6)
- Playwright (`playwright-core`) con Chrome del sistema, 390×844, `isMobile`, `es-CO`. Login en `/mobile` (la app móvil cuelga de esa ruta, ver BUG-05).
- **Corrida v1, sin intercepción** (no se conserva como evidencia aparte; su registro está en `bug_bucle_subida_pwa.txt`): al subir una foto la app siguió subiéndola. Resultado: **722 recibos duplicados** `receipt.jpg` en 28 s. Antes de limpiar, confirmé que el número ya no crecía. Se borraron los 722 (script de limpieza que solo borra pendientes con ese nombre) y la bandeja quedó en 0 pendientes.
- **Corrida v3, con intercepción de red**: se dejó pasar una subida por captura y se bloquearon las repeticiones, para medir el bucle sin inundar el servidor.
  - Captura 1: Recibo (toque 1), disparo simulado con `setInputFiles`, Subir (toque 2). Solicitudes de subida: **#1 enviada, #2 repetición de la misma foto bloqueada**. La hoja queda abierta con la foto (`pwa_bug_hoja_tras_subida.png`); se cerró con la X.
  - Captura 2: igual con `e2e_07_pwa_papeleria.jpg`: **#3 enviada, #4 repetición bloqueada**.
  - Bandeja → Pendientes: 2 recibos (`pwa_02_bandeja_pendientes.png`). Se descartó el más reciente, se vio en Descartados (`pwa_04_bandeja_descartados.png`) y se reabrió.
  - Procesados: 6 ítems, 5 recibos y el extracto CSV (`pwa_03_bandeja_procesados.png`).
- **Toques de captura**: el flujo previsto es Recibo + Subir = **2 toques + disparo**. El registro del test cuenta además la X de cerrar la hoja (`pwa_playwright_log.txt`), que solo fue necesaria por el bug.
- Capturas: `pwa_00_inicio.png`, `pwa_01_hoja_captura.png`, `pwa_02_bandeja_pendientes.png`, `pwa_03_bandeja_procesados.png`, `pwa_04_bandeja_descartados.png`, `pwa_bug_hoja_tras_subida.png`.

### 4.9 Seguridad (C3)
- `seguridad_c3.txt` (ver tabla del §3). La salida no contiene tokens: el script solo imprime códigos y cuerpos de error.

## 5. Fricción (cada paso del README que falló o confundió)

| # | Paso del README | Qué pasó | Corrección propuesta |
|---|---|---|---|
| F1 | §2 `userdata user-add … -p <contraseña>` | La contraseña queda en el historial de la terminal y el comando imprime el hash y la sal (`[Password]`, `[Salt]`). | Pedir la contraseña por stdin o generarla, y no imprimir el registro completo. Ya lo advierte el README en parte; faltaría el aviso sobre la salida. |
| F2 | §2 "Si tu instancia permite registrarse desde la web, regístrate ahí" | Esta instancia tenía `EnableUserRegister: true` (log de arranque), pero el README no dice cómo comprobarlo. Usé `user-add` para el usuario principal. | Indicar cómo comprobar el registro web (p. ej., la pantalla de alta o el log de arranque). |
| F3 | §2 `[Environment]::SetEnvironmentVariable('EBK_MCP_TOKEN', …, 'User')` | Es un cambio global del usuario de Windows. No lo ejecuté (ver D2). | Documentar la variable de proceso o un `.env` del workspace como alternativa de bajo impacto. |
| F4 | §2 y §6 "crea las cuentas y categorías en la app" | Un usuario creado por CLI no trae categorías. Además, el MCP solo entiende categorías de dos niveles (BUG-04). | Decir que las categorías necesitan subcategorías y crearlas en ese formato. |
| F5 | §5 Claude Code: "aprueba el servidor `ebk` cuando lo pida" | Ese paso es interactivo; con `claude -p` no aplica. Usé `--mcp-config --strict-mcp-config --allowedTools`. | Añadir la receta headless. |
| F6 | §5 Antigravity: configuración global `~/.gemini/config/mcp_config.json` | Usé `.agents/mcp_config.json` del workspace (funciona). `agy -p` necesita `--dangerously-skip-permissions`, que el README no menciona. Versión probada 1.4.12 frente a 1.4.4 del README. | Documentar el flag y la configuración de workspace. |
| F7 | §5 Codex: `codex mcp add` / `config.toml` | Funciona con `-c` y `default_tools_approval_mode="approve"`, pero `codex exec` necesita `''` por stdin. En Windows, el wrapper `codex.ps1` rompe los prompts con comillas dobles (`unexpected argument`). | Documentar la receta y evitar comillas en el prompt. |
| F8 | §6 y comandos: "el recibo indica la cuenta" | Un recibo con "tarjeta débito ****4412" no dice el banco. Los tres CLI preguntaron la cuenta (Claude en la corrida 1, Codex en la 14). | Guardar un mapeo tarjeta→cuenta en la memoria del usuario o en el README, para que el comando no tenga que preguntar. |
| F9 | §6 `/extracto inbox` | Depende de BUG-02: con Claude Code el contenido del CSV no llega. | Corregir el defecto (ver BUG-02) o documentar el fallback. |
| F10 | §6 y `shared/finanzas-workflows.md`: `kind` 1 y 2 | El MCP espera `receipt`/`statement`. Los modelos lo descubrieron por error (BUG-03). | Corregir la documentación y el ejemplo. |
| F11 | §4 y `.env`: `EBK_MCP_ALLOWED_REMOTE_IPS` | Se mapea a `EBK_MCP_MCP_ALLOWED_REMOTE_IPS` en el compose (nombre duplicado). Funciona, pero confunde. | Aclarar el mapeo en el README. |

Lo que no fue fricción: `check-mcp.ps1` (exit 0 a la primera), `docker compose exec` sin `-T` en un shell no interactivo (funciona), el build desde el clon (funciona), y `codex debug prompt-input` (lista las skills).

## 6. Bugs (reproducción, salida y sospecha; sin modificar código)

| ID | Severidad | Resumen | Evidencia | Sospecha (archivo:línea) |
|---|---|---|---|---|
| BUG-01 | **Crítica** | La PWA sube la misma foto en bucle y la hoja no se cierra. 722 duplicados en 28 s. Afecta a C2 y C6. | `bug_bucle_subida_pwa.txt`, `pwa_bug_hoja_tras_subida.png`, `pwa_playwright_log.txt` | `src/components/mobile/ReceiptCaptureSheet.vue:117-119` (`removePhoto` sale si `uploading`); `:183-186` (`removePhoto(photo.id)` mientras `uploading` sigue en true, luego `uploadNextPhoto()` vuelve a tomar `photos.value[0]`). |
| BUG-02 | Alta | `get_inbox_item` de un CSV devuelve dos bloques de texto; Claude Code solo entrega el primero, así que el contenido del extracto no llega al modelo. | `bug_get_inbox_item_csv.txt`, `extracto_csv_run6_contenido_no_llega.txt`, `extracto_csv_run7_webfetch_bloqueado.txt` | El servidor arma los dos bloques en `pkg/mcp/get_inbox_item_tool_handler.go:143-147`. Sospecha: el cliente conserva un bloque (quizá cuando hay `structuredContent`). No verificado en el código del cliente. |
| BUG-03 | Media | `list_inbox_items` espera `kind` como texto (`receipt`/`statement`); la documentación dice 1 y 2. El error es HTTP 400 pero el MCP lo devuelve como `-32603 Internal error`. | `bug_kind_invalido.txt` | Error en `pkg/errs/inbox_item.go:17`. Conversión a JSON-RPC: línea no localizada. |
| BUG-04 | Alta | `query_all_transaction_categories` devuelve catálogo vacío si el usuario solo tiene categorías de primer nivel, sin aviso. `/recibos` se detiene y pide crear categorías. | `bug_categorias_primer_nivel.txt`, `recibos_run1_catalogo_vacio.txt` | `pkg/mcp/query_all_transaction_categories_tool_handler.go:88` (`continue` para primer nivel). |
| BUG-05 | Baja | Rutas profundas de la PWA devuelven 404 JSON: `/mobile/inbox` y `/mobile/login` (solo `/mobile` existe como ruta de servidor). Abrir o recargar la bandeja directamente da 404. | `bug_deep_links_pwa.txt` | `cmd/webserver.go:154` (`StaticFile("/mobile")`), `cmd/webserver.go:131` (`NoRoute` → API JSON). Probado con curl, no con recarga en el navegador. |
| BUG-06 | Baja | `POST /api/register.json` exige `categories` aunque la documentación lo presente como opcional (400 sin el campo). | `bug_registro_categories.txt` | `pkg/models/transaction_category.go:62` (`binding:"required"`), embebido en el registro. |
| BUG-07 | Baja (seguridad) | `/mcp` sin token responde 400 `token is empty`, no 401/403 como dice el README y como exige C3. | `bug_mcp_sin_token_400.txt`, `seguridad_c3.txt` | `pkg/errs/token.go:21` (`ErrTokenIsEmpty` = `http.StatusBadRequest`). |

## 7. Verificación de cierre
- `git grep -nE 'Bearer [A-Za-z0-9]{20,}|ebk_[A-Za-z0-9]{10,}'` y el patrón extra `ebk-[A-Za-z0-9._-]{20,}`: resultado en la salida del worker (`w-e2e_report.md`). La evidencia publicada se revisó con el mismo patrón: ningún archivo lo contiene.
- `git status --porcelain` después del commit: en `w-e2e_report.md`.

## 8. Desviaciones del encargo y de los criterios
- **D1.** El usuario principal se creó con `user-add` (README) y no por el registro web, aunque el registro estaba habilitado. El segundo usuario (`e2eotro`) sí se creó por la API web, con `categories: []` (BUG-06).
- **D2.** No se ejecutó `SetEnvironmentVariable(… 'User')` (cambio global del usuario de Windows). El token se pasó como variable de proceso.
- **D3.** Claude Code: `--setting-sources project,local` (no carga los ajustes del usuario), `--no-session-persistence` (no escribe transcripciones) y `--add-dir` solo para `testdata`. Cuando el modelo se detuvo a pedir confirmación, se usó un conductor (`claude_turns.py`) que mantiene una sola sesión stream-json y envía el turno 2 con la confirmación del usuario.
- **D4.** Las respuestas del usuario (cuenta de la débito, cuenta destino, `source_label`) se pusieron en el prompt, como las daría el responsable. Sin ellas los modelos se detienen (F8).
- **D5.** El CSV de la bandeja llegó al modelo pegado en el prompt (corrida 8), porque la lectura desde la bandeja falla (BUG-02). La reimportación del CSV se hizo desde el archivo local.
- **D6.** La PWA se probó con `playwright-core` y Chrome del sistema, no con Docker, y en mobile emulado. En la corrida v3 se bloquearon las repeticiones con intercepción de red para no volver a inundar el servidor (ver §4.8).
- **D7.** Se usó la mención `$recibos` en Codex (el README no la había probado): funciona.
- **D8.** Versiones: `agy` 1.4.12 frente a 1.4.4 del README. Codex 0.163.0 y modelo `gpt-6-luna`, como en S001.
- **D9.** La transacción de la recategorización se creó por la API REST (no por un CLI) para tener un caso mal categorizado a propósito.
- **D10.** Los modelos no aplicaron siempre al primer intento. En el extracto PDF, las dos corridas de un turno (corridas 3 y 4) se detuvieron en el `dry_run` aunque el prompt traía confirmación. En cambio, la corrida con el conductor de dos turnos (corrida 5) aplicó en su primer turno, y el segundo turno solo verificó. Por eso se usó el conductor (D3). Es comportamiento no determinista del modelo; no es un defecto del servidor.
- **D11.** El fork modifica 18 archivos de upstream (C4 pide menos de 15). No se corrigió porque el encargo prohíbe modificar código; queda como hallazgo.

## 9. No probado y pendientes
- Teléfono real, Tailscale y VPN (C3 y C6). Solo se probó en Chrome con emulación móvil.
- Prueba ciega del README (C7), que corresponde a `w-blind`.
- Rebase de prueba sobre upstream (C4).
- Lista de IP limitada a localhost y Tailscale (C3). Falta configurarla y probarla.
- Suplantación de `X-Forwarded-For` contra la lista de IP (S001 la reprodujo): no repetida en esta corrida.
- Varias fotos seguidas en una sola sesión de captura (C6): bloqueada por BUG-01 en el test.
- Lectura desde la bandeja de un extracto PDF o XLSX (embedded resource): no probada.
- Recarga del navegador en rutas profundas: comprobada solo con curl (BUG-05).

## 10. Lo que funcionó
- Build desde el clon sin problemas de CRLF (`.gitattributes`), pruebas Go y frontend en verde.
- `check-mcp.ps1` con exit 0.
- Recibos con las tres CLI: montos exactos y foto vinculada.
- Gate de `dry_run` en todas las escrituras; dedupe (reimportación = 0); etiqueta `ia` y origen en todas las transacciones de IA; cuadres con diferencia 0.
- Aislamiento entre usuarios del MCP.
- `insights` y `recategorizar` usan las herramientas agregadas y cuadran con la API.
