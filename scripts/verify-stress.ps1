param(
    [string]$Backend = "default",
    [string]$Addr = "127.0.0.1:0",
    [int]$Boss = 1,
    [int]$Workers = 2,
    [int]$Connections = 32,
    [int]$Messages = 32,
    [int]$PayloadSize = 64,
    [string]$Scenario = "mixed",
    [string]$Protocol = "both",
    [switch]$ReusePort,
    [switch]$Mmap,
    [int]$MmapBlockSize = 4096,
    [int]$MmapBlocks = 4096,
    [switch]$IOUringSQPoll,
    [switch]$IOUringMultishotAccept,
    [switch]$IOUringFixedBuffers,
    [int]$IOUringEntries = 0
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("gnalloy-stress-" + [System.Guid]::NewGuid().ToString("N"))

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

Push-Location $repo
try {
    $env:GOWORK = "off"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    $stressCheckExe = Join-Path $tempDir "stress-check.exe"
    go build -o $stressCheckExe ./examples/stress-check

    $args = @(
        "-addr", $Addr,
        "-backend", $Backend,
        "-boss", "$Boss",
        "-workers", "$Workers",
        "-protocol", $Protocol,
        "-scenario", $Scenario,
        "-connections", "$Connections",
        "-messages", "$Messages",
        "-payload-size", "$PayloadSize"
    )
    if ($ReusePort) {
        $args += "-reuseport"
    }
    if ($Mmap) {
        $args += @("-mmap", "-mmap-block-size", "$MmapBlockSize", "-mmap-blocks", "$MmapBlocks")
    }
    if ($IOUringSQPoll) {
        $args += "-iouring-sqpoll"
    }
    if ($IOUringMultishotAccept) {
        $args += "-iouring-multishot-accept"
    }
    if ($IOUringFixedBuffers) {
        $args += "-iouring-fixed-buffers"
    }
    if ($IOUringEntries -gt 0) {
        $args += @("-iouring-entries", "$IOUringEntries")
    }

    & $stressCheckExe @args
    if ($LASTEXITCODE -ne 0) {
        throw "stress check failed with exit code $LASTEXITCODE"
    }
    Write-Host "stress ok backend=$Backend workers=$Workers connections=$Connections messages=$Messages scenario=$Scenario leaks=0"
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    Pop-Location
    Remove-TempDir $tempDir
}
