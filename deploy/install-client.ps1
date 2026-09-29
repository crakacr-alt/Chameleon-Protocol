param(
    [Parameter(Mandatory=$true)]
    [string]$Profile,
    [ValidateSet("smart", "proxy")]
    [string]$Mode = "smart"
)

$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run PowerShell as Administrator."
}

$Root = Join-Path $env:ProgramFiles "Chameleon"
$Data = Join-Path $env:ProgramData "Chameleon"
$Exe = Join-Path $Root "chameleon.exe"
$Config = Join-Path $Data "config.json"
$TaskName = "ChameleonClient"

New-Item -ItemType Directory -Force -Path $Root, $Data | Out-Null

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go 1.27+ is required for source installation on Windows."
}

Write-Host "[chameleon] testing and building client"
go test ./...
if ($LASTEXITCODE -ne 0) { throw "go test failed" }
go build -trimpath -o $Exe ./cmd/chameleon
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

& $Exe import $Profile --config $Config --state-dir $Data --mode $Mode
if ($LASTEXITCODE -ne 0) { throw "profile import failed" }

$legacy = Get-Service -Name $TaskName -ErrorAction SilentlyContinue
if ($null -ne $legacy) {
    Stop-Service -Name $TaskName -Force -ErrorAction SilentlyContinue
    sc.exe delete $TaskName | Out-Null
}

Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
$action = New-ScheduledTaskAction -Execute $Exe -Argument ('connect --config "{0}"' -f $Config)
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 20 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest | Out-Null

Start-ScheduledTask -TaskName $TaskName
Start-Sleep -Seconds 1

Write-Host "Chameleon client installed."
Write-Host "SOCKS5: 127.0.0.1:1080"
Write-Host "Task: Get-ScheduledTask $TaskName"
Write-Host "Doctor: & '$Exe' doctor --config '$Config'"
