# Build the Linux installer for the openrgb-api project using Go and tar.
#
# Usage: .\build-linux-installer.ps1 -Version <version> -Architecture <architecture>
#
# Example: .\build-linux-installer.ps1 -Version 0.1.0 -Architecture amd64
param(
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version = '0.0.0',

    [ValidateSet('amd64', 'arm64')]
    [string]$Architecture = 'amd64'
)

# Set error action preference and define paths for build and output
$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path $PSScriptRoot -Parent
$packageName = "openrgb-api-$Version-linux-$Architecture"
$buildRoot = Join-Path $repositoryRoot 'build'
$stagingRoot = Join-Path $buildRoot 'linux'
$stagingDirectory = Join-Path $stagingRoot $packageName
$outputDirectory = Join-Path $repositoryRoot 'release'
$archivePath = Join-Path $outputDirectory "$packageName.tar.gz"

# Ensure required tools are available
if (-not (Get-Command tar -ErrorAction SilentlyContinue)) {
    throw 'tar was not found. Windows 10 and later include it; otherwise install bsdtar.'
}
# Clean up any existing staging directory and create necessary directories
if (Test-Path $stagingDirectory) {
    Remove-Item -Recurse -Force $stagingDirectory
}
New-Item -ItemType Directory -Force $stagingDirectory, $outputDirectory | Out-Null
# Build the Go binary for Linux and prepare the staging directory
try {
    $previousEnvironment = @{
        GOOS        = $env:GOOS
        GOARCH      = $env:GOARCH
        CGO_ENABLED = $env:CGO_ENABLED
    }
    Push-Location $repositoryRoot
    try {
        $env:GOOS = 'linux'
        $env:GOARCH = $Architecture
        $env:CGO_ENABLED = '0'
        go build -trimpath -ldflags '-s -w' -o (Join-Path $stagingDirectory 'openrgb-api') .\cmd\openrgb-api
        if ($LASTEXITCODE -ne 0) {
            throw "Go build failed with exit code $LASTEXITCODE."
        }
    } finally {
        $env:GOOS = $previousEnvironment.GOOS
        $env:GOARCH = $previousEnvironment.GOARCH
        $env:CGO_ENABLED = $previousEnvironment.CGO_ENABLED
        Pop-Location
    }
    # Copy additional files to the staging directory
    Copy-Item (Join-Path $repositoryRoot 'build\linux\install.sh') $stagingDirectory
    Copy-Item (Join-Path $repositoryRoot 'build\linux\uninstall.sh') $stagingDirectory
    Copy-Item (Join-Path $repositoryRoot 'build\linux\openrgb-api.service') $stagingDirectory
    Copy-Item (Join-Path $repositoryRoot 'build\config-local.toml') $stagingDirectory
    Copy-Item (Join-Path $repositoryRoot 'build\config-lan.toml') $stagingDirectory

    # Shell scripts must use LF line endings to run on Linux.
    foreach ($script in 'install.sh', 'uninstall.sh') {
        $scriptPath = Join-Path $stagingDirectory $script
        $content = [System.IO.File]::ReadAllText($scriptPath).Replace("`r`n", "`n")
        [System.IO.File]::WriteAllText($scriptPath, $content, (New-Object System.Text.UTF8Encoding($false)))
    }
    # Create the tar.gz archive from the staging directory
    if (Test-Path $archivePath) {
        Remove-Item -Force $archivePath
    }
    tar -czf $archivePath -C $stagingRoot $packageName
    if ($LASTEXITCODE -ne 0) {
        throw "tar failed with exit code $LASTEXITCODE."
    }

} finally {
    Remove-Item -Recurse -Force $stagingDirectory -ErrorAction SilentlyContinue
}
Write-Host "Created $archivePath"
