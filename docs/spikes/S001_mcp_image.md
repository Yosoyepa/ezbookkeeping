# S001 — ¿Los CLI ven una imagen devuelta por MCP? (spike C1)

Fecha: 2026-11-03 · Rama `w-spike-mcp` · Servidor: fork de ezBookkeeping 2.2.0 con la herramienta `get_receipt_image`.

## Resultado

**Los tres CLI pasan.** Claude Code, Antigravity CLI (`agy`) y Codex CLI llamaron a `get_receipt_image`, recibieron la imagen del recibo
y respondieron el comercio y el total exactos (**ALMACENES EXITO**, **$ 45.900**). El plan B (URL de corta duración) **no hizo falta** y no se implementó.

## Qué se construyó

Herramienta MCP `get_receipt_image` (`pkg/mcp/get_receipt_image_tool_handler.go`):

- Entrada: `picture_id` (string con el id de la foto de transacción).
- Salida: un bloque `image` (base64 + `mimeType` según la extensión guardada: jpg/jpeg → `image/jpeg`, png, webp, gif) y `structuredContent` con
  `picture_id`, `transaction_id` (si está adjunta), `uploaded_time` (RFC 3339 UTC) y `mime_type`.
- Seguridad: la consulta de la foto se filtra por el `uid` del token. Una foto de otro usuario responde `transaction picture not found`.
- Errores claros: id no numérico/≤0 → `transaction picture id is invalid`; no existe → `transaction picture not found`; fila sin archivo →
  `transaction picture not exists`; imágenes desactivadas (`enable_transaction_picture=false`) → `not permitted to perform this action`.
- Pruebas unitarias (`get_receipt_image_tool_handler_test.go`, SQLite temporal + almacenamiento local): MIME por extensión, bytes idénticos,
  foto sin transacción, foto de otro usuario, id inexistente, archivo ausente, argumentos inválidos e imágenes desactivadas.

Limitaciones del diseño actual:

- `MCPToolHandler[T]` devuelve una sola lista tipada, así que el bloque de texto «id y fecha» **no puede ir en `content`** junto a la imagen. Va en
  `structuredContent` (solo se envía si el cliente negocia el protocolo `2025-06-18` o posterior; los tres CLI lo hacen).
- Se añadió el tipo `MCP_TOOL_TYPE_GET_RECEIPT_IMAGE = 8` en `pkg/core/mcp_tool.go` (la máscara `MCPAllTools` pasa de 127 a 255). Hace falta porque
  `TestMCPContainerGetMCPTools` exige un bit único por herramienta. **Está fuera de las rutas asignadas al spike**; ver «Pendientes».

## Entorno de prueba

- Contenedor `ebk-spike` (imagen construida desde este árbol) en `127.0.0.1:8080`, datos en `%TEMP%\ebk-spike-data` (fuera del repo).
- Configuración por variables de entorno: `EBK_MCP_ENABLE_MCP=true`, `EBK_MCP_MCP_ALLOWED_REMOTE_IPS=<ip>`, `EBK_SECURITY_SECRET_KEY=<aleatoria>`.
- Usuario de prueba y token MCP creados con la CLI del binario:
  `ezbookkeeping userdata user-add ...` y `ezbookkeeping userdata user-session-new --username <u> --type mcp --expiresInSeconds 14400`.
- Recibo sintético `testdata/receipts/exito_01.jpg` (640×1030, generado con Python + PIL): ALMACENES EXITO, 28/10/2026, 4 ítems
  (4.900 + 7.500 + 9.800 + 23.700), **TOTAL $ 45.900**. Subido con `POST /api/v1/transaction/pictures/upload.json` y adjunto a un gasto de COP 45.900.

### IP permitida

Desde el host Windows con Docker Desktop (WSL2), el servidor ve las peticiones a `127.0.0.1:8080` como venidas de **`172.17.0.1`** (la pasarela del puente de Docker).

| `mcp_allowed_remote_ips` | `X-Forwarded-For` | Resultado |
| --- | --- | --- |
| `172.17.0.1` | — | Pasa (imagen de 154.772 caracteres base64 = el JPEG de 116.077 bytes) |
| `10.9.9.9` | — | `200020 ip address is forbidden to access this resource` |
| `10.9.9.9` | `10.9.9.9` | **Pasa: la lista de IPs se falsea** (ver problemas) |
| `10.9.9.9` + `trusted_proxy_ips=192.0.2.0/24` | `10.9.9.9` | Bloqueado |

