#!/usr/bin/env bash
# Copia shared/finanzas-workflows.md dentro de cada archivo de CLI, entre las marcas BEGIN SHARED y END SHARED.
#   scripts/sync-workflows.sh          regenera los archivos
#   scripts/sync-workflows.sh --check  no escribe nada; sale con 1 si algún archivo no coincide con el original
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/shared/finanzas-workflows.md"
CHECK=0
if [ "${1:-}" = "--check" ]; then
    CHECK=1
fi

# render <archivo>: imprime el archivo con el bloque compartido regenerado (finales de línea LF)
render() {
    local file="$1" begin end
    begin="$(grep -n '^<!-- BEGIN SHARED' "$file" | head -n 1 | cut -d: -f1 || true)"
    end="$(grep -n '^<!-- END SHARED' "$file" | head -n 1 | cut -d: -f1 || true)"
    if [ -z "$begin" ] || [ -z "$end" ] || [ "$end" -le "$begin" ]; then
        echo "sync-workflows: marcas BEGIN/END SHARED incompletas en $file" >&2
        exit 1
    fi
    {
        head -n "$begin" "$file"
        cat "$SRC"
        tail -n +"$end" "$file"
    } | tr -d '\r'
}

# targets: archivos de CLI que contienen el bloque compartido
targets() {
    find "$ROOT/claude-code" "$ROOT/agents" -type f -name '*.md' | sort | while IFS= read -r f; do
        if grep -q '^<!-- BEGIN SHARED' "$f"; then
            printf '%s\n' "$f"
        fi
    done
}

status=0
count=0
while IFS= read -r f; do
    count=$((count + 1))
    rel="${f#"$ROOT"/}"
    if [ "$CHECK" -eq 1 ]; then
        if cmp -s <(render "$f") <(tr -d '\r' < "$f"); then
            echo "OK               $rel"
        else
            echo "DESACTUALIZADO   $rel"
            status=1
        fi
    else
        render "$f" > "$f.new"
        mv "$f.new" "$f"
        echo "generado         $rel"
    fi
done < <(targets)

if [ "$count" -eq 0 ]; then
    echo "sync-workflows: no se encontraron archivos con el bloque compartido" >&2
    exit 1
fi
exit "$status"
