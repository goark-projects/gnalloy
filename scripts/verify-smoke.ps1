param(
    [string]$Backend = "default",
    [string]$EchoAddr = "127.0.0.1:19000",
    [string]$LengthFieldAddr = "127.0.0.1:19001",
    [string]$UDPAddr = "127.0.0.1:19002",
    [string]$LineAddr = "127.0.0.1:19003",
    [string]$FixedAddr = "127.0.0.1:19004",
    [string]$ICMPTarget = "127.0.0.1",
    [int]$Workers = 2,
    [int]$Count = 3,
    [switch]$ReusePort,
    [switch]$Mmap,
    [int]$MmapBlockSize = 4096,
    [int]$MmapBlocks = 4096,
    [int]$WriteHighWatermark = 0,
    [int]$WriteLowWatermark = 0,
    [switch]$IOUringFixedBuffers,
    [switch]$RunRaw
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("gnalloy-smoke-" + [System.Guid]::NewGuid().ToString("N"))

function Stop-Server($process) {
    if ($null -ne $process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
        $process.WaitForExit()
    }
}

function Invoke-SmokeServer($serverExe, [string[]]$serverArgs, [string[]]$clientArgs) {
    $stdout = Join-Path $tempDir "server.out"
    $stderr = Join-Path $tempDir "server.err"
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
        & (Join-Path $tempDir "smoke-client.exe") @clientArgs
        if ($LASTEXITCODE -ne 0) {
            throw "smoke client failed with exit code $LASTEXITCODE"
        }
    } finally {
        Stop-Server $server
    }
}

Push-Location $repo
try {
    $env:GOWORK = "off"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    $echoExe = Join-Path $tempDir "echo.exe"
    $lengthExe = Join-Path $tempDir "length-field.exe"
    $udpExe = Join-Path $tempDir "udp-echo.exe"
    $lineExe = Join-Path $tempDir "line-frame.exe"
    $fixedExe = Join-Path $tempDir "fixed-length.exe"
    $icmpExe = Join-Path $tempDir "icmp-ping.exe"
    $clientExe = Join-Path $tempDir "smoke-client.exe"

    go build -o $echoExe ./examples/echo
    go build -o $lengthExe ./examples/length-field
    go build -o $udpExe ./examples/udp-echo
    go build -o $lineExe ./examples/line-frame
    go build -o $fixedExe ./examples/fixed-length
    if ($RunRaw) {
        go build -o $icmpExe ./examples/icmp-ping
    }
    go build -o $clientExe ./examples/smoke-client

    $common = @("-backend", $Backend, "-workers", "$Workers")
    if ($ReusePort) {
        $common += "-reuseport"
    }
    if ($Mmap) {
        $common += @("-mmap", "-mmap-block-size", "$MmapBlockSize", "-mmap-blocks", "$MmapBlocks")
    }
    if ($WriteHighWatermark -gt 0) {
        $common += @("-write-high-watermark", "$WriteHighWatermark")
    }
    if ($WriteLowWatermark -gt 0) {
        $common += @("-write-low-watermark", "$WriteLowWatermark")
    }
    if ($IOUringFixedBuffers) {
        $common += "-iouring-fixed-buffers"
    }

    Invoke-SmokeServer $echoExe `
        ($common + @("-addr", $EchoAddr)) `
        @("-addr", $EchoAddr, "-protocol", "raw", "-message", "ping", "-count", "$Count")

    Invoke-SmokeServer $lengthExe `
        ($common + @("-addr", $LengthFieldAddr)) `
        @("-addr", $LengthFieldAddr, "-protocol", "length-field", "-message", "ping", "-count", "$Count")

    Invoke-SmokeServer $udpExe `
        ($common + @("-addr", $UDPAddr)) `
        @("-addr", $UDPAddr, "-protocol", "udp", "-message", "ping", "-count", "$Count")

    Invoke-SmokeServer $lineExe `
        ($common + @("-addr", $LineAddr)) `
        @("-addr", $LineAddr, "-protocol", "line", "-message", "ping", "-count", "$Count")

    Invoke-SmokeServer $fixedExe `
        ($common + @("-addr", $FixedAddr, "-frame-length", "4")) `
        @("-addr", $FixedAddr, "-protocol", "fixed", "-message", "ping", "-count", "$Count")

    if ($RunRaw) {
        & $icmpExe @($common + @("-target", $ICMPTarget, "-timeout", "3s"))
        if ($LASTEXITCODE -ne 0) {
            throw "icmp raw smoke failed with exit code $LASTEXITCODE"
        }
    }

    Write-Host "smoke ok backend=$Backend workers=$Workers count=$Count raw=$RunRaw"
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    Pop-Location
    if (Test-Path $tempDir) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force
    }
}
