# B003: reensayo ciego del README de `ai-clients` (C7)

- **Ejecutor:** worker `w-blind3` (Claude Haiku 5.5, esfuerzo max), sin contexto previo del proyecto.
- **Fecha y horario:** 2026-11-03, de 23:07 a 23:40 (hora local, UTC-5).
- **Código probado:** rama `Yosoyepa/w-blind3` @ `5f06eb72`, clonada a `%TEMP%\ebk-blind3-src`.
- **Entorno:** Windows 11 Pro 10.0.26300, PowerShell 5.1. Docker Desktop 28.1.1 (10,4 GB de memoria). Claude Code 2.1.306. Python 3.12.2 con Pillow y Playwright (Chromium) en un venv temporal. Node del host v22.23.1 (no se usó).
- **Reglas del ensayo:** solo leí `ai-clients/README.md` y lo que el README me pidió abrir: `ai-clients/.env.example` (para la clave), y `.mcp.json.example` y la carpeta `.claude/` (para copiarlos). No leí `docs/tasks/`, `docs/spikes/`, `docs/e2e/B001_blind_report.md`, `docs/e2e/B002_blind_report.md` ni código fuente. En `docs/e2e/` solo listé nombres para confirmar que B003 no existía.
- **Herramientas que no vienen en el README:** Playwright (Chromium) para la web, porque Claude in Chrome no estaba disponible en este worker; Python con Pillow para generar los recibos; el token de sesión web del navegador para consultar la API.

## 1. Resultado por tarea

| # | Tarea | ¿Lo logré? | Tiempo | Evidencia |
|---|---|---|---|---|
| 1 | Instalar: servidor, usuario, cuentas COP con categorías, token MCP, Claude Code en modo `-p`, `check-mcp` | **Sí** | ≈26 min (≈11 de ellos, el build) | `check-mcp`: exit 0, 10 de 10 herramientas. `claude -p` devolvió las dos cuentas. API: 2 cuentas en COP; 17 categorías de primer nivel y 63 de segundo. |
| 2 | Registrar 2 recibos: subida desde el PC por la web y `/recibos` | **Sí** | ≈3 min | API `transactions/list.json`: 2 transacciones tipo 3 (gasto), importes `sourceAmount` 1850000 (Efectivo, panadería) y 6730000 (Bancolombia Ahorros, droguería), comentario `[origen: inbox:<id>]`. Bandeja: Pendientes vacía, Procesados con 2 recibos. |
| 3 | `/insights` del mes en curso, guardar la respuesta | **Sí** | 22 s | Respuesta completa en el anexo A. Se guardó en un archivo temporal UTF-8 que se borró en la limpieza. |

## 2. Comandos usados

Sin tokens ni contraseñas. Las rutas de trabajo temporales se resumen.

### Tarea 1

```powershell
git clone --branch Yosoyepa/w-blind3 "C:\Users\juanc\orca\workspaces\Finanzas\w-blind3" "$env:TEMP\ebk-blind3-src"
Set-Location "$env:TEMP\ebk-blind3-src"
Copy-Item ai-clients\.env.example ai-clients\.env
# EBK_SECURITY_SECRET_KEY: receta de .env.example (32 bytes aleatorios en hex), escrita en ai-clients\.env sin imprimirla
docker compose -f ai-clients/docker-compose.yml up -d --build
curl.exe -s http://127.0.0.1:8080/server_settings.js        # contiene _['r']=1
# Registro web (Playwright) en http://127.0.0.1:8080/desktop#/signup
#   usuario blind3, apodo Blind3, correo de prueba blind3@example.com
#   paso 1: Divisa predeterminada = Peso Colombiano COP (por defecto estaba Euro EUR)
#   paso 2: interruptor «Utilizar categorías de transacciones predeterminadas» = activado
# Cuentas (web: icono Cuentas -> Añadir):
#   «Efectivo», categoría Efectivo, COP
#   «Bancolombia Ahorros», categoría Cuentas de ahorros, COP
docker compose -f ai-clients/docker-compose.yml exec ezbookkeeping /ezbookkeeping/ezbookkeeping userdata user-session-new -n blind3 -t mcp -e 31536000
$env:EBK_MCP_TOKEN = <token leído de un archivo temporal>
.\ai-clients\scripts\check-mcp.ps1                          # exit 0, 10 de 10
New-Item -ItemType Directory -Force "$env:USERPROFILE\.finanzas"
# Carpeta de trabajo temporal: .mcp.json.example -> .mcp.json, y ai-clients\claude-code\.claude\ copiada
claude -p "Usa la herramienta query_all_accounts y responde solo con el nombre y la divisa de cada cuenta, una por línea." --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools mcp__ebk__query_all_accounts
```

