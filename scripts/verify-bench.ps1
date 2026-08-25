param(
    [string[]]$Backends = @("default"),
    [string[]]$Groups = @("all"),
    [int]$Workers = 0,
    [int]$Count = 1,
    [string]$Benchtime = "",
    [switch]$Mmap,
    [int]$MmapBlockSize = 4096,
    [int]$MmapBlocks = 4096,
    [int]$WriteHighWatermark = 0,
    [int]$WriteLowWatermark = 0,
    [int]$IOUringEntries = 0,
    [switch]$IOUringSQPoll,
    [switch]$IOUringMultishotAccept,
    [switch]$IOUringFixedBuffers
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$oldBenchBackend = $env:GNALLOY_BENCH_BACKEND
$oldBenchWorkers = $env:GNALLOY_BENCH_WORKERS
$oldBenchMmap = $env:GNALLOY_BENCH_MMAP
$oldBenchMmapBlockSize = $env:GNALLOY_BENCH_MMAP_BLOCK_SIZE
$oldBenchMmapBlocks = $env:GNALLOY_BENCH_MMAP_BLOCKS
$oldBenchIOUringEntries = $env:GNALLOY_BENCH_IOURING_ENTRIES
$oldBenchIOUringSQPoll = $env:GNALLOY_BENCH_IOURING_SQPOLL
$oldBenchIOUringMultishotAccept = $env:GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT
$oldBenchIOUringFixedBuffers = $env:GNALLOY_BENCH_IOURING_FIXED_BUFFERS
$oldBenchWriteHighWatermark = $env:GNALLOY_BENCH_WRITE_HIGH_WATERMARK
$oldBenchWriteLowWatermark = $env:GNALLOY_BENCH_WRITE_LOW_WATERMARK

function Set-EnvValue($name, $value) {
    [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
}

function Restore-EnvValue($name, $value) {
    if ($null -eq $value) {
        Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
    } else {
        [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
}

function Test-BenchGroup($name) {
    foreach ($group in $Groups) {
        $normalized = $group.Trim().ToLowerInvariant()
        if ($normalized -eq "all" -or $normalized -eq $name) {
            return $true
        }
    }
    return $false
}

function Invoke-GoBench($name, $bench, [string[]]$packages) {
    if (-not (Test-BenchGroup $name)) {
        return
    }
    Write-Host "== benchmarks: $name"
    $args = @("test", "-run", "^$", "-bench", $bench, "-benchmem", "-count", "$Count")
    if (-not [string]::IsNullOrWhiteSpace($Benchtime)) {
        $args += @("-benchtime", $Benchtime)
    }
    $args += $packages
    & go @args
}

Push-Location $repo
try {
    $env:GOWORK = "off"

    Invoke-GoBench "buffer" "." @("./buffer")
    Invoke-GoBench "pipeline" "." @("./channel")
    Invoke-GoBench "codec" "Benchmark(LengthFieldDecoder|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder)$" @("./codec")
    Invoke-GoBench "codec-protocol" "." @("./codec/icmp", "./codec/ip")
    Invoke-GoBench "queue" "." @("./queue")
    Invoke-GoBench "timer" "." @("./timer")
    Invoke-GoBench "quic" "." @("./transport/quic")
    Invoke-GoBench "udp" "." @("./transport/udp")
    Invoke-GoBench "raw" "." @("./transport/raw")

    if ((Test-BenchGroup "tcp") -or (Test-BenchGroup "tcp-echo") -or (Test-BenchGroup "length-field")) {
        foreach ($backend in $Backends) {
            if ([string]::IsNullOrWhiteSpace($backend)) {
                continue
            }
            Set-EnvValue "GNALLOY_BENCH_BACKEND" $backend.Trim()
            Set-EnvValue "GNALLOY_BENCH_WORKERS" "$Workers"
            Set-EnvValue "GNALLOY_BENCH_MMAP" ($(if ($Mmap) { "1" } else { "0" }))
            Set-EnvValue "GNALLOY_BENCH_MMAP_BLOCK_SIZE" "$MmapBlockSize"
            Set-EnvValue "GNALLOY_BENCH_MMAP_BLOCKS" "$MmapBlocks"
            Set-EnvValue "GNALLOY_BENCH_WRITE_HIGH_WATERMARK" "$WriteHighWatermark"
            Set-EnvValue "GNALLOY_BENCH_WRITE_LOW_WATERMARK" "$WriteLowWatermark"
            Set-EnvValue "GNALLOY_BENCH_IOURING_ENTRIES" "$IOUringEntries"
            Set-EnvValue "GNALLOY_BENCH_IOURING_SQPOLL" ($(if ($IOUringSQPoll) { "1" } else { "0" }))
            Set-EnvValue "GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT" ($(if ($IOUringMultishotAccept) { "1" } else { "0" }))
            Set-EnvValue "GNALLOY_BENCH_IOURING_FIXED_BUFFERS" ($(if ($IOUringFixedBuffers) { "1" } else { "0" }))
            if (Test-BenchGroup "tcp") {
                Invoke-GoBench "tcp" "." @("./transport/tcp")
            } else {
                Invoke-GoBench "tcp-echo" "BenchmarkNativeTCPEchoRoundTrip$" @("./transport/tcp")
                Invoke-GoBench "length-field" "BenchmarkLengthFieldTCPRoundTrip$" @("./transport/tcp")
            }
        }
    }
} finally {
    Restore-EnvValue "GOWORK" $oldGoWork
    Restore-EnvValue "GNALLOY_BENCH_BACKEND" $oldBenchBackend
    Restore-EnvValue "GNALLOY_BENCH_WORKERS" $oldBenchWorkers
    Restore-EnvValue "GNALLOY_BENCH_MMAP" $oldBenchMmap
    Restore-EnvValue "GNALLOY_BENCH_MMAP_BLOCK_SIZE" $oldBenchMmapBlockSize
    Restore-EnvValue "GNALLOY_BENCH_MMAP_BLOCKS" $oldBenchMmapBlocks
    Restore-EnvValue "GNALLOY_BENCH_IOURING_ENTRIES" $oldBenchIOUringEntries
    Restore-EnvValue "GNALLOY_BENCH_IOURING_SQPOLL" $oldBenchIOUringSQPoll
    Restore-EnvValue "GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT" $oldBenchIOUringMultishotAccept
    Restore-EnvValue "GNALLOY_BENCH_IOURING_FIXED_BUFFERS" $oldBenchIOUringFixedBuffers
    Restore-EnvValue "GNALLOY_BENCH_WRITE_HIGH_WATERMARK" $oldBenchWriteHighWatermark
    Restore-EnvValue "GNALLOY_BENCH_WRITE_LOW_WATERMARK" $oldBenchWriteLowWatermark
    Pop-Location
}
