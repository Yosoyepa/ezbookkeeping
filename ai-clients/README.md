# ai-clients: finanzas desde tus CLI (ezBookkeeping + MCP)

Este paquete conecta tu servidor de ezBookkeeping con **Claude Code**, **Antigravity (`agy`)** y **Codex**. Desde la terminal puedes procesar recibos, importar extractos, analizar gastos y recategorizar transacciones. El análisis lo hace tu CLI con tu propia suscripción: el servidor no usa ninguna API key de pago. El servidor solo guarda los datos y los expone por MCP (Model Context Protocol).

## Arquitectura

```
 Celular (PWA)                              PC con Docker
 foto de recibo, extracto  ----HTTP---->  +-----------------------------+
                           (Tailscale)    | ezBookkeeping (contenedor)  |
                                          |  - API y PWA      :8080     |
 PC: Claude Code / agy / Codex  --MCP-->  |  - MCP  /mcp  (con token)   |
     (tu suscripción)                     |  - datos, fotos y logs en   |
                                          |    volúmenes de Docker      |
                                          +-----------------------------+
```

## Qué hay en esta carpeta

| Ruta | Para qué |
|---|---|
| `docker-compose.yml`, `.env.example` | Servidor ezBookkeeping con MCP activado. |
| `shared/finanzas-workflows.md` | Reglas y flujo de los cuatro comandos. Es la fuente única. |
| `agents/.agents/skills/` | Las skills `recibos`, `extracto`, `insights` y `recategorizar`. **Las usan Antigravity y Codex**: una sola carpeta para los dos. |
| `claude-code/` | `.mcp.json.example`, la skill `finanzas` y los comandos `/recibos`, `/extracto`, `/insights` y `/recategorizar`. |
| `antigravity/` | `mcp_config.json.example`. Las skills están en `agents/`. |
| `codex/` | `config.toml.example`. Las skills están en `agents/`. |
| `scripts/` | `check-mcp` (comprueba la conexión), `sync-workflows` (regenera las copias de las reglas) y `fake_mcp_server.py` (servidor falso para probar los scripts). |

## Requisitos

- Docker Desktop en marcha.
- Tailscale en el PC y en el celular, solo si quieres usar el celular fuera de casa o de tu red local.
- Claude Code, Antigravity CLI (`agy`) o Codex. Se probó con Claude Code 2.1.306, `agy` 1.4.12 y Codex 0.163.0.

## 1. Levantar el servidor

Construye desde el **clon principal** del repositorio: una copia normal (con su propia carpeta `.git`) con la rama que quieres desplegar. No construyas desde un worktree de Git ni desde una copia exportada: el build lee la revisión de Git y falla sin una carpeta `.git` propia. Si trabajas desde un worktree, crea un clon limpio de esa rama y construye ahí:

```powershell
git clone --branch <rama> <ruta-del-worktree> <destino>
cd <destino>
```

```powershell
Copy-Item ai-clients\.env.example ai-clients\.env
# Genera la clave y escríbela en ai-clients\.env sin mostrarla en pantalla:
$b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
$k = -join ($b | ForEach-Object { $_.ToString('x2') })
(Get-Content ai-clients\.env) -replace '^EBK_SECURITY_SECRET_KEY=.*', "EBK_SECURITY_SECRET_KEY=$k" | Set-Content ai-clients\.env
docker compose -f ai-clients/docker-compose.yml up -d --build
```

El build ejecuta las pruebas de upstream y tarda entre 6 y 12 minutos la primera vez. Docker necesita al menos 8 GB de memoria: el compose le da al Node del build un heap de 6 GB (`BUILD_NODE_OPTIONS`, que puedes cambiar con `EBK_BUILD_NODE_OPTIONS` en `.env`), porque el lint del frontend (`vue-tsc`) se queda sin memoria con el heap por defecto. El compose pasa `SKIP_TESTS=TestExchangeRatesApi` como argumento de build. Es un patrón de `go test -skip`, no un booleano: omite las 20 funciones `TestExchangeRatesApi*` de `pkg/exchangerates`, que llaman a APIs externas de tipos de cambio.

Abre `http://127.0.0.1:8080` y comprueba que la aplicación carga.

Tus datos viven en los volúmenes `ai-clients_ezbookkeeping-data`, `ai-clients_ezbookkeeping-storage` y `ai-clients_ezbookkeeping-log` (Compose les antepone el nombre del proyecto, `ai-clients`; compruébalo con `docker volume ls`). `docker compose ... down` no los borra. `docker compose ... down -v` sí borra los datos: no lo uses salvo que quieras perderlos.