### Tarea 2

```powershell
python -I make_receipts.py <carpeta temporal>     # blind3_panaderia.jpg ($ 18.500, efectivo); blind3_farmacia.jpg ($ 67.300, débito ****9911)
# Mapa de tarjetas (sección 6 del README), con ****9911:
[IO.File]::WriteAllText("$env:USERPROFILE\.finanzas\cuentas.md", "****9911 → Bancolombia Ahorros`r`n", (New-Object Text.UTF8Encoding $false))
# Subida (Playwright): http://127.0.0.1:8080/desktop#/inbox -> «Subir recibos» con los 2 JPG -> aviso «2 recibos en la bandeja»
# /recibos, paso 1 (solo lectura):
claude -p "/recibos Procesa los recibos pendientes con una fila por recibo. No tienes permiso para escribir: devuelve solo la tabla propuesta (id, fecha, comercio, categoría, cuenta, monto)." --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools mcp__ebk__query_all_accounts mcp__ebk__query_all_transaction_categories mcp__ebk__list_inbox_items mcp__ebk__get_inbox_item
# Revisé la tabla (anexo B) y la aprobé. /recibos, paso 2 (escribe):
claude -p "/recibos Aplica esta tabla que ya revisé, sin cambiar cuenta, categoría ni monto. Confirmo la tabla: aplica con dry_run=false después del dry_run. Tabla: <anexo B>" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools mcp__ebk
# Verificación por API (token de sesión web; cabecera X-Timezone-Offset: -300):
#   GET http://127.0.0.1:8080/api/v1/transactions/list.json?maxTransactionId=0&minTransactionId=0&count=20
# Bandeja (web): Pendientes -> vacía; Procesados -> 2
```

### Tarea 3

```powershell
claude -p "/insights" --model claude-haiku-5-5 --mcp-config .mcp.json --strict-mcp-config --add-dir "$env:USERPROFILE\.finanzas" --allowedTools mcp__ebk__query_transactions mcp__ebk__get_spending_summary mcp__ebk__query_all_accounts mcp__ebk__query_all_transaction_categories
```

### Limpieza

```powershell
# Token MCP revocado con el comando de la sección 2 del README; después, /mcp responde 401
docker compose -f ai-clients/docker-compose.yml down -v
Remove-Item -Recurse -Force "$env:TEMP\ebk-blind3-src"
Remove-Item -Recurse -Force "$env:USERPROFILE\.finanzas"
# Archivos temporales con el token, la contraseña de prueba y la sesión web: borrados
```

Salida de la limpieza: `down -v` eliminó el contenedor `ai-clients-ezbookkeeping-1`, la red `ai-clients_default` y los volúmenes `ai-clients_ezbookkeeping-data`, `-storage` y `-log`. Después, `docker ps -a` y `docker volume ls` no muestran nada de ezbookkeeping.

## 3. Fricción

Las líneas citadas son las de `ai-clients/README.md`. Ordenadas por impacto.

**F1. L61: divisa predeterminada en el registro (media-alta).**
Cita: «Abre directamente `http://127.0.0.1:8080/desktop#/signup` … activa el interruptor …».
Esperaba: que el registro no dejara una divisa que contradice el resto del README (COP en L76 y L93).
Pasó: el paso 1 trae «Divisa predeterminada: Euro EUR», y el README no lo menciona. Si no se cambia, las cuentas nuevas proponen EUR. La cambié a Peso Colombiano; después, el diálogo «Añadir cuenta» ya propuso COP.

**F2. L185 y L196: confirmación explícita en el paso 2 (media).**
Cita de L185: con `--allowedTools "mcp__ebk"` hay que «pon en el prompt la confirmación explícita (por ejemplo, «aplica con `dry_run=false` después del `dry_run`»)», o usar el flujo de dos pasos.
Pasó: el ejemplo del paso 2 (L196) sigue el flujo de dos pasos, pero su prompt no contiene esa confirmación. La añadí yo. El comando hizo `dry_run` antes de aplicar, pero el texto del README no lo garantiza.

**F3. L107–123 y L181: ¿dónde guardar el token para Claude Code? (media).**
L121 sugiere `.claude/settings.local.json` para Claude Code, L107 una variable de sesión, y la receta `-p` de L181 usa `$env:EBK_MCP_TOKEN`. El README no dice cuál usar en modo no interactivo. Usé la variable de proceso en cada comando, leyendo el token de un archivo temporal, porque en mi shell las variables no persisten entre llamadas. Esto es del entorno, pero L107 asume una terminal que las conserva.

