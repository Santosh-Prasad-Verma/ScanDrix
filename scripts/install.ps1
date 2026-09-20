param(
    [string]$TeamKey
)

# Copyright (c) ScanDrix Authors. All rights reserved.
# Licensed under the Apache License, Version 2.0.

# ═══════════════════════════════════════════════════════════════
# SCANDRIX CLI INSTALLER (Windows PowerShell)
# ═══════════════════════════════════════════════════════════════

$ErrorActionPreference = "Stop"

$Repo = "scandrix/scandrix"
$InstallDir = if ($env:SCANDRIX_INSTALL_DIR) { $env:SCANDRIX_INSTALL_DIR } else { "$env:USERPROFILE\.scandrix\bin" }
$BinaryName = "scandrix.exe"

Write-Host "==> Installing ScanDrix CLI for Windows..." -ForegroundColor Cyan

# Detect Architecture
$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
        "arm64"
    } else {
        "amd64"
    }
} else {
    Write-Error "ScanDrix requires 64-bit Windows."
    exit 1
}

# Determine Version
$Version = if ($env:SCANDRIX_VERSION) { $env:SCANDRIX_VERSION } else { "latest" }
if ($Version -eq "latest") {
    try {
        $ReleaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "ScanDrix-Installer" }
        $Version = $ReleaseInfo.tag_name
    } catch {
        $Version = "v1.0.0"
    }
}

Write-Host "    Target: windows-$Arch ($Version)"

# Ensure destination directory exists
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$ZipFile = "scandrix-windows-$Arch.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$ZipFile"
$TempPath = Join-Path $env:TEMP "scandrix-install"

if (Test-Path $TempPath) {
    Remove-Item -Recurse -Force $TempPath
}
New-Item -ItemType Directory -Path $TempPath -Force | Out-Null

$ZipPath = Join-Path $TempPath $ZipFile
$Destination = Join-Path $InstallDir $BinaryName

Write-Host "==> Downloading release archive..." -ForegroundColor Blue
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing
    Expand-Archive -Path $ZipPath -DestinationPath $TempPath -Force
    Copy-Item -Path (Join-Path $TempPath $BinaryName) -Destination $Destination -Force
} catch {
    # Fallback to local build if available
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $ParentDir = Split-Path -Parent $ScriptDir
    if ((Test-Path (Join-Path $ParentDir "go.mod")) -and (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "    Compiling from local source..."
        Push-Location $ParentDir
        go build -o $Destination ./cmd/scandrix
        Pop-Location
    } else {
        Write-Error "Failed downloading or compiling ScanDrix: $_"
        exit 1
    }
} finally {
    Remove-Item -Recurse -Force $TempPath -ErrorAction SilentlyContinue
}

Write-Host "==> ScanDrix installed successfully to: $Destination" -ForegroundColor Green

# Add to User PATH if missing
$UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host "Adding $InstallDir to user PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", [EnvironmentVariableTarget]::User)
    $env:Path = "$env:Path;$InstallDir"
}

if ($TeamKey) {
    Write-Host ""
    Write-Host "==> Authenticating with team key..." -ForegroundColor Cyan
    & $Destination auth team-key --key $TeamKey | Out-Host
    Write-Host "✓ Authenticated successfully" -ForegroundColor Green
}

Write-Host ""
Write-Host "==> Installing bundled ScanDrix agent skills..." -ForegroundColor Cyan
& $Destination skills install | Out-Host
Write-Host "✓ Agent skills installed" -ForegroundColor Green

Write-Host ""
Write-Host "Get started:"
Write-Host "  scandrix --help"
Write-Host "  scandrix auth login"
Write-Host "  scandrix review"
Write-Host ""