## 2. Crear tu usuario y el token MCP

### Crear el usuario: mejor por la web

**Recomendado: regístrate por la web.** Abre directamente `http://127.0.0.1:8080/desktop#/signup` (si el enlace «Crear una cuenta» de la pantalla de inicio no responde, esta ruta funciona igual). Los comandos necesitan categorías de **dos niveles** (categoría → subcategoría), y al registrarte por la web upstream te ofrece las categorías por defecto: en el paso 1 cambia la **divisa predeterminada** a «Peso Colombiano» (viene en Euro, EUR, y las cuentas nuevas la heredan), y en el paso 2 **activa el interruptor «Utilizar categorías de transacciones predeterminadas»** (en inglés, «Use default transaction categories»). Viene **desactivado**; si lo dejas así, el usuario queda sin categorías. Las categorías por defecto ya tienen el formato de dos niveles. Un usuario creado con `user-add` queda **sin categorías**, y `user-add` no puede sembrarlas: solo recibe usuario, correo, apodo, contraseña y moneda. Por eso `user-add` es solo la alternativa.

Para saber si el registro web está habilitado:

- En la pantalla de inicio de sesión, el enlace «Crear una cuenta» está activo (si está deshabilitado, el registro está apagado). La ruta directa es `http://127.0.0.1:8080/desktop#/signup`.
- O desde la terminal: `curl.exe -s http://127.0.0.1:8080/server_settings.js` (en Windows PowerShell 5.1 escribe `curl.exe`: `curl` a secas es un alias de `Invoke-WebRequest`). Si la salida contiene `_['r']=1`, el registro está habilitado; si contiene `_['r']=0`, no.
- El registro viene habilitado de fábrica (`[user] enable_register = true`). Para apagarlo, pon `EBK_USER_ENABLE_REGISTER: "false"` en el `environment` del compose.

### Crear el usuario con `user-add` (alternativa)

Si el registro web está apagado, crea el usuario desde el contenedor. Para no dejar la contraseña en el historial de la terminal, léela con `Read-Host -AsSecureString` y pásala por una variable de entorno del proceso:

```powershell
$secure = Read-Host 'Contraseña' -AsSecureString
$env:EBK_NEW_PASSWORD = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
docker compose -f ai-clients/docker-compose.yml exec -e EBK_NEW_PASSWORD ezbookkeeping sh -c '/ezbookkeeping/ezbookkeeping userdata user-add -n <usuario> -m <correo> -i <apodo> -p "$EBK_NEW_PASSWORD" -c COP'
Remove-Item Env:EBK_NEW_PASSWORD
```

**Sin interacción** (un script, una herramienta sin consola). `Read-Host` no funciona si no hay consola. Si la contraseña ya está en una variable del proceso (por ejemplo, la dejó tu gestor de secretos en `EBK_NEW_PASSWORD`), salta la lectura y ejecuta solo el segundo comando de arriba; `-e EBK_NEW_PASSWORD` la pasa al contenedor sin escribirla en la línea de comandos. No la generes ni la imprimas en el mismo comando: así queda en el historial y en la salida.

> **Aviso.** La salida de `user-add` (y la de los demás comandos `userdata`, como `user-session-revoke`) imprime el registro completo del usuario, con el hash de la contraseña y su sal (`[Password]` y `[Salt]`), y además la configuración completa del servidor (decenas de líneas). No compartas esa salida (ni la pegues en un chat, un issue o una conversación con un modelo).

Como el usuario queda sin categorías, créalas a mano en la app antes de usar los comandos, en dos niveles (por ejemplo, categoría «Alimentación» con subcategoría «Supermercado»). Si solo creas categorías de primer nivel, el MCP las ignora y `query_all_transaction_categories` llega vacío (ver «Notas de upstream»).

### Crear las cuentas

Los comandos no crean cuentas: créalas en la web antes del primer uso.

1. Inicia sesión en `http://127.0.0.1:8080/desktop` y abre la lista de cuentas: el botón **Cuentas** de la barra superior es solo un icono, y la ruta directa es `http://127.0.0.1:8080/desktop#/account/list`. Pulsa **Añadir** en esa lista (no «Añadir transacción» de la barra lateral).
2. Escribe el **nombre** (por ejemplo «Efectivo» o «Bancolombia Ahorros»).
3. Elige la **categoría de cuenta**: es obligatoria. Usa «Efectivo» para el efectivo y «Cuentas de ahorros» para una cuenta bancaria.
4. Elige la **divisa** «Peso Colombiano» (COP) y guarda.

