# B001 · Prueba ciega del README de `ai-clients`

**Estado: INCOMPLETA.** Tarea 1 sí (con desviaciones). Tarea 2 no (bloqueada en la subida de recibos a la bandeja). Tarea 3 sí (ejecutada, sin datos que analizar).

- Fecha: 2026-11-03, de 21:09 a 21:42 (hora local, UTC-5).
- Rama probada: `Yosoyepa/w-blind` en `a5f6aa4f` (la misma punta que `feat/finanzas-ia`). Clon de trabajo: `%TEMP%\ebk-blind-src`, creado con `git clone --branch Yosoyepa/w-blind <ruta del worktree>`.
- Entorno: Windows 11 Pro; Docker Engine 28.1.1 y Compose v2.35.1; Claude Code 2.1.306 (la versión con la que probó el README); Python 3.12.2 con Pillow 10.2.0 para `python -I`; ezBookkeeping `v2.2.0-dev (a5f6aa4)`, según el pie de la web.
- Lo que **no** leí: `docs/tasks/`, `docs/e2e/`, `docs/spikes/` ni el código fuente. Solo `ai-clients/README.md` y los archivos que el README pide abrir (`.env.example`, `.mcp.json.example`, la carpeta `claude-code/.claude`).
- Herramientas: el hook `orca-agent-hook` agotó el tiempo varias veces al usar `Read` y `PowerShell` («Git Bash no encontrado»). Reintenté; cuando `Read` falló, leí con `Get-Content`. No toqué el hook.

## Resultados por tarea

### Tarea 1 · Instalar · **SÍ, con desviaciones**

| Paso | ¿Logrado? | Evidencia |
|---|---|---|
| Servidor (`docker compose … up -d --build`) | sí | Build de 21:15:00 a 21:21:03, código 0. `GET /` → 200. `server_settings.js` → `_['r']=1` (registro web) y `_['mcp']=1`. |
| Usuario | sí, por una vía desviada | Primero `userdata user-add` (21:22:29) y después `user-delete` (21:24:49). Registro web del usuario `blindtest2` (log: «has registered successfully», 21:32:22). |
| Cuentas «Efectivo» y «Bancolombia Ahorros» (COP) | sí | «Efectivo» (categoría de cuenta «Efectivo», divisa «Peso Colombiano»): `POST /api/v1/accounts/add.json` 200 a las 21:33:33. «Bancolombia Ahorros» (categoría «Cuentas de ahorros», COP): 200 a las 21:34:46. |
| Categorías utilizables | sí | `query_all_transaction_categories` por MCP: 17 de primer nivel y 63 subcategorías (las predeterminadas del registro web). |
| Token MCP | sí | `userdata user-session-new -n blindtest2 -t mcp -e 31536000`. Guardado en `C:\Users\juanc\AppData\Local\Temp\ebk-blind\mcp-token.txt` (no impreso). |
| Claude Code, receta headless | sí | `claude -p "/recibos" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --allowedTools "mcp__ebk"`: código 0 y herramientas ejecutadas (21:23:11, con el usuario de `user-add`). |
| `check-mcp` | sí | Código 0, 10 de 10 herramientas de Finanzas. |

- Tiempo: de 21:12:49 a unos 21:37 (≈ 25 min). Incluye 6 min de build y unos 6 min de desvíos (`user-add`, borrado y un registro fallido).

Comandos (sin tokens ni contraseñas):

```powershell
git clone --branch Yosoyepa/w-blind "<ruta del worktree>" "$env:TEMP\ebk-blind-src"
Copy-Item ai-clients\.env.example ai-clients\.env
# EBK_SECURITY_SECRET_KEY: snippet de .env.example, escrito en .env sin imprimirlo
docker compose -f ai-clients/docker-compose.yml up -d --build
curl.exe -s -o NUL -w "%{http_code}" http://127.0.0.1:8080/
curl.exe -s http://127.0.0.1:8080/server_settings.js
# (desviación) usuario de prueba con user-add; luego borrado con user-delete
docker compose -f ai-clients/docker-compose.yml exec -e EBK_NEW_PASSWORD ezbookkeeping sh -c '/ezbookkeeping/ezbookkeeping userdata user-add -n blindtest -m <correo de prueba> -i Blind -p "$EBK_NEW_PASSWORD" -c COP'
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-delete -n blindtest
# registro web en http://127.0.0.1:8080/desktop#/signup: usuario blindtest2, divisa COP, «Utilizar categorías … predeterminadas» → Enviar
# cuentas: Cuentas → Añadir → «Efectivo»; y «Bancolombia Ahorros» con categoría «Cuentas de ahorros»
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-session-new -n blindtest2 -t mcp -e 31536000
Copy-Item ai-clients\claude-code\.mcp.json.example .mcp.json
Copy-Item -Recurse ai-clients\claude-code\.claude .claude
$env:EBK_MCP_TOKEN = <token leído del archivo temporal>; claude mcp list
claude -p "/recibos" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --allowedTools "mcp__ebk"
.\ai-clients\scripts\check-mcp.ps1
```

