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

Construye desde el **clon principal** del repositorio. No construyas desde un worktree de Git ni desde una copia exportada.

```powershell
Copy-Item ai-clients\.env.example ai-clients\.env
# Edita ai-clients\.env y pon EBK_SECURITY_SECRET_KEY (el archivo explica cómo generarla)
docker compose -f ai-clients/docker-compose.yml up -d --build
```

El build ejecuta las pruebas de upstream. El compose pasa `SKIP_TESTS=TestExchangeRatesApi` como argumento de build. Es un patrón de `go test -skip`, no un booleano: omite las 20 funciones `TestExchangeRatesApi*` de `pkg/exchangerates`, que llaman a APIs externas de tipos de cambio.

Abre `http://127.0.0.1:8080` y comprueba que la aplicación carga.

Tus datos viven en los volúmenes `ezbookkeeping-data`, `ezbookkeeping-storage` y `ezbookkeeping-log`. `docker compose ... down` no los borra. `docker compose ... down -v` sí borra los datos: no lo uses salvo que quieras perderlos.

## 2. Crear tu usuario y el token MCP

### Crear el usuario: mejor por la web

**Recomendado: regístrate por la web** (`http://127.0.0.1:8080`). Los comandos necesitan categorías de **dos niveles** (categoría → subcategoría), y al registrarte por la web upstream te ofrece las categorías por defecto («Usar categorías preestablecidas»), que ya tienen ese formato. Un usuario creado con `user-add` queda **sin categorías**, y `user-add` no puede sembrarlas: solo recibe usuario, correo, apodo, contraseña y moneda. Por eso `user-add` es solo la alternativa.

Para saber si el registro web está habilitado:

- En la pantalla de inicio de sesión, el enlace «Crear una cuenta» está activo (si está deshabilitado, el registro está apagado).
- O desde la terminal: `curl -s http://127.0.0.1:8080/server_settings.js`. Si la salida contiene `_['r']=1`, el registro está habilitado; si contiene `_['r']=0`, no.
- El registro viene habilitado de fábrica (`[user] enable_register = true`). Para apagarlo, pon `EBK_USER_ENABLE_REGISTER: "false"` en el `environment` del compose.

### Crear el usuario con `user-add` (alternativa)

Si el registro web está apagado, crea el usuario desde el contenedor. Para no dejar la contraseña en el historial de la terminal, léela con `Read-Host -AsSecureString` y pásala por una variable de entorno del proceso:

```powershell
$secure = Read-Host 'Contraseña' -AsSecureString
$env:EBK_NEW_PASSWORD = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
docker compose -f ai-clients/docker-compose.yml exec -e EBK_NEW_PASSWORD ezbookkeeping sh -c '/ezbookkeeping/ezbookkeeping userdata user-add -n <usuario> -m <correo> -i <apodo> -p "$EBK_NEW_PASSWORD" -c COP'
Remove-Item Env:EBK_NEW_PASSWORD
```

> **Aviso.** La salida de `user-add` imprime el registro completo del usuario, con el hash de la contraseña y su sal (`[Password]` y `[Salt]`). No compartas esa salida (ni la pegues en un chat, un issue o una conversación con un modelo).

Como el usuario queda sin categorías, créalas a mano en la app antes de usar los comandos, en dos niveles (por ejemplo, categoría «Alimentación» con subcategoría «Supermercado»). Si solo creas categorías de primer nivel, el MCP las ignora y `query_all_transaction_categories` llega vacío (ver «Notas de upstream»).

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
4. Ejecuta `claude` en esa carpeta y aprueba el servidor `ebk` cuando lo pida. Comprueba con `claude mcp list`.
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
claude -p "/recibos" --mcp-config .mcp.json --strict-mcp-config --allowedTools "mcp__ebk"
```

`.mcp.json` es el de la sección 5. Con `--allowedTools "mcp__ebk"` también quedan autorizadas las herramientas que escriben: pon en el prompt la confirmación explícita (por ejemplo, «aplica con `dry_run=false` después del `dry_run`»), o limita la lista a las de lectura, por ejemplo `"mcp__ebk__list_inbox_items mcp__ebk__get_inbox_item"`.

**Antigravity** (`agy -p`). Pone el servidor en `.agents/mcp_config.json` del workspace (el formato de `antigravity/mcp_config.json.example`, con el token en texto plano) y ejecuta desde esa carpeta. En modo `-p` hace falta `--dangerously-skip-permissions`, que autoriza todas las herramientas:

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

Un recibo que dice «tarjeta débito ****4412» no dice a qué cuenta pertenece, y sin ayuda el comando te lo pregunta cada vez. Para que no pregunte, crea el archivo opcional `~/.finanzas/cuentas.md` (en Windows, `%USERPROFILE%\.finanzas\cuentas.md`) con una línea por tarjeta:

```
****4412 → Bancolombia Ahorros
****9087 → Nequi
```

Las cuatro skills y comandos lo leen al empezar, si existe. Reglas:

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
- **`/mcp` sin token responde 400, no 401.** Sin la cabecera `Authorization` el servidor responde `400` con `token is empty`; con un token inválido responde `401`. En los dos casos no hay acceso: el token sigue siendo la barrera.

## Problemas frecuentes

- **HTTP 401 / «token MCP ausente o incorrecto»**: el token está mal copiado, lo revocaste o caducó. Crea otro con `userdata user-session-new`. Sin token, el servidor responde 400 (`token is empty`; ver la sección 10).
- **Claude Code muestra `ebk` como «Pending approval»**: ejecuta `claude` en esa carpeta y aprueba el servidor.
- **`check-mcp` dice `FALTA`**: el servidor no tiene las herramientas de Finanzas. Comprueba que el contenedor se construyó desde esta rama.
- **Un comando no encuentra cuentas o categorías**: la app todavía no tiene esas cuentas o categorías. Créalas en la aplicación; los comandos no las crean. Las categorías deben ser de dos niveles (categoría → subcategoría); con solo primer nivel el catálogo llega vacío (sección 10).
- **Codex no encuentra las skills**: la carpeta `.agents/skills/` debe estar en la raíz del proyecto desde la que ejecutas Codex, o en `~/.agents/skills/`. El nombre de cada carpeta debe coincidir con su `name`. `codex debug prompt-input "hola"` lista las skills que ve Codex.
