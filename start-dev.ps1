Write-Host "=== FlowCanva 开发模式启动 ===" -ForegroundColor Cyan
Write-Host ""

$root = Split-Path -Parent $MyInvocation.MyCommand.Path

$jobs = @()

$goCommand = Get-Command go.exe -ErrorAction SilentlyContinue
$goExe = if ($goCommand) { $goCommand.Source } else { Join-Path $root '.tools\go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExe)) {
    throw 'Go 运行环境未找到。请安装 Go，或保留项目内的 .tools\go。'
}

$nodeCommand = Get-Command node.exe -ErrorAction SilentlyContinue
if (-not $nodeCommand) {
    throw 'Node.js 运行环境未找到。'
}
$nodeExe = $nodeCommand.Source
$adminVite = Join-Path $root 'frontend-admin\node_modules\vite\bin\vite.js'
$canvasVite = Join-Path $root 'frontend-canvas\node_modules\vite\bin\vite.js'
if (-not (Test-Path -LiteralPath $adminVite) -or -not (Test-Path -LiteralPath $canvasVite)) {
    throw '前端依赖未安装。请先在两个 frontend 目录执行 npm install。'
}

$env:DEV_MODE = "true"
$env:SERVER_HOST = "0.0.0.0"
$env:PORT = "6789"
$env:VITE_HOST = "0.0.0.0"
$env:VITE_PORT = "5173"
$env:VITE_ADMIN_PORT = "5174"
$jobs += Start-Process -FilePath $goExe -ArgumentList "run ." -WorkingDirectory "$root\backend" -PassThru -WindowStyle Hidden
Write-Host "✓ 后端代理 0.0.0.0:6789" -ForegroundColor Green

Start-Sleep -Seconds 2

$jobs += Start-Process -FilePath $nodeExe -ArgumentList "`"$adminVite`"" -WorkingDirectory "$root\frontend-admin" -PassThru -WindowStyle Hidden
Write-Host "✓ 控制台 0.0.0.0:5174" -ForegroundColor Green

$jobs += Start-Process -FilePath $nodeExe -ArgumentList "`"$canvasVite`"" -WorkingDirectory "$root\frontend-canvas" -PassThru -WindowStyle Hidden
Write-Host "✓ 画布   0.0.0.0:5173" -ForegroundColor Green

$lanIPv4 = Get-NetIPConfiguration |
    Where-Object { $_.IPv4DefaultGateway -and $_.IPv4Address } |
    ForEach-Object { $_.IPv4Address.IPAddress } |
    Where-Object { $_ -and $_ -notlike '127.*' } |
    Select-Object -First 1

Write-Host ""
Write-Host "统一入口:" -ForegroundColor Cyan
Write-Host "  http://127.0.0.1:6789   → 本机访问" -ForegroundColor Yellow
if ($lanIPv4) {
    Write-Host "  http://${lanIPv4}:6789   → 局域网访问" -ForegroundColor Yellow
}
else {
    Write-Host "  未检测到带默认网关的局域网 IPv4 地址" -ForegroundColor DarkYellow
}
Write-Host ""
Write-Host "按任意键停止所有服务..." -ForegroundColor DarkGray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")

foreach ($job in $jobs) {
    if (-not $job.HasExited) {
        Stop-Process -Id $job.Id -Force -ErrorAction SilentlyContinue
    }
}
Write-Host "所有服务已停止" -ForegroundColor Red
