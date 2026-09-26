param(
    [Parameter(Mandatory=$true)]
    [string]$Profile,
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

$ServiceName = "ChameleonClient"
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
}

$BinaryPath = ('"{0}" connect --config "{1}"' -f $Exe, $Config)
New-Service -Name $ServiceName -BinaryPathName $BinaryPath -DisplayName "Chameleon Adaptive Client" -Description "Chameleon Protocol local SOCKS5 adaptive client" -StartupType Automatic | Out-Null

Start-Service -Name $ServiceName
Write-Host "Chameleon client installed."
Write-Host "SOCKS5: 127.0.0.1:1080"
Write-Host "Status: Get-Service ChameleonClient"
Write-Host "Doctor: & '$Exe' doctor --config '$Config'"
