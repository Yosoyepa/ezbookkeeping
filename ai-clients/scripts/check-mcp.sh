#!/usr/bin/env bash
# Comprueba que el servidor MCP de ezBookkeeping responde y expone las herramientas de Finanzas.
#   EBK_MCP_TOKEN=<token> [EBK_MCP_URL=http://127.0.0.1:8080/mcp] scripts/check-mcp.sh
# Códigos de salida: 0 todo correcto; 1 falta una herramienta o falla una petición; 2 falta configuración.
set -u

URL="${EBK_MCP_URL:-http://127.0.0.1:8080/mcp}"
PROTOCOL="2025-06-18"
REQUIRED="list_inbox_items get_inbox_item resolve_inbox_item discard_inbox_item add_transactions_batch update_transaction get_spending_summary query_transactions query_all_accounts query_all_transaction_categories"

if [ -z "${EBK_MCP_TOKEN:-}" ]; then
    echo "Falta la variable de entorno EBK_MCP_TOKEN (token MCP; no se guarda en el repo)." >&2
    exit 2
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
    echo "ERROR: $1" >&2
    exit 1
}

# post <cuerpo JSON> <archivo de salida>: envía una petición y muestra el código HTTP (000 si no hay conexión)
post() {
    curl -sS -o "$2" -w '%{http_code}' \
        -H "Authorization: Bearer ${EBK_MCP_TOKEN}" \
        -H "Accept: application/json, text/event-stream" \
        -H "Content-Type: application/json" \
        -H "MCP-Protocol-Version: ${PROTOCOL}" \
        --data "$1" "$URL" 2>"$TMP/curl.err" || true
}

code="$(post "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"${PROTOCOL}\",\"capabilities\":{},\"clientInfo\":{\"name\":\"check-mcp\",\"version\":\"1.0.0\"}}}" "$TMP/init.json")"
if [ "$code" = "000" ]; then
    fail "no hay conexión con $URL (¿está levantado el servidor?). Detalle: $(cat "$TMP/curl.err")"
fi
if [ "$code" != "200" ]; then
    fail "initialize devolvió HTTP $code (revisa EBK_MCP_TOKEN y la URL)"
fi
version="$(tr -d ' \t\r\n' < "$TMP/init.json" | sed -n 's/.*"protocolVersion":"\([^"]*\)".*/\1/p')"
if [ -z "$version" ]; then
    fail "initialize no devolvió protocolVersion"
fi
echo "initialize ........ OK (HTTP 200, protocolo $version)"

code="$(post '{"jsonrpc":"2.0","method":"notifications/initialized"}' "$TMP/notif.json")"
if [ "$code" != "202" ]; then
    fail "notifications/initialized devolvió HTTP $code (se esperaba 202)"
fi

code="$(post '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' "$TMP/tools.json")"
if [ "$code" != "200" ]; then
    fail "tools/list devolvió HTTP $code"
fi
if tr -d ' \t\r\n' < "$TMP/tools.json" | grep -q '"error":{'; then
    fail "tools/list devolvió un error JSON-RPC"
fi

NAMES="$(tr -d ' \t\r\n' < "$TMP/tools.json" | grep -o '"name":"[^"]*"' | sed 's/^"name":"//; s/"$//' || true)"
COUNT="$(printf '%s\n' "$NAMES" | grep -c . || true)"
echo "tools/list ........ OK ($COUNT herramientas)"

missing=0
for tool in $REQUIRED; do
    if printf '%s\n' "$NAMES" | grep -qx "$tool"; then
        echo "  OK      $tool"
    else
        echo "  FALTA   $tool"
        missing=$((missing + 1))
    fi
done

if [ "$missing" -gt 0 ]; then
    echo "Resultado: faltan $missing herramienta(s) de Finanzas. El servidor no está listo."
    exit 1
fi
echo "Resultado: las $(echo $REQUIRED | wc -w | tr -d ' ') herramientas de Finanzas están disponibles."
exit 0