## Matriz CLI × resultado

El mismo prompt en los tres:
`Usa la herramienta get_receipt_image del servidor ebk con picture_id=<PICTURE_ID>. Dime el comercio y el total exacto. No adivines.`

| CLI | Versión | Resultado | Cómo ve la imagen |
| --- | --- | --- | --- |
| Claude Code | 2.1.306 | **Pasa** | El `tool_result` trae `content_types=image,text`; el modelo (haiku 5.5) lee la imagen directamente |
| Antigravity CLI (`agy`) | 1.4.4 | **Pasa** | `agy` descarga el contenido MCP a un archivo (`.../brain/<conv>/.system_generated/steps/4/media_0.jpg`) y el modelo lo abre con `read_file` |
| Codex CLI | 0.163.0 (el encargo decía 0.155.1) | **Pasa**, con `-m gpt-6-luna` y aprobación de la herramienta | El resultado MCP trae `{"type":"image","data":"/9j/4AAQ..."}` y el modelo responde con el comercio y el total |

### Comandos exactos (sin token)

Claude Code (config en un archivo temporal, token solo en el entorno; ejemplo en `clients/claude-code.mcp.json`):

```powershell
$env:EBK_MCP_TOKEN = '<EBK_MCP_TOKEN>'
claude -p "<prompt>" --mcp-config <tmp>\mcp.json --strict-mcp-config --allowedTools "mcp__ebk__get_receipt_image" `
  --model claude-haiku-5-5 --output-format stream-json --verbose --no-session-persistence
```

`agy` (carpeta de workspace temporal con `.agents/mcp_config.json`; ejemplo en `clients/agy.mcp_config.json`):

```powershell
Set-Location <tmp>\agy      # contiene .agents\mcp_config.json
agy -p "<prompt>" --dangerously-skip-permissions --output-format stream-json --print-timeout 240s
```

Codex (solo overrides `-c`, sin tocar `~/.codex/config.toml`; ejemplo en `clients/codex.config.toml`):

```powershell
$env:EBK_MCP_TOKEN = '<EBK_MCP_TOKEN>'
'' | codex exec -m gpt-6-luna -C <tmp>\codex --skip-git-repo-check --ephemeral --json -s read-only `
  -c 'mcp_servers.ebk.url="http://127.0.0.1:8080/mcp"' `
  -c 'mcp_servers.ebk.bearer_token_env_var="EBK_MCP_TOKEN"' `
  -c 'mcp_servers.ebk.default_tools_approval_mode="approve"' "<prompt>"
```

### Extractos de la salida

Claude Code:

```
INIT mcp_servers={"name":"ebk","status":"connected"} model=claude-haiku-5-5
TOOL_USE: mcp__ebk__get_receipt_image input={"picture_id":"3852046285473316864"}
TOOL_RESULT is_error= content_types=image,text
ASSISTANT: El comercio es **Almacenes Éxito**, sede Laureles Medellín. El total exacto es **$ 45.900**.
           El recibo es del 28/10/2026 a las 18:42, factura FE-0048213, y se pagó con tarjeta débito. ...
```

`agy`:

```
tool call_mcp_tool {"ServerName":"ebk","ToolName":"get_receipt_image","Arguments":{"picture_id":"3852046285473316864"}}
output: [Resource offloaded to file:///C:/Users/juanc/.gemini/antigravity-cli/brain/<conv>/.system_generated/steps/4/media_0.jpg]
tool read_file {"Path": ".../steps/4/media_0.jpg"}
response: - **Comercio:** ALMACENES EXITO (Sede: EXITO LAURELES MEDELLIN, NIT 890.900.608-9)
          - **Total exacto:** **$ 45.900**
```

Codex:

```
mcp_tool_call server=ebk tool=get_receipt_image arguments={"picture_id":"3852046285473316864"} status=completed
result.content[0] = {"type":"image","data":"/9j/4AAQSkZJRgAB..."}
agent_message: Comercio: **Almacenes Éxito** (sede Laureles, Medellín).  Total: **$45.900**, tal como aparece en el recibo.
```

