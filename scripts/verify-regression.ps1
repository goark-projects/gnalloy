param(
    [string]$Benchtime = "100ms",
    [int]$Count = 1
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

Push-Location $repo
try {
    $go = Resolve-GoCommand
    $env:GOWORK = "off"

    Write-Host "== tests: full suite"
    Invoke-CheckedGo @("test", "./...", "-count=$Count")

    Write-Host "== benchmarks: hot path allocation guards"
    Invoke-CheckedGo @(
        "test",
        "./buffer",
        "./codec",
        "./queue",
        "./timer",
        "-run", "^$",
        "-bench", "Benchmark(HeapAllocatorAcquireRelease|MmapAllocatorAcquireRelease|FixedLengthFrameDecoder|LineBasedFrameDecoder|DelimiterBasedFrameDecoder|ByteToMessageListDecoder|MPSCOfferPoll|WheelScheduleAdvance)$",
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
