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
- Claude Code, Antigravity CLI (`agy`) o Codex. Se probó con Claude Code 2.1.306, `agy` 1.4.4 y Codex 0.163.0.

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

Si tu instancia permite registrarse desde la web, regístrate ahí. Si no, crea el usuario desde el contenedor:

```powershell
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-add -n <usuario> -m <correo> -i <apodo> -p <contraseña> -c COP
```

El comando deja la contraseña en el historial de la terminal. Cámbiala desde la aplicación después.

Crea el token MCP. Con `-e 0` no caduca; con `-e 31536000` dura un año:

```powershell
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-session-new -n <usuario> -t mcp -e 31536000
```

La última línea de la salida es `[NewToken] <token>`. Guárdalo como variable de entorno de tu usuario de Windows, nunca en el repositorio:

```powershell
[Environment]::SetEnvironmentVariable('EBK_MCP_TOKEN', '<token>', 'User')
```

Abre una terminal nueva para que la vea. Para revocar un token: `userdata user-session-revoke -t <token>`. Para ver las sesiones de un usuario: `userdata user-session-list -n <usuario>`.

## 3. MCP y red

- **El token MCP es la barrera real.** Quien lo tenga puede leer y escribir tus finanzas. Protégelo como una contraseña.
- MCP ya está activado en `docker-compose.yml` con `EBK_MCP_ENABLE_MCP=true`. Las variables de entorno siguen el patrón `EBK_<SECCIÓN>_<CLAVE>` de upstream.
- `EBK_MCP_ALLOWED_REMOTE_IPS` (en `ai-clients/.env`) es un filtro secundario por IP. Lee la sección 8 antes de cambiarlo.
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
5. Invoca una skill nombrándola en el mensaje, por ejemplo `$recibos`, o pídela en lenguaje natural: Codex la elige por su descripción. No he probado la mención `$recibos` con el modelo.

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

## 7. Comprobar la conexión

```powershell
$env:EBK_MCP_TOKEN = '<tu token>'   # no hace falta si ya lo tienes en tus variables de entorno
.\ai-clients\scripts\check-mcp.ps1
```

En Git Bash, Linux o macOS: `ai-clients/scripts/check-mcp.sh`. El script sale con código `0` si están las diez herramientas de Finanzas, con `1` si falta alguna o no hay conexión, y con `2` si no hay token.

## 8. Seguridad

- **El token MCP es la barrera real.** Da permiso de lectura y escritura sobre tus finanzas. Vive en las variables de entorno de tu usuario o en la configuración global de tu CLI. Nunca va al repositorio. Para comprobarlo: `git grep -nE 'Bearer [A-Za-z0-9]{20,}'` no debe devolver nada.
- **Puertos.** El compose publica el 8080 solo en `127.0.0.1` y en `EBK_BIND_IP`, nunca en todas las interfaces.
- **Filtro por IP (secundario).** Con Docker, el servidor puede ver la IP del gateway de Docker en lugar de la de tu PC o la del celular. La lista por defecto (`172.*.*.*` y `192.168.65.*`) está pensada para ese caso, pero todavía no está comprobada con Docker Desktop en Windows. Si el filtro no distingue tu PC del celular, la barrera sigue siendo el token. Para filtrar por la IP real del cliente, ejecuta el servidor fuera de Docker. Revisa en `docker compose -f ai-clients/docker-compose.yml logs ezbookkeeping` la IP que aparece en las líneas de `/mcp` (el cuarto campo de cada línea) y ajusta `EBK_MCP_ALLOWED_REMOTE_IPS`.
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

## Problemas frecuentes

- **HTTP 401 / «token MCP ausente o incorrecto»**: el token está mal copiado, lo revocaste o caducó. Crea otro con `userdata user-session-new`.
- **Claude Code muestra `ebk` como «Pending approval»**: ejecuta `claude` en esa carpeta y aprueba el servidor.
- **`check-mcp` dice `FALTA`**: el servidor no tiene las herramientas de Finanzas. Comprueba que el contenedor se construyó desde esta rama.
- **Un comando no encuentra cuentas o categorías**: la app todavía no tiene esas cuentas o categorías. Créalas en la aplicación; los comandos no las crean.
- **Codex no encuentra las skills**: la carpeta `.agents/skills/` debe estar en la raíz del proyecto desde la que ejecutas Codex, o en `~/.agents/skills/`. El nombre de cada carpeta debe coincidir con su `name`. `codex debug prompt-input "hola"` lista las skills que ve Codex.
