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
| `claude-code/` | `.mcp.json.example`, la skill `finanzas` y los comandos `/recibos`, `/extracto`, `/insights` y `/recategorizar`. |
| `antigravity/` | `mcp_config.json.example` y las skills `recibos`, `extracto`, `insights` y `recategorizar`. |
| `codex/` | `config.toml.example` y los prompts `recibos`, `extracto`, `insights` y `recategorizar`. |
| `scripts/` | `check-mcp` (comprueba la conexión), `sync-workflows` (regenera las copias de las reglas) y `fake_mcp_server.py` (servidor falso para probar los scripts). |

## Requisitos

- Docker Desktop en marcha.
- Tailscale en el PC y en el celular, solo si quieres usar el celular fuera de casa o de tu red local.
- Claude Code, Antigravity CLI (`agy`) o Codex. Se probó con Claude Code 2.1.306, `agy` 1.4.4 y Codex 0.163.0.

## 1. Levantar el servidor

Desde la raíz del repositorio:

```powershell
Copy-Item ai-clients\.env.example ai-clients\.env
# Edita ai-clients\.env y pon EBK_SECURITY_SECRET_KEY (el archivo explica cómo generarla)
docker compose -f ai-clients/docker-compose.yml up -d --build
```

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

- MCP ya está activado en `docker-compose.yml` con `EBK_MCP_ENABLE_MCP=true`. Las variables de entorno siguen el patrón `EBK_<SECCIÓN>_<CLAVE>` de upstream.
- Si ejecutas el servidor sin Docker, activa lo mismo en `conf/ezbookkeeping.ini`: en la sección `[mcp]`, `enable_mcp = true` y `mcp_allowed_remote_ips = 127.0.0.1`.
- `EBK_MCP_ALLOWED_REMOTE_IPS` (en `ai-clients/.env`) limita qué IP pueden usar MCP. Lee la advertencia de la sección 8 antes de cambiarlo.

## 4. Celular por Tailscale

1. Instala Tailscale en el PC y en el celular con la misma cuenta.
2. Anota la IP `100.x.y.z` del PC: la ves en la app de Tailscale o con `tailscale ip -4`.
3. En `ai-clients/.env` pon `EBK_BIND_IP=<esa IP>` y `EBK_SERVER_DOMAIN=<esa IP o su nombre MagicDNS>`. Después vuelve a ejecutar `docker compose -f ai-clients/docker-compose.yml up -d`. `EBK_SERVER_DOMAIN` importa: upstream escribe esa dirección en las URL absolutas de las fotos, y con `localhost` el celular no las vería.
4. En el celular abre `http://<IP>:8080`.

El puerto solo se publica en la IP de Tailscale, así que no hace falta abrir puertos en el router.

## 5. Instalar en tu CLI (cinco pasos o menos)

### Claude Code

1. Copia `ai-clients/claude-code/.mcp.json.example` a `.mcp.json` en la carpeta desde la que usarás Claude Code.
2. Copia la carpeta `ai-clients/claude-code/.claude/` a esa misma carpeta. Trae la skill `finanzas` y los cuatro comandos.
3. Define `EBK_MCP_TOKEN` con tu token, como en la sección 2. Si tu servidor no está en `http://127.0.0.1:8080/mcp`, define también `EBK_MCP_URL`. Abre una terminal nueva.
4. Ejecuta `claude` en esa carpeta y aprueba el servidor `ebk` cuando lo pida. Comprueba con `claude mcp list`.
5. Prueba `/recibos`.

### Antigravity (`agy`)

1. Abre `~/.gemini/config/mcp_config.json`. Es la configuración global, la comparten el IDE y la app de Antigravity. Si no existe, créalo.
2. Copia el bloque `mcpServers.ebk` de `ai-clients/antigravity/mcp_config.json.example` y reemplaza `<TU_TOKEN_MCP>` por tu token. `agy` no sustituye variables de entorno en las cabeceras, así que el token va literal en ese archivo, fuera del repositorio. No copies ese archivo al repositorio.
3. Copia la carpeta `ai-clients/antigravity/.agents/skills/` a `~/.gemini/config/skills/` para usarla en todos tus proyectos, o a `<workspace>/.agents/skills/` solo para uno.
4. Comprueba con `agy mcp list`: debe aparecer `ebk`, tipo `http`, `enabled`. Si `agy` ya estaba abierto, usa `/mcp` para recargar la configuración.
5. Prueba `/recibos`, `/extracto`, `/insights` o `/recategorizar`.

### Codex

1. Copia los archivos de `ai-clients/codex/prompts/` a `~/.codex/prompts/`.
2. Añade el bloque de `ai-clients/codex/config.toml.example` a `~/.codex/config.toml`. También puedes ejecutar `codex mcp add ebk --url http://127.0.0.1:8080/mcp --bearer-token-env-var EBK_MCP_TOKEN`.
3. Define `EBK_MCP_TOKEN` como en la sección 2 y abre una terminal nueva.
4. Comprueba con `codex mcp list`: debe aparecer `ebk` con `Bearer token`.
5. Prueba `/prompts:recibos`. La documentación de Codex marca los prompts personalizados como obsoletos a favor de las skills. Si dejan de aparecer en una versión nueva, avisa para migrarlos.