### Tarea 2 · Registrar 2 recibos · **NO**

- **Generados:** `python -I gen_receipts.py` (Pillow del sistema). `blind_panaderia_efectivo.jpg` (Panadería La Espiga, 02/11/2026, **$ 18.500** en efectivo) y `blind_farmacia_debito.jpg` (Farmacia Vida Sana, 03/11/2026, **$ 67.300**, tarjeta débito ****9911). Los revisé a ojo y los copié a `testdata/receipts/`.
- **Mapa de tarjetas:** creado `%USERPROFILE%\.finanzas\cuentas.md` con la línea `****9911 → Bancolombia Ahorros` (UTF-8 sin BOM). Está fuera del repositorio y pendiente de borrar en la limpieza.
- **Subida a la bandeja con la API: no logrado.** El README no documenta ninguna API de subida (la línea 199 solo habla de extractos subidos desde el celular). Intenté ver qué hace la PWA (`http://127.0.0.1:8080/mobile#!/inbox`) en el navegador integrado de Orca:
  - Hay dos `input type=file` ocultos. Al asignarles el archivo quedan con 0 archivos.
  - Un evento `change` sintético no genera ninguna petición. Con un interceptor de `fetch`/XHR que no envía nada, tampoco se registró ninguna.
  - No leí el código fuente para buscar el endpoint, porque el ensayo es ciego.
- **`/recibos`** con la bandeja vacía (21:23 y después): «No hay recibos pendientes en la bandeja».
- **Verificación con la API:** no aplicable, porque no hay registros que verificar.

### Tarea 3 · `/insights` del mes actual · **SÍ (ejecutado, sin datos)**

- Comando: `claude -p "/insights" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --allowedTools "mcp__ebk"`. Código 0, de 21:38:21 a 21:38:51.
- Respuesta guardada en `C:\Users\juanc\AppData\Local\Temp\ebk-blind\insights_2026-11.md`. Resumen: «No hay gastos que analizar, así que no he generado el informe». Lo consultó en el periodo 01/11/2026–03/11/2026 y en el anterior. Confirma que el MCP ve las cuentas `Efectivo` y `Bancolombia Ahorros`.
- Sin recibos registrados (tarea 2) no hay gastos, así que el comando no tiene nada que analizar.

## Fricción

Cada fila cita la línea del README (`ai-clients/README.md`) y lo que yo esperaba.