Las respuestas solo pueden salir de la imagen: `structuredContent` no incluye comercio ni total, y Claude Code y Codex citan datos del pie y de la sede que solo están en el JPEG.

## Problemas encontrados

1. **La lista `mcp_allowed_remote_ips` se falsea con `X-Forwarded-For`.** Por defecto `[security] trusted_proxy_ips` incluye `10/8`, `127/8`,
   `172.16/12`, `192.168/16` y `169.254/16`. Si el par directo (en Docker, `172.17.0.1`) cae en esos rangos, Gin toma la IP del encabezado.
   Cualquiera que llegue al puerto con un token puede declararse una IP permitida. Mitigación comprobada: `EBK_SECURITY_TRUSTED_PROXY_IPS=<solo el proxy real>`.
   Para C3 hay que fijarlo, o no depender de la lista de IPs y proteger la red con un túnel o VPN.
2. **Codex 0.163.0 con esta cuenta**: el modelo por defecto del `config.toml` (`gpt-6.1-sol`) responde `400 model_not_found` («not supported when using Codex with a
   ChatGPT account»). El catálogo (`codex debug models`) lista `gpt-6-luna`, `gpt-5.6-terra` y `gpt-5.6-luna`; se usó `gpt-6-luna`.
3. **Codex `exec` rechaza las llamadas MCP sin aprobación** («MCP tool call requires approval, but approval policy is never»).
   Se resuelve con `default_tools_approval_mode="approve"` en el servidor `ebk`. Además espera stdin: hay que enviarle `''` por tubería.
4. **`agy -p` exige `--dangerously-skip-permissions`** para llamar a herramientas MCP sin interacción. Además guarda en
   `~/.gemini/antigravity-cli/brain/<conversación>/` una copia de la imagen y en `~/.gemini/antigravity-cli/mcp/ebk/` el esquema de la herramienta (estado propio de `agy`, no la configuración global).
5. **Docker en Windows**: `core.autocrlf=true` deja los `*.sh` con CRLF y `docker build` falla con `exit code 127` en `docker/backend-build-pre-setup.sh`.
   Además `build.sh` trata `SKIP_TESTS` como patrón de `go test -skip`, no como booleano (`SKIP_TESTS=1` no salta nada), y el frontend exige un
   repositorio git para `git-rev-sync` (en un worktree `.git` es un archivo que apunta fuera del contexto de Docker). El build se hizo desde una **copia temporal** con los `.sh` en LF, `SKIP_TESTS=TestExchangeRatesApiLatestExchangeRateHandler`
   (esas dos pruebas llaman a APIs externas y fallan sin red) y la importación de `git-rev-sync` quitada de `vite.config.ts` solo en la copia. El repo no se modificó.
6. Los textos «id y fecha» van en `structuredContent` y no en `content` (límite del genérico `MCPToolHandler[T]`).

## Recomendación

- **Seguir con las fases 1 y 2 usando `get_receipt_image` tal cual**: los tres CLI la ven sin plan B.
- Para extractos o recibos grandes, vigilar el tamaño: el base64 sube un 33 % (154.772 caracteres para 116 KB). `MaxTransactionPictureFileSize` por defecto es 10 MB.
- Documentar en las instrucciones de cada CLI las tres particularidades: variable `EBK_MCP_TOKEN`, `default_tools_approval_mode="approve"` en Codex y
  `--dangerously-skip-permissions` en `agy -p`.
- C3: fijar `trusted_proxy_ips` al proxy real y no confiar solo en `mcp_allowed_remote_ips`; usar tokens MCP con caducidad corta.

## Pendientes

- Decidir si `core.MCP_TOOL_TYPE_GET_RECEIPT_IMAGE` (y su bit en `MCPAllTools`) se queda en `pkg/core/mcp_tool.go`; falta mostrar la herramienta
  en la pantalla de tokens MCP del frontend (`src/consts/mcp.ts`, fuera del alcance del spike).
- Probar con un recibo fotografiado real (rotación, baja luz) y con PDF/PNG grandes antes de fijar límites.
