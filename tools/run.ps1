# =============================================================================
# SimpleHTTPRedirector - Tools Container Launcher (PowerShell)
# =============================================================================

param(
    [Parameter(Position=0)]
    [ValidateSet("generate-env", "shell", "help")]
    [string]$Command = "help",

    [Alias("b")]
    [switch]$Build
)

$ErrorActionPreference = "Stop"

# Configuration
$ToolsDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ToolsDir
$ImageName = "redirector-tools"

# Check if Docker is running
try {
    docker info 2>&1 | Out-Null
} catch {
    Write-Host "[ERROR] Docker is not running. Please start Docker Desktop first." -ForegroundColor Red
    exit 1
}

# Show help
function Show-Help {
    Write-Host "Usage: .\run.ps1 [COMMAND] [OPTIONS]"
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  generate-env     Generate .env from redirects.json"
    Write-Host "  shell            Start interactive shell in tools container"
    Write-Host "  help             Show this help"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Build, -b       Rebuild the tools container"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\run.ps1 generate-env              Generate .env file"
    Write-Host "  .\run.ps1 generate-env -Build       Rebuild container and generate .env"
    Write-Host "  .\run.ps1 shell                     Start interactive shell"
    exit 0
}

# Handle help command
if ($Command -eq "help") {
    Show-Help
}

# Build if requested or image doesn't exist
$imageExists = $false
try {
    docker image inspect $ImageName 2>&1 | Out-Null
    $imageExists = ($LASTEXITCODE -eq 0)
} catch {
    $imageExists = $false
}

if ($Build -or -not $imageExists) {
    Write-Host "[INFO] Building tools container..." -ForegroundColor Cyan
    docker build -t $ImageName "$ToolsDir"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to build tools container" -ForegroundColor Red
        exit 1
    }
    Write-Host ""
}

# Execute command
switch ($Command) {
    "generate-env" {
        Write-Host "[INFO] Generating .env from redirects.json..." -ForegroundColor Cyan
        docker run --rm `
            -v "${ProjectDir}:/workspace" `
            -w /workspace `
            $ImageName `
            /bin/bash /workspace/scripts/generate-env.sh
    }
    "shell" {
        Write-Host "===========================================" -ForegroundColor Green
        Write-Host " SimpleHTTPRedirector Tools" -ForegroundColor Green
        Write-Host "===========================================" -ForegroundColor Green
        Write-Host ""
        Write-Host "Available scripts:"
        Write-Host "  ./scripts/generate-env.sh  - Generate .env from config" -ForegroundColor Yellow
        Write-Host ""
        Write-Host "Type 'exit' to leave the container."
        Write-Host "===========================================" -ForegroundColor Green
        Write-Host ""
        docker run -it --rm `
            -v "${ProjectDir}:/workspace" `
            -w /workspace `
            $ImageName
    }
}
