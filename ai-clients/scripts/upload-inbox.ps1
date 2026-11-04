<#
.SYNOPSIS
Sube recibos y extractos a la bandeja de ezBookkeeping desde el PC, por la API.

.DESCRIPTION
Uso: upload-inbox.ps1 [-Kind receipt|statement] <archivo>...

El tipo se deduce de la extensión: imágenes (jpg, jpeg, png, gif, webp) -> receipt; csv, txt, ofx, qif, pdf, xlsx, xls -> statement.
-Kind lo fuerza para todos los archivos. Lee la URL de EBK_URL (por defecto http://127.0.0.1:8080) y el token de EBK_API_TOKEN,
un token de tipo api (userdata user-session-new -n <usuario> -t api). El token nunca se imprime ni se guarda en el repo.
Por cada ítem creado imprime "<id> <kind>". Códigos de salida: 0 todo subido; 1 falló algún archivo; 2 falta configuración o argumentos.

.EXAMPLE
$env:EBK_API_TOKEN = '<token api>'
.\ai-clients\scripts\upload-inbox.ps1 recibo1.jpg recibo2.jpg
.\ai-clients\scripts\upload-inbox.ps1 -Kind statement .\extracto.txt
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [ValidateSet('receipt', 'statement')]
    [string]$Kind,

    [Parameter(Position = 0, ValueFromRemainingArguments = $true)]
    [string[]]$Files
)

$ErrorActionPreference = 'Stop'
$baseUrl = if ($env:EBK_URL) { $env:EBK_URL.TrimEnd('/') } else { 'http://127.0.0.1:8080' }
$imageExtensions = @('jpg', 'jpeg', 'png', 'gif', 'webp')
$statementExtensions = @('csv', 'txt', 'ofx', 'qif', 'pdf', 'xlsx', 'xls')

if (-not $env:EBK_API_TOKEN) {
    [Console]::Error.WriteLine('Falta la variable de entorno EBK_API_TOKEN (token api; no se guarda en el repo).')
    exit 2
}
if (-not $Files -or $Files.Count -eq 0) {
    [Console]::Error.WriteLine('Uso: upload-inbox.ps1 [-Kind receipt|statement] <archivo>...')
    exit 2
}

Add-Type -AssemblyName System.Net.Http

$client = New-Object System.Net.Http.HttpClient
$client.Timeout = [TimeSpan]::FromMinutes(5)
$client.DefaultRequestHeaders.Authorization = New-Object System.Net.Http.Headers.AuthenticationHeaderValue('Bearer', $env:EBK_API_TOKEN)

# Get-ItemKind: receipt o statement según la extensión; $null si no se reconoce
function Get-ItemKind([string]$path) {
    $extension = [IO.Path]::GetExtension($path).TrimStart('.').ToLowerInvariant()
    if ($imageExtensions -contains $extension) { return 'receipt' }
    if ($statementExtensions -contains $extension) { return 'statement' }
    return $null
}

# Send-File: sube un archivo y devuelve el ítem creado; lanza una excepción con el motivo si falla
function Send-File([string]$path, [string]$itemKind) {
    $stream = [IO.File]::OpenRead($path)
    $content = New-Object System.Net.Http.MultipartFormDataContent
    try {
        $kindNumber = if ($itemKind -eq 'receipt') { '1' } else { '2' }
        $content.Add((New-Object System.Net.Http.StringContent($kindNumber)), 'kind')
        $content.Add((New-Object System.Net.Http.StreamContent($stream)), 'file', [IO.Path]::GetFileName($path))

        $response = $client.PostAsync($baseUrl + '/api/v1/inbox/items/upload.json', $content).GetAwaiter().GetResult()
        $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

        $body = $null
        try { $body = $text | ConvertFrom-Json } catch { }

        if (-not $response.IsSuccessStatusCode -or $null -eq $body -or -not $body.success) {
            $reason = if ($null -ne $body -and $body.errorMessage) { [string]$body.errorMessage } else { 'respuesta inesperada' }
            throw ('HTTP ' + [int]$response.StatusCode + ': ' + $reason)
        }
        return $body.result
    } finally {
        $content.Dispose()
        $stream.Dispose()
    }
}

$failed = 0
try {
    foreach ($file in $Files) {
        try {
            if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
                throw 'el archivo no existe'
            }
            $itemKind = $Kind
            if (-not $itemKind) {
                $itemKind = Get-ItemKind $file
            }
            if (-not $itemKind) {
                throw 'extensión no reconocida (usa -Kind receipt o -Kind statement)'
            }
            $item = Send-File $file $itemKind
            $kindName = if ([int]$item.kind -eq 1) { 'receipt' } else { 'statement' }
            Write-Output ([string]$item.id + ' ' + $kindName)
        } catch {
            $failed++
            [Console]::Error.WriteLine('ERROR ' + $file + ': ' + $_.Exception.Message)
        }
    }
} finally {
    $client.Dispose()
}

if ($failed -gt 0) {
    [Console]::Error.WriteLine('Resultado: ' + $failed + ' de ' + $Files.Count + ' archivo(s) no se subieron.')
    exit 1
}
exit 0