Las categorías de transacción (la jerarquía categoría → subcategoría) se editan en **Ajustes** (el icono de tu usuario, arriba a la derecha) → menú lateral **Datos básicos** → **Categorías de transacciones**. Las categorías de cuenta del paso 3 (Efectivo, Cuentas de ahorros…) son otra lista, fija, que trae upstream.

### El token MCP

Crea el token MCP. Con `-e 0` no caduca; con `-e 31536000` dura un año:

```powershell
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-session-new -n <usuario> -t mcp -e 31536000
```

La última línea de la salida es `[NewToken] <token>`. Nunca lo guardes en el repositorio. Tienes tres formas de dárselo a tu CLI como variable `EBK_MCP_TOKEN`:

- **Solo para esta sesión de terminal (sin cambios globales).** Es lo que menos toca tu sistema: la variable vive mientras la terminal esté abierta y la hereda el CLI que lances desde ella.

  ```powershell
  $env:EBK_MCP_TOKEN = '<token>'
  claude   # o agy, o codex, en esta misma terminal
  ```

- **Para todas tus terminales (cambio global del usuario de Windows).** Ábrela de nuevo después de ejecutarlo:

  ```powershell
  [Environment]::SetEnvironmentVariable('EBK_MCP_TOKEN', '<token>', 'User')
  ```

- **Un archivo del workspace (sin tocar variables globales).** Cada CLI lo hace a su manera:
  - **Claude Code:** crea `.claude/settings.local.json` en la carpeta desde la que lo usas, con `{ "env": { "EBK_MCP_TOKEN": "<token>" } }`. Claude Code carga ese `env` al arrancar en esa carpeta y el `.mcp.json` lo expande, sin variable de proceso ni de usuario (comprobado). El archivo guarda el token en texto plano: no lo subas al repositorio (añade `.claude/settings.local.json` al `.gitignore` de esa carpeta).
  - **Antigravity:** el token va en texto plano en `.agents/mcp_config.json` del workspace (ver la advertencia de la sección 5).
  - **Codex:** solo lee la variable de entorno del proceso (`bearer_token_env_var`), así que usa la variable de sesión de arriba.

Para revocar un token: `userdata user-session-revoke -t <token>`. Para ver las sesiones de un usuario: `userdata user-session-list -n <usuario>`.

## 3. MCP y red

- **El token MCP es la barrera real.** Quien lo tenga puede leer y escribir tus finanzas. Protégelo como una contraseña.
- MCP ya está activado en `docker-compose.yml` con `EBK_MCP_ENABLE_MCP=true`. Las variables de entorno siguen el patrón `EBK_<SECCIÓN>_<CLAVE>` de upstream.
- `EBK_MCP_MCP_ALLOWED_REMOTE_IPS` (en `ai-clients/.env`) es un filtro secundario por IP. Es el nombre que lee upstream (`EBK_` + sección `MCP` + clave `MCP_ALLOWED_REMOTE_IPS`) y el compose lo pasa tal cual, sin renombrarlo. Antes se llamaba `EBK_MCP_ALLOWED_REMOTE_IPS`: si tu `.env` usa el nombre viejo, renómbralo. Lee la sección 8 antes de cambiarlo.
- Si ejecutas el servidor sin Docker, activa lo mismo en `conf/ezbookkeeping.ini`: en la sección `[mcp]`, `enable_mcp = true` y `mcp_allowed_remote_ips = 127.0.0.1`.

## 4. Celular por Tailscale

1. Instala Tailscale en el PC y en el celular con la misma cuenta.
2. Anota la IP `100.x.y.z` del PC: la ves en la app de Tailscale o con `tailscale ip -4`.
3. En `ai-clients/.env` pon `EBK_BIND_IP=<esa IP>` y `EBK_SERVER_DOMAIN=<esa IP o su nombre MagicDNS>`. Después vuelve a ejecutar `docker compose -f ai-clients/docker-compose.yml up -d`. `EBK_SERVER_DOMAIN` importa: upstream escribe esa dirección en las URL absolutas de las fotos, y con `localhost` el celular no las vería.
4. En el celular abre `http://<IP>:8080`.

El compose publica el puerto 8080 en **dos** direcciones: `127.0.0.1` (tus CLI en este PC) y `EBK_BIND_IP` (el celular). No lo publica en todas las interfaces, así que no hace falta abrir puertos en el router. Si dejas `EBK_BIND_IP` en `127.0.0.1`, solo queda esa dirección.

