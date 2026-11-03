<#
.SYNOPSIS
Comprueba que el servidor MCP de ezBookkeeping responde y expone las herramientas de Finanzas.

.DESCRIPTION
Lee el token de EBK_MCP_TOKEN (nunca se guarda en el repo) y la URL de EBK_MCP_URL (por defecto http://127.0.0.1:8080/mcp).
Envía initialize, notifications/initialized y tools/list, y comprueba que están las herramientas de Finanzas.
Produce la misma salida que scripts/check-mcp.sh. Códigos de salida: 0 todo correcto; 1 falta una herramienta
o falla una petición; 2 falta configuración.

.EXAMPLE
$env:EBK_MCP_TOKEN = '<token MCP>'
.\ai-clients\scripts\check-mcp.ps1
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$protocol = '2025-06-18'
$required = @(
    'list_inbox_items', 'get_inbox_item', 'resolve_inbox_item', 'discard_inbox_item',
    'add_transactions_batch', 'update_transaction', 'get_spending_summary',
    'query_transactions', 'query_all_accounts', 'query_all_transaction_categories'
)
$url = if ($env:EBK_MCP_URL) { $env:EBK_MCP_URL } else { 'http://127.0.0.1:8080/mcp' }

if (-not $env:EBK_MCP_TOKEN) {
    [Console]::Error.WriteLine('Falta la variable de entorno EBK_MCP_TOKEN (token MCP; no se guarda en el repo).')
    exit 2
}

$headers = @{
    'Authorization'        = 'Bearer ' + $env:EBK_MCP_TOKEN
    'Accept'               = 'application/json, text/event-stream'
    'MCP-Protocol-Version' = $protocol
}

function Fail([string]$message) {
    [Console]::Error.WriteLine('ERROR: ' + $message)
    exit 1
}

# Send-Rpc: envía una petición JSON-RPC y devuelve el código HTTP y el cuerpo (Status 0 = sin conexión)
function Send-Rpc([hashtable]$body) {
    $json = $body | ConvertTo-Json -Depth 10 -Compress
    try {
        $resp = Invoke-WebRequest -Uri $url -Method Post -Headers $headers -ContentType 'application/json' -Body $json -UseBasicParsing
        return @{ Status = [int]$resp.StatusCode; Content = [string]$resp.Content; Error = '' }
    } catch {
        $status = 0
        $response = $_.Exception.Response
        if ($null -ne $response) {
            $status = [int]$response.StatusCode
        }
        return @{ Status = $status; Content = ''; Error = $_.Exception.Message }
    }
}

# ConvertFrom-Rpc: convierte el cuerpo (JSON o text/event-stream) en un objeto
function ConvertFrom-Rpc([string]$content) {
    $text = $content.Trim()
    if (-not $text.StartsWith('{')) {
        $dataLines = @($text -split "`n" | Where-Object { $_.Trim().StartsWith('data:') })
        if ($dataLines.Count -eq 0) {
            return $null
        }
        $text = $dataLines[-1].Trim().Substring(5).Trim()
    }
    return ($text | ConvertFrom-Json)
}

$init = Send-Rpc @{ jsonrpc = '2.0'; id = 1; method = 'initialize'; params = @{ protocolVersion = $protocol; capabilities = @{}; clientInfo = @{ name = 'check-mcp'; version = '1.0.0' } } }
if ($init.Status -eq 0) {
    Fail ('no hay conexión con ' + $url + ' (¿está levantado el servidor?). Detalle: ' + $init.Error)
}
if ($init.Status -ne 200) {
    Fail ('initialize devolvió HTTP ' + $init.Status + ' (revisa EBK_MCP_TOKEN y la URL)')
}
$initResult = ConvertFrom-Rpc $init.Content
if ($null -eq $initResult -or $null -eq $initResult.result -or [string]::IsNullOrEmpty($initResult.result.protocolVersion)) {
    Fail 'initialize no devolvió protocolVersion'
}
Write-Output ('initialize ........ OK (HTTP 200, protocolo ' + $initResult.result.protocolVersion + ')')

$notif = Send-Rpc @{ jsonrpc = '2.0'; method = 'notifications/initialized' }
if ($notif.Status -ne 202) {
    Fail ('notifications/initialized devolvió HTTP ' + $notif.Status + ' (se esperaba 202)')
}

$list = Send-Rpc @{ jsonrpc = '2.0'; id = 2; method = 'tools/list'; params = @{} }
if ($list.Status -ne 200) {
    Fail ('tools/list devolvió HTTP ' + $list.Status)
}
$listResult = ConvertFrom-Rpc $list.Content
if ($null -eq $listResult -or $null -eq $listResult.result) {
    Fail 'tools/list devolvió un error JSON-RPC'
}
$names = @($listResult.result.tools | ForEach-Object { $_.name })
Write-Output ('tools/list ........ OK (' + $names.Count + ' herramientas)')

$missing = 0
foreach ($tool in $required) {
    if ($names -contains $tool) {
        Write-Output ('  OK      ' + $tool)
    } else {
        Write-Output ('  FALTA   ' + $tool)
        $missing++
    }
}

if ($missing -gt 0) {
    Write-Output ('Resultado: faltan ' + $missing + ' herramienta(s) de Finanzas. El servidor no está listo.')
    exit 1
}
Write-Output ('Resultado: las ' + $required.Count + ' herramientas de Finanzas están disponibles.')
exit 0