**F4. L182, L185 y L240–245: la carpeta de `--add-dir` (baja-media).**
El comando de L182 usa `--add-dir "$env:USERPROFILE\.finanzas"`, pero esa carpeta no existe tras la instalación. Que hay que crearla lo dice L185 («crea antes la carpeta, aunque esté vacía, con el comando de esa sección»), y el comando está en la sección 6. Hay que saltar de la sección 5 a la 6. No probé qué pasa sin la carpeta; la creé antes.

**F5. L47 y `.env.example` líneas 4–5: la clave secreta (baja).**
L47 dice «el archivo explica cómo generarla». La línea de PowerShell de `.env.example` imprime la clave en pantalla y no dice qué hacer con ella. Esperaba un comando que la escribiera en `ai-clients\.env`. La escribí con un script, sin imprimirla.

**F6. L82 y L125: `user-session-revoke` imprime toda la configuración del servidor (media).**
Revocar un token (L125) imprime la configuración completa del servidor. La clave aparece como `****`, pero el resto sale en pantalla. L82 avisa de esto solo para `user-add`. Lo vi al revocar el token de prueba.

**F7. L51: duración del build (baja).**
El README dice «tarda unos 6 minutos la primera vez». Tardó unos 11 minutos, desde el arranque del build hasta `Container … Started`, con las pruebas de upstream y el lint del frontend. No bloqueó.

**F8. Verificación por API sin receta (media).**
El README no tiene una receta para comprobar datos por la API. L327 solo habla del token `api` para scripts, y ese viene desactivado. Usé el token de sesión web del navegador. `GET /api/v1/transactions/list.json` responde 400 (`client timezone offset is invalid`, código 200008) sin la cabecera `X-Timezone-Offset`, que el README no menciona. Con `X-Timezone-Offset: -300` responde 200.

**F9. L55: nombres de los volúmenes (baja).**
El README dice que los datos están en `ezbookkeeping-data`, `ezbookkeeping-storage` y `ezbookkeeping-log`. Compose los creó con el prefijo del proyecto: `ai-clients_ezbookkeeping-data`, etc. Un `docker volume rm ezbookkeeping-data` no los encontraría.

**F10. L90–93: «Abre Cuentas → Añadir» (baja).**
«Cuentas» es un botón solo con icono en la barra superior, sin texto. «Añadir» también aparece en la barra lateral como «Añadir transacción»; abrí ese diálogo por error y lo cerré sin guardar (ver incidentes). Esperaba una opción con nombre.

**F11. L150 y L174: el flujo interactivo no quedó probado.**
L174 dice que el uso interactivo es el recomendado para el día a día, y L150 describe aprobar `ebk` en una sesión interactiva. Desde mi herramienta no pude abrir una sesión interactiva; solo probé `claude -p`. No hay evidencia de la aprobación ni de `claude mcp list`.

**Lo que funcionó tal como dice el README:**
- L61: el interruptor de categorías predeterminadas viene desactivado.
- L66: `curl.exe -s …/server_settings.js` muestra `_['r']=1`.
- L92: las categorías de cuenta «Efectivo» y «Cuentas de ahorros» existen con esos nombres.
- L102 y L105: el comando de token MCP funciona y la última línea es `[NewToken] …`.
- L265–269: `check-mcp` con `$env:EBK_MCP_TOKEN` sale con 0 y encuentra las 10 herramientas.
- L244: el comando del mapa de tarjetas escribe UTF-8 sin BOM con la flecha correcta.
- L307: `#/inbox` abre directamente, tiene «Subir recibos» y muestra «2 recibos en la bandeja».
- L200: el flujo de dos pasos de `/recibos` funciona.
- L235: `/insights` sin periodo usa el mes en curso.

## 4. Calificación: 4 de 5, «lo pude usar sin ayuda»

Seguí el README sin preguntar a nadie y las tres tareas dieron resultados verificados. Resto un punto por tres cosas: la divisa EUR por defecto no documentada (F1), la falta de una receta para verificar por API (F8) y el ejemplo del paso 2 sin la confirmación que el propio README pide (F2). El flujo interactivo de Claude Code no lo probé (F11).

## 5. Incidentes (míos, no del README)

- **Cuenta duplicada.** Mi automatización creó «Bancolombia Ahorros» dos veces: el aviso de éxito de la primera cuenta tapó el segundo envío, y después volví a crearla en una depuración. Borré una con `POST /api/v1/accounts/delete.json`, que no tenía transacciones. Estado final verificado: 2 cuentas.
- **Diálogo equivocado.** Un selector ambiguo abrió «Añadir transacción» en lugar de «Añadir cuenta». Lo cerré sin guardar. La API confirma que solo hay 2 transacciones, las de `/recibos`.
- **Ruta adivinada.** Intenté `#/accounts`, que no sale en el README, y la página salió en blanco. La navegación desde la barra lateral sí funciona (`#/account/list`).
- **Entorno.** La herramienta de PowerShell devolvió un error de hook por tiempo agotado unas cinco veces; reintentar resolvió cada caso.