## 5. Instalar en tu CLI (cinco pasos o menos)

### Claude Code

1. Copia `ai-clients/claude-code/.mcp.json.example` a `.mcp.json` en la carpeta desde la que usarás Claude Code.
2. Copia la carpeta `ai-clients/claude-code/.claude/` a esa misma carpeta. Trae la skill `finanzas` y los cuatro comandos.
3. Define `EBK_MCP_TOKEN` con tu token, como en la sección 2. Si tu servidor no está en `http://127.0.0.1:8080/mcp`, define también `EBK_MCP_URL`. Abre una terminal nueva.
4. Ejecuta `claude` en esa carpeta y aprueba el servidor `ebk` cuando lo pida. Comprueba con `claude mcp list`. Hasta que lo apruebes en una sesión interactiva, `claude mcp list` muestra `ebk` como «Pending approval»; la receta `-p` (sección 5, «Uso sin interacción») no lo necesita.
5. Prueba `/recibos`.

### Antigravity (`agy`)

1. Copia la carpeta `ai-clients/agents/.agents/` a la raíz de tu proyecto: queda `.agents/skills/`. Para usar las skills en todos tus proyectos, copia `ai-clients/agents/.agents/skills/*` a `~/.gemini/config/skills/`.
2. Abre `~/.gemini/config/mcp_config.json`. Es la configuración global, la comparten el IDE y la app de Antigravity. Si no existe, créalo. Copia el bloque `mcpServers.ebk` de `ai-clients/antigravity/mcp_config.json.example` y reemplaza `<TU_TOKEN_MCP>` por tu token.

   > **Advertencia.** `agy` no sustituye variables de entorno en las cabeceras, así que el token queda en texto plano en ese archivo. Quien pueda leerlo puede usar tu token: no lo sincronices con una nube compartida ni lo copies al repositorio. Si se filtra, revócalo con `userdata user-session-revoke -t <token>` y crea otro.

3. Comprueba con `agy mcp list`: debe aparecer `ebk`, tipo `http`, `enabled`. Si `agy` ya estaba abierto, usa `/mcp` para recargar la configuración. La ubicación de workspace (`.agents/mcp_config.json`) no aparece en `agy mcp list`; por eso la guía usa la global.
4. Prueba `/recibos`, `/extracto`, `/insights` o `/recategorizar`. `agy` crea un comando por cada skill.
5. Si una skill no aparece, usa `/skills reload` o abre una conversación nueva.

### Codex

1. Copia la carpeta `ai-clients/agents/.agents/` a la raíz de tu proyecto: queda `.agents/skills/`. Codex lee esa carpeta desde el directorio en el que lo ejecutas. Para usar las skills en todos tus proyectos, copia `ai-clients/agents/.agents/skills/*` a `~/.agents/skills/`. Codex 0.163.0 lee las dos rutas.
2. Añade el bloque de `ai-clients/codex/config.toml.example` a `~/.codex/config.toml`. También puedes ejecutar `codex mcp add ebk --url http://127.0.0.1:8080/mcp --bearer-token-env-var EBK_MCP_TOKEN`.
3. Define `EBK_MCP_TOKEN` como en la sección 2 y abre una terminal nueva.
4. Comprueba con `codex mcp list` (debe aparecer `ebk` con `Bearer token`) y con `codex debug prompt-input "hola"`: la salida es JSON, y su sección `Available skills` debe listar `recibos`, `extracto`, `insights` y `recategorizar`.
5. Invoca una skill nombrándola en el mensaje, por ejemplo `$recibos`, o pídela en lenguaje natural: Codex la elige por su descripción. La mención `$recibos` está probada con el modelo y funciona.

### Uso sin interacción (headless): solo para automatizar o probar

**El uso interactivo es el recomendado para el día a día.** Los comandos hacen primero un `dry_run` y esperan tu confirmación; en una terminal interactiva la ves y decides. En modo headless no hay quién confirme: el modelo puede detenerse en el `dry_run` (no aplica nada) o, si le diste permiso para escribir, aplicar sin que tú revises la tabla. Úsalo solo si la confirmación va dentro del propio prompt y revisas el resultado después. Las recetas siguientes sirven para scripts y para pruebas, y no tocan la configuración global de ningún CLI.

El servidor está en `http://127.0.0.1:8080/mcp`. Claude Code y Codex leen el token de `EBK_MCP_TOKEN` (variable de sesión, sección 2); `agy` lo lleva en su archivo de configuración.

