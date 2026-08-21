param(
    [int]$Workers = 2,
    [int]$SmokeCount = 3,
    [int]$StressConnections = 16,
    [int]$StressMessages = 16,
    [int]$BenchConnections = 16,
    [int]$BenchMessages = 64,
    [int]$PayloadSize = 64,
    [int]$LargePayloadSize = 262144
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("gnalloy-iocp-" + [System.Guid]::NewGuid().ToString("N"))

function Stop-Server($process) {
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }
}

function Remove-TempDir($path) {
    if (-not (Test-Path $path)) {
        return
    }
    for ($i = 0; $i -lt 10; $i++) {
        try {
            Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
            return
        } catch {
            Start-Sleep -Milliseconds 100
        }
    }
    Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
}

function Invoke-BenchServer($serverExe, [string[]]$serverArgs, [string[]]$clientArgs) {
    $stdout = Join-Path $tempDir "bench-server.out"
    $stderr = Join-Path $tempDir "bench-server.err"
    Remove-Item -LiteralPath $stdout, $stderr -ErrorAction SilentlyContinue

    $server = Start-Process -FilePath $serverExe `
        -ArgumentList $serverArgs `
        -PassThru `
        -WindowStyle Hidden `
        -RedirectStandardOutput $stdout `
        -RedirectStandardError $stderr
    try {
        Start-Sleep -Milliseconds 1000
        if ($server.HasExited) {
            $outText = if (Test-Path $stdout) { Get-Content -Raw $stdout } else { "" }
            $errText = if (Test-Path $stderr) { Get-Content -Raw $stderr } else { "" }
            throw "server exited early code=$($server.ExitCode)`nstdout=$outText`nstderr=$errText"
        }
        & (Join-Path $tempDir "bench-client.exe") @clientArgs
        if ($LASTEXITCODE -ne 0) {
            throw "bench client failed with exit code $LASTEXITCODE"
        }
    } finally {
        Stop-Server $server
    }
}

Push-Location $repo
try {
    if (-not $IsWindows -and $PSVersionTable.PSEdition -eq "Core") {
        throw "IOCP verification requires Windows"
    }
    $env:GOWORK = "off"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    Write-Host "== iocp smoke"
    .\scripts\verify-smoke.ps1 -Backend iocp -Workers $Workers -Count $SmokeCount

    Write-Host "== iocp stress"
    .\scripts\verify-stress.ps1 -Backend iocp -Workers $Workers -Connections $StressConnections -Messages $StressMessages -PayloadSize $PayloadSize -Scenario mixed

    Write-Host "== iocp bench"
    $echoExe = Join-Path $tempDir "echo.exe"
    $lengthExe = Join-Path $tempDir "length-field.exe"
    $benchExe = Join-Path $tempDir "bench-client.exe"
    go build -o $echoExe ./examples/echo
    go build -o $lengthExe ./examples/length-field
    go build -o $benchExe ./examples/bench-client

    Invoke-BenchServer $echoExe `
        @("-backend", "iocp", "-workers", "$Workers", "-addr", "127.0.0.1:19200") `
        @("-addr", "127.0.0.1:19200", "-protocol", "raw", "-connections", "$BenchConnections", "-messages", "$BenchMessages", "-payload-size", "$PayloadSize")

    Invoke-BenchServer $lengthExe `
        @("-backend", "iocp", "-workers", "$Workers", "-addr", "127.0.0.1:19201") `
        @("-addr", "127.0.0.1:19201", "-protocol", "length-field", "-connections", "$BenchConnections", "-messages", "$BenchMessages", "-payload-size", "$PayloadSize")

    Write-Host "== iocp large-payload partial-write stress"
    .\scripts\verify-stress.ps1 -Backend iocp -Workers $Workers -Connections 2 -Messages 4 -PayloadSize $LargePayloadSize -Scenario long

    Write-Host "iocp verification ok workers=$Workers"
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    Pop-Location
    Remove-TempDir $tempDir
}
