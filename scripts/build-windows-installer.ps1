# Build the Windows installer for the openrgb-api project using Inno Setup and Go.

# Usage: .\build-windows-installer.ps1 -Version <version>
#
# Example: .\build-windows-installer.ps1 -Version 0.1.0
param(
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version = '0.0.0'
)

# Build script for Windows installer
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path $PSScriptRoot -Parent
$compiler = Get-Command ISCC.exe -ErrorAction SilentlyContinue

# Locate Inno Setup compiler
if (-not $compiler) {
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe')
    )
    $compilerPath = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not $compilerPath) {
        throw 'Inno Setup 6 was not found. Install it, then rerun this script.'
    }
} else {
    $compilerPath = $compiler.Source
}

# Set up directories and paths for build and output
$binaryDirectory = Join-Path $repositoryRoot 'build\artifacts\windows'
$outputDirectory = Join-Path $repositoryRoot 'release'
$binaryPath = Join-Path $binaryDirectory 'openrgb-api.exe'
$installerScript = Join-Path $repositoryRoot 'build\openrgb-api.iss'
New-Item -ItemType Directory -Force $binaryDirectory, $outputDirectory | Out-Null

# Build the Go binary and create the installer
Push-Location $repositoryRoot
try {
    go build -trimpath -o $binaryPath .\cmd\openrgb-api
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed with exit code $LASTEXITCODE."
    }

    & $compilerPath "/DAppVersion=$Version" $installerScript
    if ($LASTEXITCODE -ne 0) {
        throw "Inno Setup compilation failed with exit code $LASTEXITCODE."
    }
} finally {
    Pop-Location
    Remove-Item -Recurse -Force $binaryDirectory -ErrorAction SilentlyContinue
}
