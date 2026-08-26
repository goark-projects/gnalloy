param(
    [string]$Benchtime = "100ms",
    [int]$Count = 1,
    [string]$FuzzTime = "2s"
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK

function Restore-EnvValue($name, $value) {
    if ($null -eq $value) {
        Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
    } else {
        [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
}

function Resolve-GoCommand {
    if (-not [string]::IsNullOrWhiteSpace($env:GO)) {
        return $env:GO
    }
    return "go"
}

function Invoke-CheckedGo([string[]]$GoArgs) {
    & $go @GoArgs
    if ($LASTEXITCODE -ne 0) {
        throw "go command failed: $($GoArgs -join ' ')"
    }
}

function Invoke-FuzzSmoke([string]$Name, [string]$Target, [string]$Package) {
    if ([string]::IsNullOrWhiteSpace($FuzzTime)) {
        return
    }
    Write-Host "== fuzz: $Name"
    Invoke-CheckedGo @("test", "-run", "^$", "-fuzz", $Target, "-fuzztime", $FuzzTime, $Package)
}

Push-Location $repo
try {
    $go = Resolve-GoCommand
    $env:GOWORK = "off"

    Write-Host "== tests: full suite"
    Invoke-CheckedGo @("test", "./...", "-count=$Count")

    Invoke-FuzzSmoke "codec length-field" "FuzzLengthFieldBasedFrameDecoder" "./codec"
    Invoke-FuzzSmoke "http1 request" "FuzzHTTP1RequestDecoder" "./codec/http1"
    Invoke-FuzzSmoke "websocket frame" "FuzzWebSocketFrameDecoder" "./codec/websocket"
    Invoke-FuzzSmoke "mqtt frame pipeline" "FuzzMQTTFramePipeline" "./codec/mqtt"
    Invoke-FuzzSmoke "dns message" "FuzzDNSParseMessage" "./codec/dns"
    Invoke-FuzzSmoke "redis frame pipeline" "FuzzRedisFramePipeline" "./codec/redis"
    Invoke-FuzzSmoke "http2 frame pipeline" "FuzzHTTP2FramePipeline" "./codec/http2"
    Invoke-FuzzSmoke "http3 frame pipeline" "FuzzHTTP3FramePipeline" "./codec/http3"
    Invoke-FuzzSmoke "quic frame scanner" "FuzzQUICFrameScanner" "./transport/quic"

    Write-Host "== benchmarks: hot path allocation guards"
    Invoke-CheckedGo @(
        "test",
        "./buffer",
        "./channel/pool",
        "./codec",
        "./codec/http2",
        "./handler/ipfilter",
        "./handler/pcap",
        "./queue",
        "./timer",
        "./observability",
        "./transport/quic",
        "-run", "^$",
        "-bench", "Benchmark(HeapAllocatorAcquireRelease|PooledAllocatorAcquireRelease|MmapAllocatorAcquireRelease|FixedPoolGetPut|ChannelPoolMapGet|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder|StreamMultiplexerReadData|IPFilterAllowedDatagram|PCAPCaptureByteBuf|MPSCOfferPoll|WheelScheduleAdvance|AtomicChannelRecorderRead|PrometheusExporter|QUICRuntimeApplyACK|QUICRuntimeReceiveStream)$",
        "-benchmem",
        "-benchtime", $Benchtime,
        "-count=$Count"
    )

    if ($IsLinux) {
        Write-Host "== tests: io_uring fixed buffers"
        Invoke-CheckedGo @(
            "test",
            "./transport/poller/iouring",
            "-run", "Test(RegisterBuffersStatsAndUnregister|RegisterMmapAllocatorFixedBuffers)",
            "-count=$Count"
        )
    }
} finally {
    Restore-EnvValue "GOWORK" $oldGoWork
    Pop-Location
}