**Claude Code** (`-p`). Con `--mcp-config` y `--strict-mcp-config` solo se usa el servidor de ese archivo, y `--allowedTools` autoriza las herramientas del servidor `ebk` sin preguntar (en `-p` nadie puede aprobar):

```powershell
$env:EBK_MCP_TOKEN = '<token>'
claude -p "/recibos" --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools "mcp__ebk"
```

`.mcp.json` es el de la sección 5. `--add-dir "$env:USERPROFILE\.finanzas"` es lo que permite leer el mapa de tarjetas (sección 6; si aún no lo creaste, crea antes la carpeta, aunque esté vacía, con el comando de esa sección): en `-p` nadie puede aprobar la lectura de un archivo fuera de la carpeta de trabajo, y sin ese flag el comando dice «no pude leer `cuentas.md` porque el permiso no fue concedido», aunque el archivo exista. (Comprobado con Claude Code 2.1.306: sirve `--add-dir` solo, o `Read` en `--allowedTools`; sin ninguno de los dos falla. `--add-dir` limita el acceso a esa carpeta y no cambia tu configuración global.) Con `--allowedTools "mcp__ebk"` también quedan autorizadas las herramientas que escriben: pon en el prompt la confirmación explícita (por ejemplo, «aplica con `dry_run=false` después del `dry_run`»), o usa el flujo de dos pasos de abajo.

**Revisar el `dry_run` antes de aplicar (dos ejecuciones).** `--allowedTools` autoriza herramientas completas, no parámetros: no se puede permitir `resolve_inbox_item` o `add_transactions_batch` «solo con `dry_run=true`». Lo que sí funciona es separar los permisos:

```powershell
# Paso 1: solo herramientas de lectura. El modelo no puede escribir y devuelve la tabla propuesta.
claude -p "/recibos Procesa los recibos pendientes con una fila por recibo. No tienes permiso para escribir: devuelve solo la tabla propuesta (id, fecha, comercio, categoría, cuenta, monto)." `
  --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" `
  --allowedTools mcp__ebk__query_all_accounts mcp__ebk__query_all_transaction_categories mcp__ebk__list_inbox_items mcp__ebk__get_inbox_item

# Revisa la tabla. Después, paso 2: autoriza todo el servidor y pega la tabla aprobada en el prompt.
claude -p "/recibos Aplica esta tabla que ya revisé, sin cambiar cuenta, categoría ni monto. Confirmo: aplica con dry_run=false después del dry_run. Tabla: <tabla>" `
  --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools "mcp__ebk"
```

Comprobado con dos recibos: el paso 1 devolvió la tabla (cuenta de la tarjeta incluida) y dejó la bandeja intacta (2 pendientes, 0 transacciones); el paso 2 hizo el `dry_run`, luego aplicó y dejó las 2 transacciones.

**Antigravity** (`agy -p`). Pone el servidor en `.agents/mcp_config.json` del workspace (el formato de `antigravity/mcp_config.json.example`, con el token en texto plano) y ejecuta desde esa carpeta. En modo `-p` hace falta `--dangerously-skip-permissions`, que autoriza todas las herramientas y también la lectura del mapa de tarjetas (comprobado con `agy` 1.4.12: lee `%USERPROFILE%\.finanzas\cuentas.md` sin flags adicionales). Con ese flag no hay permisos que separar entre el paso 1 y el paso 2 del flujo de arriba (no probé si `agy` acepta una lista de herramientas permitidas): pídele en el prompt del paso 1 que no escriba y revisa su tabla antes del paso 2.

```powershell
agy -p "/recibos" --dangerously-skip-permissions --print-timeout 240s
```

**Codex** (`codex exec`). Se configura el servidor con `-c` para no tocar `~/.codex/config.toml`. `default_tools_approval_mode="approve"` autoriza sus herramientas. `codex exec` lee el prompt de los argumentos y, si hay stdin, también de ahí: la entrada vacía `''` evita que se quede esperando:

```powershell
'' | codex exec --skip-git-repo-check -s read-only `
  -c 'mcp_servers.ebk.url="http://127.0.0.1:8080/mcp"' `
  -c 'mcp_servers.ebk.bearer_token_env_var="EBK_MCP_TOKEN"' `
  -c 'mcp_servers.ebk.default_tools_approval_mode="approve"' `
  'Usa $recibos para procesar los recibos pendientes'
```

