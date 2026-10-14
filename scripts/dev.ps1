param([int]$WebPort = 3000, [int]$EnginePort = 8080)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$engine = Join-Path $root 'apps/engine'
$binaryDirectory = Join-Path $root '.local/bin'
$binary = Join-Path $binaryDirectory 'engine.exe'
$process = $null
$previous = @{}
$keys = @('TERMINAL_PASSWORD', 'SESSION_SECRET', 'ENGINE_API_TOKEN', 'ENGINE_ADDRESS', 'ENGINE_URL', 'APP_ORIGIN')
foreach ($key in $keys) { $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process') }

function New-RandomSecret {
    $bytes = [byte[]]::new(48)
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return [Convert]::ToBase64String($bytes)
}

try {
    if ($WebPort -lt 1024 -or $WebPort -gt 65535 -or $EnginePort -lt 1024 -or $EnginePort -gt 65535 -or $WebPort -eq $EnginePort) { throw 'Puertos no válidos.' }
    $securePassword = Read-Host 'Contraseña temporal de tu terminal (mínimo 16 caracteres)' -AsSecureString
    $plainPassword = [System.Net.NetworkCredential]::new('', $securePassword).Password
    if ($plainPassword.Length -lt 16 -or $plainPassword.StartsWith('replace-with-')) { throw 'Elige una contraseña única de al menos 16 caracteres.' }
    $env:TERMINAL_PASSWORD = $plainPassword
    $env:SESSION_SECRET = New-RandomSecret
    $env:ENGINE_API_TOKEN = New-RandomSecret
    $env:ENGINE_ADDRESS = "127.0.0.1:$EnginePort"
    $env:ENGINE_URL = "http://127.0.0.1:$EnginePort"
    $env:APP_ORIGIN = "http://localhost:$WebPort"

    New-Item -ItemType Directory -Path $binaryDirectory -Force | Out-Null
    Push-Location $engine
    try {
        & go build -o $binary ./cmd/engine
        if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar el motor.' }
    } finally { Pop-Location }
    $process = Start-Process -FilePath $binary -WorkingDirectory $engine -NoNewWindow -PassThru
    Write-Host "Abre $env:APP_ORIGIN. Solo simulación; los secretos no se guardan a disco."
    Push-Location $root
    try { & npm.cmd run dev --workspace '@regrow/web' -- --hostname localhost --port $WebPort }
    finally { Pop-Location }
} finally {
    if ($process -and -not $process.HasExited) { Stop-Process -Id $process.Id }
    foreach ($key in $keys) { [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process') }
    $plainPassword = $null
}
