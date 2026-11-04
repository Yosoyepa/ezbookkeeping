# B002 — Reensayo ciego del README de `ai-clients`

- Fecha: 2026-11-03. Inicio 22:34:23.
- Worker: `w-blind2`. Rama: `Yosoyepa/w-blind2` (base `feat/finanzas-ia`). HEAD de partida: `15d33143`.
- Método: seguí solo `ai-clients/README.md` y lo que me pedía. No leí `docs/tasks/`, `docs/spikes/`, `docs/e2e/B001_blind_report.md` ni código fuente.

## Resultado

**0 de 3 tareas completadas.** La instalación se bloquea en el build: `docker compose -f ai-clients/docker-compose.yml up -d --build` falla en el paso de lint del frontend porque Node se queda sin heap. Fallo reproducido en 2 de 2 intentos. Sin servidor no pude crear el usuario, las cuentas ni el token, así que las tareas 2 y 3 no se pudieron ejecutar.

| Tarea | ¿Lo logré? | Tiempo | Nota |
|---|---|---|---|
| 1. Instalar (servidor, usuario, cuentas, token, Claude Code, `check-mcp`) | **No** | 22:34:47 → 22:54:34 (≈20 min) | Dos builds fallidos de ≈5 min cada uno |
| 2. Registrar 2 recibos (subida, `/recibos`, verificación con la API) | **No** | No completada. Preparación desde 22:36:35 | Depende del servidor |
| 3. `/insights` del mes actual | **No** | No ejecutada | Depende de 1 y 2 |

## Tarea 1 — Instalar: NO

### Comandos usados (sin secretos)

```powershell
git clone --branch Yosoyepa/w-blind2 "C:\Users\juanc\orca\workspaces\Finanzas\w-blind2" "$env:TEMP\ebk-blind2-src"   # README l.41
Copy-Item ai-clients\.env.example ai-clients\.env                 # README l.46, en el clon
# EBK_SECURITY_SECRET_KEY: generada con el snippet de .env.example (líneas 4-5) y escrita en .env sin mostrarla
docker compose -f ai-clients/docker-compose.yml up -d --build     # README l.48: FALLA en el intento 1 y en el reintento
```

Antes del build: el puerto 8080 estaba libre, no había contenedores de Finanzas y Docker era 28.1.1 con Compose v2.35.1.

### Evidencia del fallo

Reintento (las líneas del intento 1 son iguales, a 202 s y 308 s):

```
#19 198.4 FATAL ERROR: Ineffective mark-compacts near heap limit Allocation failed - JavaScript heap out of memory
#19 292.6 Aborted (core dumped)
#19 292.6 Error: Failed to pass lint checking
failed to solve: process "/bin/sh -c ./build.sh frontend" did not complete successfully: exit code: 1
```

Antes del fallo, el log muestra el paso que corre el lint: `RUN ./build.sh frontend` → `> vue-tsc --noEmit && eslint . --fix`.

### Diagnóstico

- La etapa que falla usa `docker.io/library/node:26.13.0-alpine3.24` (línea `FROM` del log).
- Esa misma imagen, ejecutada con `docker run --rm node:26.13.0-alpine3.24 node -p ...`, da `heap_size_limit_MB=2144 node=v26.13.0`.
- Docker Desktop tiene `DockerMemTotal=10431574016` (≈9,7 GiB) y 16 CPU. El límite es el heap de Node dentro del contenedor, no la memoria asignada a Docker. No comprobé si dar más memoria a Docker cambiaría ese heap.
- El README no menciona requisitos de memoria ni una alternativa para el build.
- No leí el Dockerfile ni `build.sh`, así que no sé si existe un argumento de build para subir el heap.

### Lo que no se pudo ejecutar

Registro web, cuentas, token MCP, `check-mcp` con token y conexión MCP de Claude Code dependen del servidor.

Solo comprobé sin servidor:

- `check-mcp.ps1` sin `EBK_MCP_TOKEN` sale con código **2** y el mensaje «Falta la variable de entorno EBK_MCP_TOKEN…», como dice el README (l.262). PowerShell no bloqueó la ejecución del script.
- Receta `-p` con el modelo pedido (prueba de humo; no prueba la conexión MCP):

```powershell
claude -p "Responde solo con la palabra: ok" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools "mcp__ebk"
```

Resultado: respondió `ok` con código 0 (Claude Code 2.1.306, la versión del README l.34).

## Tarea 2 — Registrar 2 recibos: NO

Preparado sin servidor:

- Recibos sintéticos con Python + PIL (`python -I gen_receipts.py`) en un venv temporal, verificados visualmente:
  - `blind2_panaderia.jpg`: PANADERÍA EL TRIGAL, 03/11/2026, pago EFECTIVO, TOTAL $ 18.500 (6.000 + 4.800 + 3.700 + 4.000).
  - `blind2_farmacia.jpg`: FARMACIA SALUD VITAL, 03/11/2026, TARJETA DÉBITO ****9911, TOTAL $ 67.300 (8.500 + 21.300 + 37.500).
  - Están en `testdata/receipts/` y se commitean.
- Mapa de tarjetas según §6: `%USERPROFILE%\.finanzas\cuentas.md` con la línea `****9911 → Bancolombia Ahorros`. Son 37 bytes, UTF-8 con BOM. Se borró en la limpieza.

No hecho: registro de cuenta, subida a la bandeja (ni web ni script), `/recibos` y verificación con la API.

## Tarea 3 — `/insights` del mes actual: NO

No ejecutada. Depende de 1 y 2. No hay análisis guardado.

## Fricción

Cada cita es una línea de `ai-clients/README.md`.

**F1. Bloqueante: el build falla por heap de Node (l.48, l.51).**
- Cita, l.51: «El build ejecuta las pruebas de upstream y tarda unos 6 minutos la primera vez.»
- Esperaba: un build correcto en unos 6 minutos.
- Pasó: OOM en el lint del frontend, dos veces de forma consistente (ver evidencia). El README no avisa de requisitos de memoria ni da alternativa.

**F2. `curl -s` no funciona en PowerShell 5.1 (l.66).**
- Cita: «O desde la terminal: `curl -s http://127.0.0.1:8080/server_settings.js`.»
- Esperaba: el contenido de `server_settings.js`.
- Pasó: en PowerShell 5.1, `curl` es un alias de `Invoke-WebRequest` y da «No se puede procesar el comando porque faltan uno o varios de los parámetros obligatorios: Uri.» Tuve que usar `curl.exe -s`. El resto del README usa PowerShell y no lo advierte.

**F3. El README no dice que hay que crear la carpeta `~/.finanzas` (l.240).**
- Cita: «crea el archivo opcional `~/.finanzas/cuentas.md` (en Windows, `%USERPROFILE%\.finanzas\cuentas.md`)».
- Esperaba: crear el archivo.
- Pasó: `Set-Content` falló con «No se puede encontrar una parte de la ruta de acceso 'C:\Users\juanc\.finanzas\cuentas.md'.» Tuve que crear la carpeta con `New-Item` antes.

**F4. `Set-Content -Encoding utf8` escribe BOM en PowerShell 5.1 (l.249).**
- Cita: «usa un editor que guarde en UTF-8 o `Set-Content -Encoding utf8`.»
- Pasó: el archivo empieza con `EF BB BF`. Es UTF-8 con BOM, y el README no lo menciona.
- **No verificado:** si `/recibos` lo lee bien. No hubo servidor.

**F5. La receta `-p` usa una carpeta que el README crea después (l.185 frente a l.240).**
- Cita, l.185: «`--add-dir "$env:USERPROFILE\.finanzas"` es lo que permite leer el mapa de tarjetas (sección 6)».
- Dudé: la sección 5 usa la carpeta antes de que la sección 6 diga cómo crearla. En la práctica la creé por F3.