| # | Línea del README | Qué esperaba | Qué pasó | Gravedad |
|---|---|---|---|---|
| 1 | L38: «Construye desde el clon principal… No construyas desde un worktree» | Cómo obtener un clon limpio cuando trabajo desde un worktree | `git clone --branch <rama> <worktree>` funcionó. El README no define «clon principal» | baja |
| 2 | L46: «El build ejecuta las pruebas de upstream» | Cuánto tarda | 6 min 03 s, sin ninguna indicación previa | baja |
| 3 | L56: «regístrate por la web» y «Usar categorías preestablecidas» | Registro web con ese paso | Sin navegador conectado (la extensión de Chrome no estaba conectada), la vía recomendada no era posible. En la UI el paso dice «Utilizar categorías de transacciones predeterminadas», texto distinto al del README. En el navegador de Orca, el enlace «Crear una cuenta» no respondió al clic; llegué con la ruta `#/signup` | media |
| 4 | L69: `Read-Host 'Contraseña' -AsSecureString` (L64–73, `user-add`) | Crear el usuario sin intervención | `Read-Host` no sirve en un shell no interactivo. Generé la contraseña en el proceso y la pasé por variable de entorno | media |
| 5 | L77: «créalas a mano en la app»; L262: «Créalas en la aplicación» | Dónde y cómo crear cuentas y categorías | No hay pasos. Encontré Cuentas → Añadir en unos 5 min. El README no dice que cada cuenta necesita una categoría de cuenta (Efectivo, Cuentas de ahorros…) | media |
| 6 | No está en el README: `userdata user-delete` | Que borrar un usuario de prueba libere su correo | El correo queda reservado. Un registro posterior con el mismo correo devolvió HTTP 500 «failed to sign up» sin explicación (log: `UNIQUE constraint failed: user.email`). Tuve que usar otro usuario | media |
| 7 | L81–107: tres formas de dar `EBK_MCP_TOKEN` | Una forma clara para uso no interactivo | La receta headless (L162–165) pide `$env:EBK_MCP_TOKEN` en el mismo proceso. En mi herramienta el entorno no persiste entre comandos, así que lo fijé en cada comando | baja |
| 8 | L132: «Comprueba con `claude mcp list`»; L260 | `ebk` listo para usar | `claude mcp list` mostró «⏸ Pending approval (run claude to approve)». Aun así, `claude -p` con la receta de L162–165 ejecutó las herramientas. El README no explica la diferencia ni cómo aprobar en modo no interactivo | media |
| 9 | L162–167 (receta headless) y L205–217 (mapa de tarjetas) | Que el mapa de tarjetas funcione en `-p` | `/recibos` y `/insights` dicen «no pude leer `C:\Users\juanc\.finanzas\cuentas.md` porque el permiso de lectura no fue concedido». Con el archivo sin crear (1.ª ejecución) y ya creado (2.ª), el mensaje es el mismo: confunde «no existe» con «sin permiso». La receta no concede lectura fuera del workspace, y el README no lo menciona | **alta** |
| 10 | L156: «Úsalo solo si la confirmación va dentro del propio prompt» | Revisar la tabla del `dry_run` antes de aplicar | Solo hay la opción de confirmar en el propio prompt. No se indica cómo revisar y después aplicar (por ejemplo, con dos ejecuciones). No lo probé, porque no hubo recibos | media (sin probar) |
| 11 | L199 solo menciona subir extractos desde el celular; no hay sección de subida | Cómo subir recibos a la bandeja desde el PC («con la API») | El README no documenta la API de subida ni la subida desde el PC. En el navegador de Orca, los inputs de archivo de la PWA no recibieron el archivo y un `change` sintético no generó ninguna petición. Esto bloqueó la tarea 2 | **alta** |
| 12 | L203–217: formato del mapa de tarjetas (usa `→`) | Codificación del archivo | El README no dice que el archivo es UTF-8. Lo escribí en UTF-8 sin BOM | baja |

Lo que funcionó bien, sin fricción:
- L41–42: el snippet de `.env.example` genera la clave sin problemas.
- L58–62: `server_settings.js` permite comprobar si el registro web está activo.
- L226: `check-mcp` da una salida clara y código 0 (10 de 10).
- L253: `http://127.0.0.1:8080/mobile#!/inbox` abre la bandeja, como dice el README.

Hallazgos de la aplicación (fuera del README):
- Registrar con un correo que ya usó un usuario borrado devuelve HTTP 500 genérico (ver fila 6).
- `user-add` imprime la configuración completa del servidor (decenas de líneas) además del usuario. El aviso de L75 cubre solo el hash.

## Calificación: 2 / 5 («lo pude usar sin ayuda»)

Con el README levanté el servidor, conecté Claude Code en modo headless y pasé `check-mcp`. Pero sin ayuda externa no habría creado las cuentas (no hay pasos), no habría resuelto el permiso de lectura del mapa de tarjetas en `-p` (fila 9), y no habría subido ningún recibo desde el PC (fila 11).

## Limpieza

**No ejecutada.** Quedan activos: el contenedor `ai-clients-ezbookkeeping-1`, el clon `%TEMP%\ebk-blind-src`, el token y los datos en `%TEMP%\ebk-blind\`, y el mapa de tarjetas `%USERPROFILE%\.finanzas\cuentas.md`. Queda a decisión del responsable si se limpia ahora o si la prueba continúa.

## Hecho cuando

- [x] Las 3 tareas tienen resultado (sí/no) con evidencia: tarea 1 sí (con desviaciones), tarea 2 no, tarea 3 sí (sin datos).
- [x] Sección de fricción completa.
- [ ] `git grep` y `git status --porcelain` tras el commit: ver el informe del worker (`C:\Users\juanc\.claude\orca_prompts\w-blind_report.md`).
