@echo off
REM =============================================================================
REM SimpleHTTPRedirector - Tools Container Launcher (Windows CMD)
REM =============================================================================

setlocal enabledelayedexpansion

REM Get the project directory (parent of tools folder)
set "TOOLS_DIR=%~dp0"
set "TOOLS_DIR=%TOOLS_DIR:~0,-1%"
for %%I in ("%TOOLS_DIR%\..") do set "PROJECT_DIR=%%~fI"

REM Container/image name
set "IMAGE_NAME=redirector-tools"

REM Check if Docker is running
docker info >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Docker is not running. Please start Docker Desktop first.
    exit /b 1
)

REM Parse arguments
set "COMMAND="
set "BUILD=false"

:parse_args
if "%1"=="" goto :check_command
if /i "%1"=="generate-env" (
    set "COMMAND=generate-env"
    shift
    goto :parse_args
)
if /i "%1"=="shell" (
    set "COMMAND=shell"
    shift
    goto :parse_args
)
if /i "%1"=="--build" (
    set "BUILD=true"
    shift
    goto :parse_args
)
if /i "%1"=="-b" (
    set "BUILD=true"
    shift
    goto :parse_args
)
if /i "%1"=="--help" goto :show_help
if /i "%1"=="-h" goto :show_help
if /i "%1"=="help" goto :show_help

echo [ERROR] Unknown option: %1
goto :show_help

:check_command
if "%COMMAND%"=="" goto :show_help

REM Build if requested or image doesn't exist
docker image inspect %IMAGE_NAME% >nul 2>&1
if errorlevel 1 set "BUILD=true"

if "%BUILD%"=="true" (
    echo [INFO] Building tools container...
    docker build -t %IMAGE_NAME% "%TOOLS_DIR%"
    if errorlevel 1 (
        echo [ERROR] Failed to build tools container
        exit /b 1
    )
    echo.
)

REM Execute command
if "%COMMAND%"=="generate-env" (
    echo [INFO] Generating .env from redirects.json...
    docker run --rm ^
        -v "%PROJECT_DIR%:/workspace" ^
        -w /workspace ^
        %IMAGE_NAME% ^
        /bin/bash /workspace/scripts/generate-env.sh
    goto :eof
)

if "%COMMAND%"=="shell" (
    echo ===========================================
    echo  SimpleHTTPRedirector Tools
    echo ===========================================
    echo.
    echo Available scripts:
    echo   ./scripts/generate-env.sh  - Generate .env from config
    echo.
    echo Type 'exit' to leave the container.
    echo ===========================================
    echo.
    docker run -it --rm ^
        -v "%PROJECT_DIR%:/workspace" ^
        -w /workspace ^
        %IMAGE_NAME%
    goto :eof
)

goto :eof

:show_help
echo Usage: run.cmd [COMMAND] [OPTIONS]
echo.
echo Commands:
echo   generate-env     Generate .env from redirects.json
echo   shell            Start interactive shell in tools container
echo   help             Show this help
echo.
echo Options:
echo   --build, -b      Rebuild the tools container
echo.
echo Examples:
echo   run.cmd generate-env              Generate .env file
echo   run.cmd generate-env --build      Rebuild container and generate .env
echo   run.cmd shell                     Start interactive shell
exit /b 0

endlocal