## 6. Pendientes (no borrado)

- **Historial de sesiones de `claude -p`:** 4 archivos en `C:\Users\juanc\.claude\projects\C--Users-juanc-AppData-Local-Temp-claude-C--Users-juanc-orca-workspaces-Finanzas-w-blind3-d780a3e5-cf27-4a8b-b9aa-8d604258a473-scratchpad-ws`. Están en el directorio global de Claude Code, así que no los toqué. `%USERPROFILE%\.claude.json` no tiene entradas del workspace temporal.
- **Imágenes Docker:** `ezbookkeeping-ai:local` (115 MB) y las imágenes base de build (`node:26.13.0-alpine3.24`, `golang:1.27.3-alpine3.24`) siguen en la caché. No estaban en la lista de limpieza.
- **Recibos sintéticos en el repo:** `testdata/receipts/blind3_panaderia.jpg` y `testdata/receipts/blind3_farmacia.jpg` van en este commit. Son datos de prueba con NIT y comercios ficticios.

## Anexo A: respuesta de `/insights` (mes en curso)

Respuesta literal de `claude -p "/insights"` (sin cambios):

---

## Insights de gastos: 01/11/2026 al 03/11/2026 (mes en curso, hasta hoy)

Tomé el mes en curso hasta hoy. Como solo lleva 3 días, comparé con el mismo número de días del periodo anterior: 29/10/2026 al 31/10/2026. Todas las cifras son en COP.

**Resultado: solo hay 2 gastos registrados en todo el periodo consultado (desde 01/08/2026).** Ambos son de noviembre. Los agregados no dan para tendencias, así que las secciones de abajo son parciales.

**a. Top de categorías (01/11 al 03/11)**

| Categoría | Total | Transacciones | Participación |
|---|---|---|---|
| Medicamentos | 67.300 | 1 | 78,4 % |
| Comida | 18.500 | 1 | 21,6 % |
| **Total** | **85.800** | **2** | 100 % |

**b. Variación frente al periodo anterior (29/10 al 31/10)**

- El periodo anterior tiene 0 gastos registrados. La variación en COP es +85.800 y en porcentaje no se puede calcular (división entre cero).
- Ojo: esto no significa que no gastaste nada en octubre. Significa que no hay gastos de octubre en ezBookkeeping.

**c. Gastos recurrentes o suscripciones**

- No hay. Cada comercio aparece una sola vez: «drogueria ficticia sas» (67.300) y «panaderia la prueba s a s» (18.500). Con un solo mes de datos no se puede identificar recurrencia.

**d. Anomalías**

- No puedo detectar anomalías: no hay promedio histórico con el que comparar. La compra de 67.300 en medicamentos es el 78 % del gasto del periodo, pero sin historial no sé si es habitual.

**e. Recomendaciones**

1. **Cargar el historial de octubre y septiembre.** Sin esos meses, cualquier comparación es vacía. Con `/extracto <ruta>` o con los extractos en la bandeja (`inbox`) podemos importarlos.
2. **Revisar la categoría de «drogueria ficticia sas» (67.300).** Si es medicamentos, bien. Si incluye otras compras del hogar, conviene separarla para no inflar esa categoría.
3. **Subir los recibos pendientes con `/recibos`.** Cada recibo que se procese suma al seguimiento. Hoy el dato más útil que falta es la continuidad mes a mes.

**Lo que no pude calcular:** recurrentes, anomalías y variación porcentual, por la falta de datos históricos. No consulté transacciones concretas con `query_transactions`, porque los dos gastos ya aparecen completos en los agregados.

---

## Anexo B: tabla de `/recibos` (propuesta en el paso 1 y aprobada)

| id | fecha | comercio | categoría | cuenta | monto |
|---|---|---|---|---|---|
| 3852085034131390465 | 03/11/2026 | Droguería Ficticia SAS | Medicina y Atención Sanitaria / Medicamentos | Bancolombia Ahorros | 67300 |
| 3852085034131390464 | 03/11/2026 | Panadería La Prueba S.A.S. | Comida y Bebida / Comida | Efectivo | 18500 |

Transacciones creadas por el paso 2: `3852085178818101248` (droguería) y `3852085178818101249` (panadería).
