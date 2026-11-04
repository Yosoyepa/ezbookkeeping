#!/usr/bin/env bash
# Sube recibos y extractos a la bandeja de ezBookkeeping desde el PC, por la API.
#   EBK_API_TOKEN=<token api> [EBK_URL=http://127.0.0.1:8080] scripts/upload-inbox.sh [--kind receipt|statement] <archivo>...
# El tipo se deduce de la extensión: imágenes (jpg, jpeg, png, gif, webp) -> receipt; csv, txt, ofx, qif, pdf, xlsx, xls -> statement.
# --kind lo fuerza para todos los archivos. EBK_API_TOKEN es un token de tipo api (userdata user-session-new -n <usuario> -t api);
# nunca se imprime ni se pone en la línea de comandos de curl. Por cada ítem creado imprime "<id> <kind>".
# Códigos de salida: 0 todo subido; 1 falló algún archivo; 2 falta configuración o argumentos.
set -u

BASE_URL="${EBK_URL:-http://127.0.0.1:8080}"
BASE_URL="${BASE_URL%/}"
KIND=""

while [ $# -gt 0 ]; do
    case "$1" in
        --kind)
            [ $# -ge 2 ] || { echo "Falta el valor de --kind (receipt o statement)." >&2; exit 2; }
            KIND="$2"
            shift 2
            ;;
        --kind=*)
            KIND="${1#--kind=}"
            shift
            ;;
        --)
            shift
            break
            ;;
        *)
            break
            ;;
    esac
done

if [ -n "$KIND" ] && [ "$KIND" != "receipt" ] && [ "$KIND" != "statement" ]; then
    echo "--kind debe ser receipt o statement." >&2
    exit 2
fi
if [ -z "${EBK_API_TOKEN:-}" ]; then
    echo "Falta la variable de entorno EBK_API_TOKEN (token api; no se guarda en el repo)." >&2
    exit 2
fi
if [ $# -eq 0 ]; then
    echo "Uso: upload-inbox.sh [--kind receipt|statement] <archivo>..." >&2
    exit 2
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# kind_of <archivo>: receipt o statement según la extensión; vacío si no se reconoce
kind_of() {
    local ext
    ext="$(printf '%s' "${1##*.}" | tr '[:upper:]' '[:lower:]')"
    case "$ext" in
        jpg|jpeg|png|gif|webp) echo receipt ;;
        csv|txt|ofx|qif|pdf|xlsx|xls) echo statement ;;
    esac
}

failed=0
total=0
for file in "$@"; do
    total=$((total + 1))
    if [ ! -f "$file" ]; then
        echo "ERROR $file: el archivo no existe" >&2
        failed=$((failed + 1))
        continue
    fi
    item_kind="$KIND"
    [ -n "$item_kind" ] || item_kind="$(kind_of "$file")"
    if [ -z "$item_kind" ]; then
        echo "ERROR $file: extensión no reconocida (usa --kind receipt o --kind statement)" >&2
        failed=$((failed + 1))
        continue
    fi
    kind_number=2
    [ "$item_kind" = "receipt" ] && kind_number=1

    # la cabecera con el token entra por stdin (-K -) para que no aparezca en la lista de procesos
    code="$(printf 'header = "Authorization: Bearer %s"\n' "$EBK_API_TOKEN" | curl -sS -K - \
        -o "$TMP/response.json" -w '%{http_code}' \
        -F "kind=${kind_number}" -F "file=@${file}" \
        "$BASE_URL/api/v1/inbox/items/upload.json" 2>"$TMP/curl.err")" || code="000"
    if [ "$code" = "000" ]; then
        echo "ERROR $file: no hay conexión con $BASE_URL ($(tr -d '\r\n' < "$TMP/curl.err"))" >&2
        failed=$((failed + 1))
        continue
    fi

    body="$(tr -d '\r\n' < "$TMP/response.json")"
    if [ "$code" != "200" ] || ! printf '%s' "$body" | grep -q '"success":true'; then
        reason="$(printf '%s' "$body" | sed -n 's/.*"errorMessage":"\([^"]*\)".*/\1/p')"
        echo "ERROR $file: HTTP $code: ${reason:-respuesta inesperada}" >&2
        failed=$((failed + 1))
        continue
    fi

    id="$(printf '%s' "$body" | sed -n 's/.*"id":"\([0-9]*\)".*/\1/p')"
    kind_value="$(printf '%s' "$body" | sed -n 's/.*"kind":\([0-9]*\).*/\1/p')"
    kind_name=statement
    [ "$kind_value" = "1" ] && kind_name=receipt
    echo "$id $kind_name"
done

if [ "$failed" -gt 0 ]; then
    echo "Resultado: $failed de $total archivo(s) no se subieron." >&2
    exit 1
fi
exit 0