**F6. El README no dice qué vía de subida recomienda (l.296, l.304, l.320).**
- Cita, l.296: «llegan a la bandeja desde el celular (sección 4) o desde el PC, de dos formas.»
- Dudé: el botón de la web (l.300) no exige cambios de seguridad. El script (l.304) exige `EBK_SECURITY_ENABLE_API_TOKEN=true` en `ai-clients/.env` y reiniciar (l.320). El README no elige.
- **No probado:** no hubo servidor.

## Lo que no pude probar

- Registro web y categorías predeterminadas (l.61-63). Hay que usar un navegador, y el README lo recomienda (l.61).
- Cuentas «Efectivo» y «Bancolombia Ahorros» (l.86-95).
- Token MCP y las tres formas de dárselo al CLI (l.97-125).
- `check-mcp` con token (l.255-262).
- Conexión MCP desde `claude -p` y los dos pasos de `/recibos` (l.172-200).
- Subida a la bandeja (l.294-321).
- `/insights` (l.235).

## Lo que funcionó según el README

- Clonar desde el worktree (l.41).
- Copiar `.env.example` (l.46) y generar la clave con el snippet de `.env.example` (líneas 4-5).
- `check-mcp` sin token da el código 2 que documenta el README (l.262).
- La receta `-p` acepta sus flags, incluido `--model claude-haiku-5-5`.

## Calificación: 1 de 5 («lo pude usar sin ayuda»)

No pude completar la instalación siguiendo el README. El build falla por una causa que el README no anticipa y no tiene salida documentada. Sin servidor, las tareas 2 y 3 no se pueden hacer. Los pasos que sí ejecuté (clon, `.env`, `check-mcp` sin token, receta `-p`) van como dice el README.

## Incidencias del entorno (no son del README)

- **E1.** El hook PreToolUse de PowerShell (`orca-agent-hook.exe claude`) expiró dos veces seguidas y bloqueó las llamadas. El mensaje dice que falta Git Bash (Git for Windows) y que el responsable debe instalarlo o corregir el hook. No toqué el hook. Al tercer intento funcionó.
- **E2.** El host tiene Node v22.23.1. Las reglas del encargo dicen v20. No usé Node.

## Desviaciones del encargo

- Tareas 2 y 3 no completadas. Tarea 1 incompleta.
- No ejecuté `docker build -t ebk-w-blind2 .` (la regla pide la imagen completa). El paso `fe-builder` es el mismo que falla en el build de compose, así que no lo repetí.
- No ejecuté las pruebas de Go ni de Node. No cambié código: solo añadí este informe y dos recibos.
- Usé `curl.exe` en lugar de `curl` (F2).
- Creé `%USERPROFILE%\.finanzas` (no existía) y lo borré en la limpieza.
- Generé una contraseña de prueba en `%TEMP%` (`creds.json`). No llegué a crear el usuario, y el archivo se borró.

## Limpieza (verificada)

- `docker compose -f ai-clients/docker-compose.yml down -v`: código 0, sin salida. No hay contenedores `ezbookkeeping`.
- Borrados: el clon `%TEMP%\ebk-blind2-src` (con `.env`), el workspace `%TEMP%\ebk-blind2-ws` (`.mcp.json`), el venv `%TEMP%\ebk-blind2-venv`, Chromium de Playwright en `%TEMP%\ebk-blind2-pw` (instalado, no usado), `%TEMP%\ebk-blind2-data` (incluye `creds.json`), `%USERPROFILE%\.finanzas` y el transcript temporal de Claude Code del workspace.
- Restos `ebk-blind2*` en `%TEMP%`: 0.
- Token MCP y token API: no se generaron. Ningún secreto en el repo.
- Configuración global: no modifiqué ningún archivo. Tras la prueba `-p`, `~/.claude.json` no contiene la ruta del workspace temporal.

## Pendientes

- Decidir si se autoriza un cambio en el build (heap de Node o salto del lint) para repetir B002 completo. Sin eso, B002 no puede avanzar.
- Revisar el hook de Orca (E1).
