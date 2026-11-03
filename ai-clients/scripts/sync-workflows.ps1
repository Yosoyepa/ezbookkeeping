<#
.SYNOPSIS
Copia shared/finanzas-workflows.md dentro de cada archivo de CLI, entre las marcas BEGIN SHARED y END SHARED.

.DESCRIPTION
Sin parámetros regenera los archivos. Con -Check no escribe nada y sale con 1 si algún archivo no coincide.
Produce exactamente los mismos bytes que scripts/sync-workflows.sh (LF, UTF-8 sin BOM).

.EXAMPLE
.\ai-clients\scripts\sync-workflows.ps1

.EXAMPLE
.\ai-clients\scripts\sync-workflows.ps1 -Check
#>
[CmdletBinding()]
param(
    [switch]$Check
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$src = Join-Path $root 'shared\finanzas-workflows.md'
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)

function Get-Lf([string]$text) {
    return $text -replace "`r`n", "`n"
}

$sharedLines = (Get-Lf ([System.IO.File]::ReadAllText($src))).TrimEnd("`n") -split "`n"

# Get-Rendered: texto del archivo con el bloque compartido regenerado
function Get-Rendered([string]$file) {
    $lines = (Get-Lf ([System.IO.File]::ReadAllText($file))) -split "`n"
    $begin = -1
    $end = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($begin -lt 0 -and $lines[$i].StartsWith('<!-- BEGIN SHARED')) {
            $begin = $i
        } elseif ($begin -ge 0 -and $end -lt 0 -and $lines[$i].StartsWith('<!-- END SHARED')) {
            $end = $i
        }
    }
    if ($begin -lt 0 -or $end -le $begin) {
        throw "sync-workflows: marcas BEGIN/END SHARED incompletas en $file"
    }
    $out = New-Object System.Collections.Generic.List[string]
    $out.AddRange([string[]]$lines[0..$begin])
    $out.AddRange([string[]]$sharedLines)
    $out.AddRange([string[]]$lines[$end..($lines.Count - 1)])
    return ($out -join "`n")
}

$targets = Get-ChildItem -Path (Join-Path $root 'claude-code'), (Join-Path $root 'agents') -Recurse -File -Force -Filter '*.md' |
    Where-Object { (Get-Lf ([System.IO.File]::ReadAllText($_.FullName))) -match '(?m)^<!-- BEGIN SHARED' } |
    Sort-Object FullName

if (-not $targets) {
    [Console]::Error.WriteLine('sync-workflows: no se encontraron archivos con el bloque compartido')
    exit 1
}

$status = 0
foreach ($t in $targets) {
    $rel = $t.FullName.Substring($root.Length + 1) -replace '\\', '/'
    $expected = Get-Rendered $t.FullName
    $current = Get-Lf ([System.IO.File]::ReadAllText($t.FullName))
    if ($Check) {
        if ($expected -eq $current) {
            Write-Output ('OK               ' + $rel)
        } else {
            Write-Output ('DESACTUALIZADO   ' + $rel)
            $status = 1
        }
    } else {
        [System.IO.File]::WriteAllText($t.FullName, $expected, $utf8NoBom)
        Write-Output ('generado         ' + $rel)
    }
}
exit $status
