param(
    [string]$Profile = "",
    [ValidateSet("smart", "proxy")]
    [string]$Mode = "smart",
    [switch]$UpdateOnly
)

$ErrorActionPreference = "Stop"

# Standalone Windows installer for official Chameleon Desktop releases.
# The binary and checksum list are fetched from the public project release.

$Repo = "crakacr-alt/Chameleon-Protocol"
$RawBase = "https://raw.githubusercontent.com/$Repo/main"
$ReleaseBase = "https://github.com/$Repo/releases/download"
$Root = Join-Path $env:ProgramFiles "Chameleon"
$Data = Join-Path $env:ProgramData "Chameleon"
$Exe = Join-Path $Root "chameleon.exe"
$Config = Join-Path $Data "config.json"
$Installer = Join-Path $Root "install-client-release.ps1"
$ClientTask = "ChameleonClient"
$UpdateTask = "ChameleonClientUpdate"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run PowerShell as Administrator."
}

New-Item -ItemType Directory -Force -Path $Root, $Data | Out-Null

function Get-LatestVersion {
    $value = (Invoke-RestMethod -Uri "$RawBase/VERSION").ToString().Trim()
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "Could not resolve latest Chameleon version."
    }
    return $value
}

function Get-AssetName {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -eq "AMD64") { return "chameleon-windows-amd64.exe" }
    if ($arch -eq "ARM64") { return "chameleon-windows-arm64.exe" }
    throw "Unsupported Windows architecture: $arch"
}

function Stop-ClientTask {
    $existing = Get-ScheduledTask -TaskName $ClientTask -ErrorAction SilentlyContinue
    if ($null -ne $existing) {
        Stop-ScheduledTask -TaskName $ClientTask -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500
    }
}

function Install-ReleaseBinary([string]$Version) {
    $asset = Get-AssetName
    $base = "$ReleaseBase/desktop-v$Version"
    $temp = Join-Path ([System.IO.Path]::GetTempPath()) ("chameleon-" + [guid]::NewGuid())

    New-Item -ItemType Directory -Force -Path $temp | Out-Null
    try {
        $download = Join-Path $temp $asset
        $sums = Join-Path $temp "SHA256SUMS"

        Write-Host "[chameleon] downloading $asset from desktop-v$Version"
        Invoke-WebRequest -Uri "$base/$asset" -OutFile $download -UseBasicParsing
        Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile $sums -UseBasicParsing

        $line = Get-Content $sums | Where-Object { $_ -match ("\s" + [regex]::Escape($asset) + "$") } | Select-Object -First 1
        if ([string]::IsNullOrWhiteSpace($line)) {
            throw "Checksum for $asset is missing."
        }

        $expected = ($line -split "\s+")[0].ToLowerInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 -Path $download).Hash.ToLowerInvariant()
        if ($expected -ne $actual) {
            throw "SHA-256 mismatch for $asset"
        }

        Stop-ClientTask
        Copy-Item -Force $download $Exe
        Write-Host "[chameleon] installed $(& $Exe version)"
    }
    finally {
        Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
    }
}

$version = Get-LatestVersion
$current = ""
if (Test-Path $Exe) {
    try { $current = (& $Exe version 2>$null).Trim() }
    catch { $current = "" }
}

if ($current -ne $version) {
    Install-ReleaseBinary $version
}
else {
    Write-Host "[chameleon] binary already current: $version"
}

if ($UpdateOnly) {
    $existing = Get-ScheduledTask -TaskName $ClientTask -ErrorAction SilentlyContinue
    if ($null -ne $existing) { Start-ScheduledTask -TaskName $ClientTask }
    exit 0
}

if ([string]::IsNullOrWhiteSpace($Profile) -and -not (Test-Path $Config)) {
    throw "First install requires -Profile C:\path\client-profile.txt"
}

if (-not [string]::IsNullOrWhiteSpace($Profile)) {
    if (-not (Test-Path $Profile)) { throw "Profile not found: $Profile" }

    & $Exe import $Profile --config $Config --state-dir $Data --mode $Mode
    if ($LASTEXITCODE -ne 0) { throw "Profile import failed." }
}

# Keep a local updater copy so the scheduled task never executes remote script text.
if ($PSCommandPath -and (Test-Path $PSCommandPath)) {
    Copy-Item -Force $PSCommandPath $Installer
}

Unregister-ScheduledTask -TaskName $ClientTask -Confirm:$false -ErrorAction SilentlyContinue
$clientAction = New-ScheduledTaskAction -Execute $Exe -Argument ('connect --config "{0}"' -f $Config)
$clientTrigger = New-ScheduledTaskTrigger -AtStartup
$clientSettings = New-ScheduledTaskSettingsSet -RestartCount 20 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName $ClientTask -Action $clientAction -Trigger $clientTrigger -Settings $clientSettings -User "SYSTEM" -RunLevel Highest | Out-Null

if (Test-Path $Installer) {
    Unregister-ScheduledTask -TaskName $UpdateTask -Confirm:$false -ErrorAction SilentlyContinue
    $updateAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument ('-NoProfile -ExecutionPolicy Bypass -File "{0}" -UpdateOnly' -f $Installer)
    $updateTrigger = New-ScheduledTaskTrigger -Daily -At 3am
    Register-ScheduledTask -TaskName $UpdateTask -Action $updateAction -Trigger $updateTrigger -User "SYSTEM" -RunLevel Highest | Out-Null
}

Start-ScheduledTask -TaskName $ClientTask
Start-Sleep -Seconds 1

Write-Host "Installed: $(& $Exe version)"
Write-Host "SOCKS5:   127.0.0.1:1080"
Write-Host "Task:     Get-ScheduledTask ChameleonClient"
Write-Host "Doctor:   & '$Exe' doctor --config '$Config'"
Write-Host "Updates:  Get-ScheduledTask ChameleonClientUpdate"