Con esta receta (`-s read-only`) Codex 0.166.0 leyó `%USERPROFILE%\.finanzas\cuentas.md` sin flags adicionales; en esa prueba su primer intento de lectura falló al crear el proceso del sandbox en Windows (`CreateProcessWithLogonW failed: 267`) y el reintento lo leyó. No lo probé con 0.163.0. Si en tu equipo no lo lee, el comando debe decir que es un problema de permisos (regla 11 de `shared/finanzas-workflows.md`). Como `default_tools_approval_mode="approve"` autoriza todas las herramientas del servidor, el paso 1 del flujo de dos pasos se limita con el prompt, no con permisos.

En Windows, el wrapper `codex.ps1` pasa mal los argumentos con comillas dobles dentro del prompt (en la prueba E001, con Codex 0.163.0, fallaba con `unexpected argument`): escribe el prompt sin comillas dobles. Las comillas dobles de los valores de `-c` (cadenas TOML) sí funcionan entre comillas simples.

## 6. Los cuatro comandos

Las reglas completas están en `shared/finanzas-workflows.md`. En los cuatro comandos el CLI:

- consulta primero las cuentas y las categorías, y nunca inventa ninguna;
- muestra una tabla con `dry_run=true` y crea las transacciones solo después de tu confirmación;
- lee montos en formato colombiano (`1.234.567`, `45.900`, `12,34`) y fechas `dd/mm/aaaa`; si algo no se lee, pregunta.

Ejemplos:

- `/recibos`: procesa todos los recibos pendientes de la bandeja. Puedes pasar un id concreto: `/recibos 12`.
- `/extracto C:\Users\<tú>\Descargas\extracto-octubre.csv`: importa un extracto local, cuadra el total y pide la cuenta y el `source_label`, por ejemplo `bancolombia-2026-10`.
- `/extracto inbox`: importa un extracto que subiste desde el celular. Al crear las transacciones, el extracto de la bandeja queda procesado: el comando pasa `inbox_item_id` en la llamada definitiva. Esto requiere que el servidor tenga ese parámetro.
- `/insights octubre 2026`: top de categorías, variación frente a septiembre, gastos recurrentes, anomalías y tres recomendaciones. Sin periodo, usa el mes en curso.
- `/recategorizar supermercado de octubre`: propone categorías para las transacciones que cumplen el filtro y las cambia tras confirmación.

### Mapa de tarjetas a cuentas

