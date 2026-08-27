param(
    [string]$Backend = "default",
    [string]$Addr = "127.0.0.1:0",
    [int]$Boss = 1,
    [int]$Workers = 2,
    [string]$Protocol = "both",
    [string]$Scenario = "mixed",
    [int]$Connections = 16,
    [int]$Messages = 16,
    [int]$PayloadSize = 64,
    [int]$DurationSeconds = 0,
    [int]$MinCycles = 1,
    [string]$Timeout = "30s",
    [string]$Delay = "1ms",
    [string]$DrainTimeout = "5s",
    [string]$ReportPath,
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
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("gnalloy-soak-" + [System.Guid]::NewGuid().ToString("N"))
$cycles = New-Object System.Collections.Generic.List[object]

function Resolve-GoCommand {
    if (-not [string]::IsNullOrWhiteSpace($env:GO)) {
        return $env:GO
    }
    return "go"
}

function Resolve-IntValue {
    param(
        [int]$Value,
        [string]$EnvName
    )
    $envValue = [System.Environment]::GetEnvironmentVariable($EnvName, "Process")
    if ($Value -ne 0 -or [string]::IsNullOrWhiteSpace($envValue)) {
        return $Value
    }
    $parsed = 0
    if ([int]::TryParse($envValue, [ref]$parsed)) {
        return $parsed
    }
    return $Value
}

function Remove-TempDir {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    for ($i = 0; $i -lt 10; $i++) {
        try {
            Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop
            return
        } catch {
            Start-Sleep -Milliseconds 100
        }
    }
    Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction SilentlyContinue
}

function Invoke-Checked {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )
    $output = & $FilePath @Arguments 2>&1
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "$FilePath exited with code ${exitCode}: $($Arguments -join ' ')`n$output"
    }
    return ($output -join "`n")
}

function New-StressArgs {
    $args = @(
        "-addr", $Addr,
        "-backend", $Backend,
        "-boss", "$Boss",
        "-workers", "$Workers",
        "-protocol", $Protocol,
        "-scenario", $Scenario,
        "-connections", "$Connections",
        "-messages", "$Messages",
        "-payload-size", "$PayloadSize",
        "-timeout", $Timeout,
        "-delay", $Delay,
        "-drain-timeout", $DrainTimeout
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
    return $args
}

function Add-CycleResult {
    param(
        [int]$Index,
        [string]$Status,
        [string]$Output,
        [double]$DurationMs
    )
    $cycles.Add([pscustomobject]@{
        index = $Index
        status = $Status
        output = $Output
        durationMs = [math]::Round($DurationMs, 2)
    }) | Out-Null
}

if (-not $PSBoundParameters.ContainsKey("DurationSeconds")) {
    $DurationSeconds = Resolve-IntValue -Value $DurationSeconds -EnvName "GNALLOY_SOAK_DURATION_SECONDS"
}
if (-not $PSBoundParameters.ContainsKey("MinCycles")) {
    $minCyclesEnv = [System.Environment]::GetEnvironmentVariable("GNALLOY_SOAK_MIN_CYCLES", "Process")
    $parsedMinCycles = 0
    if (-not [string]::IsNullOrWhiteSpace($minCyclesEnv) -and [int]::TryParse($minCyclesEnv, [ref]$parsedMinCycles)) {
        $MinCycles = $parsedMinCycles
    }
}
if ($MinCycles -le 0) {
    $MinCycles = 1
}

Push-Location $repo
try {
    $go = Resolve-GoCommand
    $env:GOWORK = "off"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    $stressCheckExe = Join-Path $tempDir "stress-check.exe"
    Invoke-Checked -FilePath $go -Arguments @("build", "-o", $stressCheckExe, "./examples/stress-check") | Out-Null

    $deadline = $null
    if ($DurationSeconds -gt 0) {
        $deadline = (Get-Date).AddSeconds($DurationSeconds)
    }

    $started = Get-Date
    $cycle = 0
    do {
        $cycle++
        $cycleStarted = Get-Date
        Write-Host "== soak cycle $cycle backend=$Backend protocol=$Protocol scenario=$Scenario connections=$Connections messages=$Messages"
        $output = Invoke-Checked -FilePath $stressCheckExe -Arguments (New-StressArgs)
        $duration = ((Get-Date) - $cycleStarted).TotalMilliseconds
        Add-CycleResult -Index $cycle -Status "passed" -Output $output -DurationMs $duration
        Write-Host $output
    } while ($cycle -lt $MinCycles -or ($null -ne $deadline -and (Get-Date) -lt $deadline))

    $elapsed = ((Get-Date) - $started).TotalMilliseconds
    Write-Host "soak ok cycles=$cycle elapsedMs=$([math]::Round($elapsed, 2)) backend=$Backend protocol=$Protocol scenario=$Scenario"
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    if (-not [string]::IsNullOrWhiteSpace($ReportPath)) {
        $report = [pscustomobject]@{
            generatedAt = (Get-Date).ToString("o")
            backend = $Backend
            protocol = $Protocol
            scenario = $Scenario
            connections = $Connections
            messages = $Messages
            payloadSize = $PayloadSize
            durationSeconds = $DurationSeconds
            minCycles = $MinCycles
            cycles = $cycles
        }
        $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $ReportPath -Encoding utf8
    }
    Pop-Location
    Remove-TempDir -Path $tempDir
}