## 6. Los cuatro comandos

Las reglas completas están en `shared/finanzas-workflows.md`. En los cuatro comandos el CLI:

- consulta primero las cuentas y las categorías, y nunca inventa ninguna;
- muestra una tabla con `dry_run=true` y crea las transacciones solo después de tu confirmación;
- lee montos en formato colombiano (`1.234.567`, `45.900`, `12,34`) y fechas `dd/mm/aaaa`; si algo no se lee, pregunta.

Ejemplos:

- `/recibos`: procesa todos los recibos pendientes de la bandeja. Puedes pasar un id concreto: `/recibos 12`.
- `/extracto C:\Users\<tú>\Descargas\extracto-octubre.csv`: importa un extracto local, cuadra el total y pide la cuenta y el `source_label`, por ejemplo `bancolombia-2026-10`. También acepta `inbox` para usar un extracto subido desde el celular.
- `/insights octubre 2026`: top de categorías, variación frente a septiembre, gastos recurrentes, anomalías y tres recomendaciones. Sin periodo, usa el mes en curso.
- `/recategorizar supermercado de octubre`: propone categorías para las transacciones que cumplen el filtro y las cambia tras confirmación.

## 7. Comprobar la conexión

```powershell
$env:EBK_MCP_TOKEN = '<tu token>'   # no hace falta si ya lo tienes en tus variables de entorno
.\ai-clients\scripts\check-mcp.ps1
```

En Git Bash, Linux o macOS: `ai-clients/scripts/check-mcp.sh`. El script sale con código `0` si están las diez herramientas de Finanzas, con `1` si falta alguna o no hay conexión, y con `2` si no hay token.

## 8. Seguridad

- El token da permiso de escritura sobre tus finanzas. Vive en las variables de entorno de tu usuario o en la configuración global de tu CLI. Nunca va al repositorio. Para comprobarlo: `git grep -nE 'Bearer [A-Za-z0-9]{20,}'` no debe devolver nada.
- Con Docker, el servidor puede ver la IP del gateway de Docker en lugar de la de tu PC o la del celular. La lista por defecto (`172.*.*.*` y `192.168.65.*`) está pensada para ese caso, pero todavía no está comprobada con Docker Desktop en Windows. Si el filtro no distingue tu PC del celular, la barrera real sigue siendo el token. Para filtrar por la IP real del cliente, ejecuta el servidor fuera de Docker. Revisa en `docker compose -f ai-clients/docker-compose.yml logs ezbookkeeping` la IP que aparece en las líneas de `/mcp` (el cuarto campo de cada línea) y ajusta `EBK_MCP_ALLOWED_REMOTE_IPS`.
- El celular usa la API y la aplicación, no MCP. Por eso no necesita token MCP.

## 9. Cambiar las reglas

Edita `shared/finanzas-workflows.md` y ejecuta:

```powershell
.\ai-clients\scripts\sync-workflows.ps1
```

Ese comando copia las reglas dentro de los archivos de cada CLI. Commitea los archivos generados. Con `-Check` no escribe nada y falla si algún archivo quedó desactualizado.

## Problemas frecuentes

- **HTTP 401 / «token MCP ausente o incorrecto»**: el token está mal copiado, lo revocaste o caducó. Crea otro con `userdata user-session-new`.
- **Claude Code muestra `ebk` como «Pending approval»**: ejecuta `claude` en esa carpeta y aprueba el servidor.
- **`check-mcp` dice `FALTA`**: el servidor no tiene las herramientas de Finanzas. Comprueba que el contenedor se construyó desde esta rama.
- **Un comando no encuentra cuentas o categorías**: la app todavía no tiene esas cuentas o categorías. Créalas en la aplicación; los comandos no las crean.
- **`/bin/sh: docker/...sh: not found` al construir la imagen en Windows**: con `core.autocrlf=true`, Git deja los scripts `.sh` con finales de línea CRLF y Docker no puede ejecutar su primera línea (`#!/bin/sh`). Solución local, que solo afecta a este repositorio: `git config --local core.autocrlf false` y después `git checkout -- build.sh docker`. Luego vuelve a ejecutar `docker compose -f ai-clients/docker-compose.yml up -d --build`. Esta solución todavía no está probada en esta máquina. La solución de fondo es un `.gitattributes` en la raíz con `*.sh text eol=lf`; eso afecta a todo el repositorio, así que está pendiente de decisión.
- **`[git-rev-sync] no git repository found` al construir el frontend**: el contexto de build no tiene un directorio `.git`, por ejemplo si lo exportaste con `git archive`. Construye desde un clon normal del repositorio. No he comprobado si funciona desde un worktree de Git, donde `.git` es un archivo.