Un recibo que dice «tarjeta débito ****4412» no dice a qué cuenta pertenece, y sin ayuda el comando te lo pregunta cada vez. Para que no pregunte, crea el archivo opcional `~/.finanzas/cuentas.md` (en Windows, `%USERPROFILE%\.finanzas\cuentas.md`) con una línea por tarjeta. La carpeta no existe al principio; en PowerShell créala y escribe el archivo en UTF-8 sin BOM así:

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.finanzas" | Out-Null
[IO.File]::WriteAllText("$env:USERPROFILE\.finanzas\cuentas.md", "****4412 → Bancolombia Ahorros`r`n", (New-Object Text.UTF8Encoding $false))
```

El contenido, una línea por tarjeta:

```
****4412 → Bancolombia Ahorros
****9087 → Nequi
```

Las cuatro skills y comandos lo leen al empezar, si existe. En modo headless, Claude Code necesita `--add-dir` para leerlo (sección 5, «Uso sin interacción»). Si no puede leerlo por permisos, el comando lo dice y no lo confunde con un archivo inexistente. Reglas:

- El archivo va en **UTF-8** (la flecha `→` es un carácter Unicode). En PowerShell 5.1, `Set-Content` y `>` no escriben UTF-8 por defecto: usa el comando de arriba o un editor que guarde en UTF-8. (`Set-Content -Encoding utf8` también escribe UTF-8, pero con BOM.)
- Solo se guardan los **últimos cuatro dígitos**; nunca el número completo.
- El nombre de la cuenta debe ser el que aparece en `query_all_accounts`. Si no existe en el catálogo, el comando te lo pregunta.
- Si la tarjeta no está en el archivo, el comando te pregunta la cuenta y te dice la línea que puedes añadir; no escribe el archivo por ti.
- El archivo vive en tu carpeta de usuario, fuera del repositorio. No contiene secretos, pero no lo subas.

## 7. Comprobar la conexión

```powershell
$env:EBK_MCP_TOKEN = '<tu token>'   # no hace falta si ya lo tienes en tus variables de entorno
.\ai-clients\scripts\check-mcp.ps1
```

En Git Bash, Linux o macOS: `ai-clients/scripts/check-mcp.sh`. El script sale con código `0` si están las diez herramientas de Finanzas, con `1` si falta alguna o no hay conexión, y con `2` si no hay token.

## 8. Seguridad

- **El token MCP es la barrera real.** Da permiso de lectura y escritura sobre tus finanzas. Vive en las variables de entorno de tu usuario o en la configuración global de tu CLI. Nunca va al repositorio. Para comprobarlo: `git grep -nE 'Bearer [A-Za-z0-9]{20,}'` no debe devolver nada.
- **Puertos.** El compose publica el 8080 solo en `127.0.0.1` y en `EBK_BIND_IP`, nunca en todas las interfaces.
- **Filtro por IP (secundario).** Con Docker, el servidor puede ver la IP del gateway de Docker en lugar de la de tu PC o la del celular. La lista por defecto (`172.*.*.*` y `192.168.65.*`) está pensada para ese caso, pero todavía no está comprobada con Docker Desktop en Windows. Si el filtro no distingue tu PC del celular, la barrera sigue siendo el token. Para filtrar por la IP real del cliente, ejecuta el servidor fuera de Docker. Revisa en `docker compose -f ai-clients/docker-compose.yml logs ezbookkeeping` la IP que aparece en las líneas de `/mcp` (el cuarto campo de cada línea) y ajusta `EBK_MCP_MCP_ALLOWED_REMOTE_IPS`.
- **Token de agy.** Va en texto plano en `~/.gemini/config/mcp_config.json` (ver la advertencia de la sección 5).
- **Una sola instancia del servidor.** Las herramientas de importación serializan las escrituras por usuario con un candado en la memoria del proceso (comprobación de duplicados, escritura y cierre del ítem de la bandeja). Eso solo es seguro con **una** instancia del servidor sobre la base de datos; varias instancias no están soportadas.
- **Enlace de descarga de un solo uso (limitación aceptada).** El `download_url` que devuelve `get_inbox_item` (válido 15 minutos como máximo) lleva un código firmado y solo se puede usar una vez, pero el registro de enlaces ya usados vive en la memoria del proceso. Si el servidor se reinicia antes de que caduque, o si hubiera varias instancias, un enlace ya usado podría descargarse otra vez hasta que caduque. Con una sola instancia y un enlace que solo da lectura de un archivo tuyo, se aceptó así. No compartas ese enlace; si algún día hay varias instancias, el consumo del código debe pasar a la base de datos.
- El celular usa la API y la aplicación, no MCP. Por eso no necesita token MCP.

## 9. Cambiar las reglas

Edita `shared/finanzas-workflows.md` y ejecuta:

```powershell
.\ai-clients\scripts\sync-workflows.ps1
```

Ese comando copia las reglas dentro de los archivos de Claude Code y de las skills de `agents/`. Commitea los archivos generados. Con `-Check` no escribe nada y falla si algún archivo quedó desactualizado.

## 10. Notas de upstream

Comportamientos de ezBookkeeping (upstream) que este fork **no cambia**, para que no te sorprendan:

- **Categorías de dos niveles.** El MCP solo lista subcategorías. Si el usuario solo tiene categorías de primer nivel, `query_all_transaction_categories` llega vacío, sin aviso, y los comandos se detienen y te piden crearlas. Crea la jerarquía categoría → subcategoría en la app, o regístrate por la web con las categorías por defecto (sección 2).
- **Rutas profundas de la PWA.** La app móvil es de una sola página: el servidor solo sirve `/mobile` y la navegación interna va después de ese punto en la misma página (tras `#!`, por ejemplo `http://127.0.0.1:8080/mobile#!/inbox`). Los enlaces dentro de la app funcionan. Si abres o recargas una ruta profunda como `http://127.0.0.1:8080/mobile/inbox` directamente, el servidor responde `404` en JSON, igual que con cualquier otra página de upstream. Los enlaces internos de la PWA son correctos (comprobado: el botón «Bandeja» navega a `/mobile#!/inbox`). Para abrir la bandeja directamente, usa `http://127.0.0.1:8080/mobile#!/inbox` (el servidor atiende `/mobile` y el resto de la ruta va en el fragmento), o entra desde el botón de la pantalla de inicio.
- **`POST /api/register.json` exige `categories`.** La documentación de upstream lo presenta como opcional, pero sin el campo responde 400. Envía `"categories": []` para registrar un usuario sin categorías.
- **`user-delete` deja reservado el correo.** Borrar un usuario con `userdata user-delete` no libera su correo. Un registro posterior con ese correo (por la web o por la API) devuelve un `500` genérico, «failed to sign up» (en el log del servidor: `UNIQUE constraint failed: user.email`). Usa otro correo.
- **`/mcp` sin token responde 400, no 401.** Sin la cabecera `Authorization` el servidor responde `400` con `token is empty`; con un token inválido responde `401`. En los dos casos no hay acceso: el token sigue siendo la barrera.

