param(
    [string[]]$Backends = @("default"),
    [int]$Workers = 0,
    [switch]$Mmap,
    [int]$MmapBlockSize = 4096,
    [int]$MmapBlocks = 4096,
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

Push-Location $repo
try {
    $env:GOWORK = "off"

    Write-Host "== benchmarks: core packages"
    go test -run "^$" -bench . -benchmem ./buffer ./channel ./codec ./queue ./timer

    foreach ($backend in $Backends) {
        if ([string]::IsNullOrWhiteSpace($backend)) {
            continue
        }
        Write-Host "== benchmarks: transport/tcp backend=$backend"
        Set-EnvValue "GNALLOY_BENCH_BACKEND" $backend.Trim()
        Set-EnvValue "GNALLOY_BENCH_WORKERS" "$Workers"
        Set-EnvValue "GNALLOY_BENCH_MMAP" ($(if ($Mmap) { "1" } else { "0" }))
        Set-EnvValue "GNALLOY_BENCH_MMAP_BLOCK_SIZE" "$MmapBlockSize"
        Set-EnvValue "GNALLOY_BENCH_MMAP_BLOCKS" "$MmapBlocks"
        Set-EnvValue "GNALLOY_BENCH_IOURING_ENTRIES" "$IOUringEntries"
        Set-EnvValue "GNALLOY_BENCH_IOURING_SQPOLL" ($(if ($IOUringSQPoll) { "1" } else { "0" }))
        Set-EnvValue "GNALLOY_BENCH_IOURING_MULTISHOT_ACCEPT" ($(if ($IOUringMultishotAccept) { "1" } else { "0" }))
        Set-EnvValue "GNALLOY_BENCH_IOURING_FIXED_BUFFERS" ($(if ($IOUringFixedBuffers) { "1" } else { "0" }))
        go test -run "^$" -bench . -benchmem ./transport/tcp
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
    Pop-Location
}
