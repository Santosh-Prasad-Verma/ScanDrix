param(
    [string]$TeamKey
)

# ScanDrix CLI Installer for Windows (PowerShell)
# Usage: irm https://get.scandrix.dev/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "Santosh-Prasad-Verma/ScanDrix"
$BinaryName = "scandrix.exe"
$InstallDir = "$env:LOCALAPPDATA\ScanDrix\bin"

Write-Host "🚀 Installing ScanDrix CLI for Windows..." -ForegroundColor Cyan

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$DownloadUrl = "https://github.com/$Repo/releases/latest/download/scandrix-cli-windows-amd64.exe"
$TargetPath = Join-Path $InstallDir $BinaryName

Write-Host "📥 Downloading binary from $DownloadUrl..." -ForegroundColor Gray
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TargetPath -UseBasicParsing
} catch {
    Write-Warning "Could not download from release asset. Checking local dist..."
    if (Test-Path ".\dist\scandrix-cli-windows-amd64.exe") {
        Copy-Item ".\dist\scandrix-cli-windows-amd64.exe" $TargetPath
    } else {
        throw "Failed to download ScanDrix CLI binary."
    }
}

# Add to user PATH if not present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "ℹ️  Added $InstallDir to user PATH." -ForegroundColor Yellow
}

Write-Host "✨ ScanDrix CLI installed successfully to $TargetPath!" -ForegroundColor Green

if ($TeamKey -and (Test-Path $TargetPath)) {
    Write-Host "🔐 Authenticating with team key..." -ForegroundColor Cyan
    & $TargetPath auth team-key --key $TeamKey
}

# Synchronize bundled agent skills
if (Test-Path $TargetPath) {
    Write-Host "🤖 Synchronizing bundled AI agent skills..." -ForegroundColor Cyan
    & $TargetPath skills install
}

Write-Host ""
Write-Host "Run 'scandrix --help' to get started." -ForegroundColor Cyan