## 11. Subir a la bandeja desde el PC

Los recibos y extractos llegan a la bandeja desde el celular (sección 4) o desde el PC, de dos formas. **La recomendada es el botón de la web**: no necesita tokens ni cambios de configuración. El script es para automatizar y exige activar los tokens api.

### El botón de la web de escritorio

En `http://127.0.0.1:8080/desktop#/inbox` hay dos botones: **Subir extracto** (un archivo) y **Subir recibos** (una o varias imágenes a la vez). Los recibos se comprimen y se suben uno por uno, con una barra de progreso; al terminar sale un aviso («N recibos en la bandeja») y la lista se recarga con los recibos nuevos en «Pendientes».

### El script `upload-inbox`

Para subir desde la terminal o desde un script, sin abrir el navegador:

```powershell
# Requisito: EBK_SECURITY_ENABLE_API_TOKEN=true en ai-clients/.env (ver abajo).
# Token de tipo api (no el token MCP). Una vez:
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-session-new -n <usuario> -t api -e 31536000
$env:EBK_API_TOKEN = '<token>'   # EBK_URL solo si el servidor no está en http://127.0.0.1:8080

.\ai-clients\scripts\upload-inbox.ps1 recibo1.jpg recibo2.jpg
.\ai-clients\scripts\upload-inbox.ps1 -Kind statement .\extracto.csv
```

En Git Bash, Linux o macOS: `ai-clients/scripts/upload-inbox.sh [--kind receipt|statement] <archivo>...`.

- El tipo se deduce de la extensión: imágenes (`jpg`, `jpeg`, `png`, `gif`, `webp`) → recibo; `csv`, `txt`, `ofx`, `qif`, `pdf`, `xlsx`, `xls` → extracto. `-Kind` / `--kind` lo fuerza.
- Por cada ítem creado imprime su id y su tipo (`12 receipt`). Sale con código `0` si todo se subió, `1` si algún archivo falló (el motivo va a la salida de error) y `2` si falta el token o los argumentos. Nunca imprime el token.
- Solo un token **api** puede llamar a la API desde un script; el token MCP solo vale para `/mcp`. Los tokens api vienen **desactivados** (`EBK_SECURITY_ENABLE_API_TOKEN=false`), porque dan acceso total a la cuenta. Para usar el script, pon `EBK_SECURITY_ENABLE_API_TOKEN=true` en `ai-clients/.env` y reinicia con `docker compose -f ai-clients/docker-compose.yml up -d`. Si no, `user-session-new -t api` responde «api token is not enabled». Si solo subes desde el navegador, déjalo en `false`.
- El token api da acceso de lectura y escritura a toda la API de tu cuenta, igual que una sesión: protégelo como el token MCP y no lo guardes en el repositorio.

## Problemas frecuentes

- **HTTP 401 / «token MCP ausente o incorrecto»**: el token está mal copiado, lo revocaste o caducó. Crea otro con `userdata user-session-new`. Sin token, el servidor responde 400 (`token is empty`; ver la sección 10).
- **Claude Code muestra `ebk` como «Pending approval»** (en `claude mcp list`): es el `.mcp.json` del proyecto, que hay que aprobar una vez en una sesión interactiva. Ejecuta `claude` en esa carpeta y aprueba el servidor. La receta `-p` de la sección 5 (`--mcp-config … --strict-mcp-config`) no necesita esa aprobación: usa el archivo que le pasas, así que funciona aunque `claude mcp list` siga mostrando «Pending approval».
- **`check-mcp` dice `FALTA`**: el servidor no tiene las herramientas de Finanzas. Comprueba que el contenedor se construyó desde esta rama.
- **Un comando no encuentra cuentas o categorías**: la app todavía no tiene esas cuentas o categorías. Créalas en la aplicación; los comandos no las crean. Las categorías deben ser de dos niveles (categoría → subcategoría); con solo primer nivel el catálogo llega vacío (sección 10).
- **Codex no encuentra las skills**: la carpeta `.agents/skills/` debe estar en la raíz del proyecto desde la que ejecutas Codex, o en `~/.agents/skills/`. El nombre de cada carpeta debe coincidir con su `name`. `codex debug prompt-input "hola"` lista las skills que ve Codex.
