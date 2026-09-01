$ErrorActionPreference = 'Stop'

$projectDir = $PSScriptRoot
$backendDir = Join-Path $projectDir 'backend'
$serverExe = Join-Path $backendDir 'flowcanva.exe'
$canvasUrl = 'http://127.0.0.1:6789/canvas'

function Test-FlowCanvaServer {
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:6789/api/canvases?page_size=1' -TimeoutSec 2
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

if (-not (Test-FlowCanvaServer)) {
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "FlowCanva backend executable was not found: $serverExe"
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
        throw 'FlowCanva backend startup timed out. Check backend/logs.'
    }
}

$chromeCandidates = @(
    (Join-Path $env:LOCALAPPDATA 'Google\Chrome\Application\chrome.exe'),
    (Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'Google\Chrome\Application\chrome.exe')
)
$chromeExe = $chromeCandidates | Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -First 1
$edgeExe = Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe'

if ($chromeExe) {
    Start-Process -FilePath $chromeExe -ArgumentList @('--new-tab', $canvasUrl)
}
elseif (Test-Path -LiteralPath $edgeExe) {
    Start-Process -FilePath $edgeExe -ArgumentList @('--new-tab', $canvasUrl)
}
else {
    Start-Process $canvasUrl
}
