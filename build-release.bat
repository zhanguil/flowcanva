@echo off
setlocal enabledelayedexpansion
echo ============================================
echo   FlowCanva Release Build
echo ============================================
echo.

set ROOT=%~dp0
set RELEASE_DIR=%ROOT%release\flowcanva
set PACKAGE_MANAGER=

echo [1/7] Checking Node.js ...
set NODE_EXE=
for /f "delims=" %%I in ('where node.exe 2^>nul') do if not defined NODE_EXE set "NODE_EXE=%%I"
if not defined NODE_EXE (
    echo [ERROR] Node.js not found. Install: https://nodejs.org
    pause
    exit /b 1
)
for %%I in ("%NODE_EXE%") do set "PATH=%%~dpI;%PATH%"
echo Node.js OK

for /f "delims=" %%I in ('where npm.cmd 2^>nul') do if not defined PACKAGE_MANAGER set "PACKAGE_MANAGER=%%I"
if not defined PACKAGE_MANAGER (
    for /f "delims=" %%I in ('where pnpm.cmd 2^>nul') do if not defined PACKAGE_MANAGER set "PACKAGE_MANAGER=%%I"
)
if not defined PACKAGE_MANAGER (
    echo [ERROR] npm or pnpm was not found.
    pause
    exit /b 1
)
echo Package manager: %PACKAGE_MANAGER%

echo [2/7] Checking frontend dependencies ...
cd /d "%ROOT%frontend-admin"
if not exist "node_modules" (
    call "%PACKAGE_MANAGER%" install
    if %errorlevel% neq 0 ( echo [ERROR] frontend-admin install failed && pause && exit /b 1 )
) else (
    echo frontend-admin dependencies already installed
)
cd /d "%ROOT%frontend-canvas"
if not exist "node_modules" (
    call "%PACKAGE_MANAGER%" install
    if %errorlevel% neq 0 ( echo [ERROR] frontend-canvas install failed && pause && exit /b 1 )
) else (
    echo frontend-canvas dependencies already installed
)

echo [3/7] Building frontend ...
cd /d "%ROOT%frontend-admin"
call "%PACKAGE_MANAGER%" run build
if %errorlevel% neq 0 ( echo [ERROR] frontend-admin build failed && pause && exit /b 1 )
cd /d "%ROOT%frontend-canvas"
call "%PACKAGE_MANAGER%" run build
if %errorlevel% neq 0 ( echo [ERROR] frontend-canvas build failed && pause && exit /b 1 )

echo [4/7] Copying dist to embed dirs ...
call :reset_embed_dir "%ROOT%backend\admin-dist"
if %errorlevel% neq 0 exit /b 1
call :reset_embed_dir "%ROOT%backend\canvas-dist"
if %errorlevel% neq 0 exit /b 1

xcopy /e /i /q /y "%ROOT%frontend-admin\dist\*" "%ROOT%backend\admin-dist\" >nul
if %errorlevel% geq 2 ( echo [ERROR] Copying frontend-admin dist failed && pause && exit /b 1 )
xcopy /e /i /q /y "%ROOT%frontend-canvas\dist\*" "%ROOT%backend\canvas-dist\" >nul
if %errorlevel% geq 2 ( echo [ERROR] Copying frontend-canvas dist failed && pause && exit /b 1 )
type nul > "%ROOT%backend\admin-dist\keep.txt"
type nul > "%ROOT%backend\canvas-dist\keep.txt"

if not exist "%ROOT%backend\admin-dist\index.html" ( echo [ERROR] Admin index.html is missing && pause && exit /b 1 )
if not exist "%ROOT%backend\admin-dist\assets\*.js" ( echo [ERROR] Admin JavaScript assets are missing && pause && exit /b 1 )
if not exist "%ROOT%backend\canvas-dist\index.html" ( echo [ERROR] Canvas index.html is missing && pause && exit /b 1 )
if not exist "%ROOT%backend\canvas-dist\assets\*.js" ( echo [ERROR] Canvas JavaScript assets are missing && pause && exit /b 1 )
echo OK

echo [5/7] Compiling backend (Go 1.23+) ...
set GO_EXE=
for /f "delims=" %%I in ('where go.exe 2^>nul') do if not defined GO_EXE set "GO_EXE=%%I"
if not defined GO_EXE if exist "%ROOT%.tools\go\bin\go.exe" set "GO_EXE=%ROOT%.tools\go\bin\go.exe"
if not defined GO_EXE if exist "%ROOT%.tools\runtime\go1.25.3\go\bin\go.exe" set "GO_EXE=%ROOT%.tools\runtime\go1.25.3\go\bin\go.exe"
if not defined GO_EXE (
    echo [ERROR] Go not found. Install: https://go.dev/dl/
    pause
    exit /b 1
)
call :reset_embed_dir "%RELEASE_DIR%"
if %errorlevel% neq 0 exit /b 1
cd /d "%ROOT%backend"
"%GO_EXE%" build -ldflags "-s -w" -o "%RELEASE_DIR%\flowcanva.exe" .
if %errorlevel% neq 0 ( echo [ERROR] Go build failed && pause && exit /b 1 )

echo [6/7] Packaging release files ...
copy "%ROOT%README.md" "%RELEASE_DIR%\README.md" >nul
copy "%ROOT%LICENSE.md" "%RELEASE_DIR%\LICENSE.md" >nul

echo [7/7] Zipping release ...
if exist "%ROOT%release\flowcanva.zip" del /f /q "%ROOT%release\flowcanva.zip"
where tar.exe >nul 2>&1
if not errorlevel 1 (
    tar.exe -a -c -f "%ROOT%release\flowcanva.zip" -C "%RELEASE_DIR%" .
) else (
    powershell -NoProfile -Command "Compress-Archive -Path '%RELEASE_DIR%\*' -DestinationPath '%ROOT%release\flowcanva.zip' -Force"
)
if %errorlevel% neq 0 ( echo [ERROR] Release zip failed && pause && exit /b 1 )
if not exist "%ROOT%release\flowcanva.zip" ( echo [ERROR] Release zip was not created && pause && exit /b 1 )

echo.
echo ============================================
echo   BUILD SUCCESS
echo   Output: release\flowcanva\
echo   Zip:    release\flowcanva.zip
echo.
echo   Double-click flowcanva.exe to start
echo   No Go / Node.js required
echo ============================================
echo.
endlocal
exit /b 0

:reset_embed_dir
set "TARGET_DIR=%~1"
rd /s /q "%TARGET_DIR%" >nul 2>&1
if exist "%TARGET_DIR%" del /f /q "%TARGET_DIR%" >nul 2>&1
if exist "%TARGET_DIR%" (
    echo [ERROR] Could not clear embed path: %TARGET_DIR%
    pause
    exit /b 1
)
mkdir "%TARGET_DIR%"
if %errorlevel% neq 0 (
    echo [ERROR] Could not create embed directory: %TARGET_DIR%
    pause
    exit /b 1
)
exit /b 0
