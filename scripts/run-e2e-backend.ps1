$ErrorActionPreference = 'Stop'

$projectDir = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $projectDir 'backend'
$e2eDir = Join-Path $backendDir '.e2e'
$goExe = Join-Path $projectDir '.tools\go\bin\go.exe'

New-Item -ItemType Directory -Force -Path $e2eDir | Out-Null
foreach ($filename in @('data.db', 'data.db-wal', 'data.db-shm')) {
    $target = Join-Path $e2eDir $filename
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Force }
}

$env:PORT = ':6791'
$env:DB_PATH = (Join-Path $e2eDir 'data.db')
$env:UPLOAD_DIR = (Join-Path $e2eDir 'uploads')
$env:DEV_MODE = 'true'
$env:IMAGE_PROVIDER = 'mock'
$env:VECTORENGINE_BASE_URL = 'http://127.0.0.1:1'
$env:VECTORENGINE_API_KEY = 'mock-only'
$env:ASSISTANT_MODEL = 'mock-assistant'
$env:CANVAS_DEV_URL = 'http://127.0.0.1:5175'
$env:GOCACHE = (Join-Path $projectDir '.tools\cache\go-build')
$env:GOMODCACHE = (Join-Path $projectDir '.tools\cache\go-mod')

Set-Location $backendDir
& $goExe run .
