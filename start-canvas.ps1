$ErrorActionPreference = 'Stop'

$projectDir = $PSScriptRoot
$backendDir = Join-Path $projectDir 'backend'
$serverExe = Join-Path $backendDir 'flowcanva.exe'
$canvasUrl = 'http://127.0.0.1:6789/canvas'

function Test-FlowCanvaServer {
    try {
        $response = Invoke-WebRequest -Uri 'http://127.0.0.1:6789/api/canvases?page_size=1' -TimeoutSec 2
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

if (-not (Test-FlowCanvaServer)) {
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "未找到 FlowCanva 后端程序：$serverExe"
    }
    Start-Process -FilePath $serverExe -WorkingDirectory $backendDir -WindowStyle Hidden

    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        Start-Sleep -Milliseconds 500
        if (Test-FlowCanvaServer) {
            $ready = $true
            break
        }
    }
    if (-not $ready) {
        throw 'FlowCanva 后端启动超时，请检查 backend/logs。'
    }
}

Start-Process $canvasUrl
