param(
    [switch]$SkipExternal,
    [switch]$SkipBench,
    [string]$ReportPath,
    [string]$DoQServer,
    [string]$DoQServerName,
    [string]$DoQName,
    [string]$DoQType,
    [string]$DoQTimeout,
    [switch]$DoQInsecure
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$results = New-Object System.Collections.Generic.List[object]
$failed = $false

function Resolve-GoCommand {
    if (-not [string]::IsNullOrWhiteSpace($env:GO)) {
        return $env:GO
    }
    return "go"
}

function Add-Result {
    param(
        [string]$Name,
        [string]$Status,
        [string]$Reason,
        [double]$DurationMs
    )
    $results.Add([pscustomobject]@{
        name = $Name
        status = $Status
        reason = $Reason
        durationMs = [math]::Round($DurationMs, 2)
    }) | Out-Null
}

function Invoke-Checked {
    param(
        [string]$FilePath,
        [string[]]$Arguments
    )
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath exited with code ${LASTEXITCODE}: $($Arguments -join ' ')"
    }
}

function Invoke-Gate {
    param(
        [string]$Name,
        [scriptblock]$Action
    )
    $started = Get-Date
    Write-Host "== $Name"
    try {
        & $Action
        $duration = ((Get-Date) - $started).TotalMilliseconds
        Add-Result -Name $Name -Status "passed" -Reason "" -DurationMs $duration
    } catch {
        $script:failed = $true
        $duration = ((Get-Date) - $started).TotalMilliseconds
        Add-Result -Name $Name -Status "failed" -Reason $_.Exception.Message -DurationMs $duration
        Write-Host "FAILED ${Name}: $($_.Exception.Message)"
    }
}

function Add-SkippedGate {
    param(
        [string]$Name,
        [string]$Reason
    )
    Write-Host "== $Name skipped: $Reason"
    Add-Result -Name $Name -Status "skipped" -Reason $Reason -DurationMs 0
}

function Test-EnvEnabled {
    param([string]$Name)
    $value = [System.Environment]::GetEnvironmentVariable($Name, "Process")
    if ([string]::IsNullOrWhiteSpace($value)) {
        return $false
    }
    $normalized = $value.Trim().ToLowerInvariant()
    return $normalized -in @("1", "true", "yes", "on")
}

Push-Location $repo
try {
    $go = Resolve-GoCommand
    $env:GOWORK = "off"

    if ([string]::IsNullOrWhiteSpace($DoQServer)) {
        $DoQServer = $env:GNALLOY_DOQ_ADDR
    }
    if ([string]::IsNullOrWhiteSpace($DoQServerName)) {
        $DoQServerName = $env:GNALLOY_DOQ_SERVER_NAME
    }
    if ([string]::IsNullOrWhiteSpace($DoQName)) {
        $DoQName = if ([string]::IsNullOrWhiteSpace($env:GNALLOY_DOQ_QUERY)) { "example.com" } else { $env:GNALLOY_DOQ_QUERY }
    }
    if ([string]::IsNullOrWhiteSpace($DoQType)) {
        $DoQType = if ([string]::IsNullOrWhiteSpace($env:GNALLOY_DOQ_TYPE)) { "A" } else { $env:GNALLOY_DOQ_TYPE }
    }
    if ([string]::IsNullOrWhiteSpace($DoQTimeout)) {
        $DoQTimeout = if ([string]::IsNullOrWhiteSpace($env:GNALLOY_DOQ_TIMEOUT)) { "5s" } else { $env:GNALLOY_DOQ_TIMEOUT }
    }
    $doqInsecureEnabled = $DoQInsecure -or (Test-EnvEnabled "GNALLOY_DOQ_INSECURE")

    Invoke-Gate -Name "protocol-tests" -Action {
        Invoke-Checked -FilePath $go -Arguments @(
            "test", "-count=1",
            "./protocol",
            "./transport/quic/application",
            "./resolver/dns/quic",
            "./transport/l2",
            "./transport/l2/bpf",
            "./transport/l2/npcap",
            "./examples/doq-query",
            "./examples/protocol-exchange"
        )
    }

    Invoke-Gate -Name "example-dry-run" -Action {
        Invoke-Checked -FilePath $go -Arguments @("run", "./examples/doq-query", "-dry-run", "-server", "dns.google:853", "-name", "example.com", "-type", "A")
        Invoke-Checked -FilePath $go -Arguments @("run", "./examples/protocol-exchange", "-dry-run", "-transport", "tcp", "-addr", "127.0.0.1:9000", "-message", "ping")
        Invoke-Checked -FilePath $go -Arguments @("run", "./examples/protocol-exchange", "-dry-run", "-transport", "udp", "-addr", "127.0.0.1:9002", "-message", "ping")
        Invoke-Checked -FilePath $go -Arguments @("run", "./examples/protocol-exchange", "-dry-run", "-transport", "raw", "-addr", "127.0.0.1", "-raw-protocol", "253", "-message", "ping")
        Invoke-Checked -FilePath $go -Arguments @("run", "./examples/protocol-exchange", "-dry-run", "-transport", "l2", "-addr", "eth0", "-payload-hex", "00112233445566778899aabb88b570696e67")
    }

    if ($SkipBench) {
        Add-SkippedGate -Name "parity-bench-dry-run" -Reason "SkipBench"
    } else {
        Invoke-Gate -Name "parity-bench-dry-run" -Action {
            Invoke-Checked -FilePath $go -Arguments @("run", "./examples/parity-bench", "-dry-run", "-config", "benchmarks/parity/baseline.json", "-format", "json")
        }
    }

    if ($SkipExternal) {
        Add-SkippedGate -Name "external-doq" -Reason "SkipExternal"
    } elseif ([string]::IsNullOrWhiteSpace($DoQServer)) {
        Add-SkippedGate -Name "external-doq" -Reason "set GNALLOY_DOQ_ADDR or -DoQServer to run an external DoQ query"
    } else {
        Invoke-Gate -Name "external-doq" -Action {
            $doqArgs = @("run", "./examples/doq-query", "-server", $DoQServer, "-name", $DoQName, "-type", $DoQType, "-timeout", $DoQTimeout)
            if (-not [string]::IsNullOrWhiteSpace($DoQServerName)) {
                $doqArgs += @("-server-name", $DoQServerName)
            }
            if ($doqInsecureEnabled) {
                $doqArgs += "-insecure"
            }
            Invoke-Checked -FilePath $go -Arguments $doqArgs
        }
    }
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    if (-not [string]::IsNullOrWhiteSpace($ReportPath)) {
        $report = [pscustomobject]@{
            generatedAt = (Get-Date).ToString("o")
            results = $results
        }
        $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $ReportPath -Encoding utf8
    }
    Pop-Location
}

if ($failed) {
    throw "protocol verification failed"
}
