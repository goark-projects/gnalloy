param(
    [switch]$AllowSkip,
    [switch]$RunIOUring,
    [string]$RawBind,
    [string]$RawProtocol,
    [string]$L2Interface,
    [string]$L2EtherType,
    [string]$BPFInterface,
    [string]$BPFEtherType,
    [string]$NpcapInterface,
    [string]$NpcapEtherType,
    [string]$DoQServer,
    [string]$QuicInteropAddr,
    [string]$ReportPath
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$oldRawPrivileged = $env:GNALLOY_RAW_PRIVILEGED
$oldRawBind = $env:GNALLOY_RAW_BIND
$oldRawProtocol = $env:GNALLOY_RAW_PROTOCOL
$oldL2Interface = $env:GNALLOY_L2_INTERFACE
$oldL2EtherType = $env:GNALLOY_L2_ETHERTYPE
$oldBPFInterface = $env:GNALLOY_BPF_INTERFACE
$oldBPFEtherType = $env:GNALLOY_BPF_ETHERTYPE
$oldNpcapInterface = $env:GNALLOY_NPCAP_INTERFACE
$oldNpcapEtherType = $env:GNALLOY_NPCAP_ETHERTYPE
$oldDoQ = $env:GNALLOY_DOQ_ADDR
$oldQuicInterop = $env:GNALLOY_QUIC_INTEROP_ADDR
$results = New-Object System.Collections.Generic.List[object]
$failed = $false

function Resolve-GoCommand {
    if (-not [string]::IsNullOrWhiteSpace($env:GO)) {
        return $env:GO
    }
    return "go"
}

function Set-EnvValue($name, $value) {
    if ([string]::IsNullOrWhiteSpace($value)) {
        Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
        return
    }
    [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
}

function Restore-EnvValue($name, $value) {
    if ($null -eq $value) {
        Remove-Item "Env:\$name" -ErrorAction SilentlyContinue
    } else {
        [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
}

function Add-Result {
    param([string]$Name, [string]$Status, [string]$Reason, [double]$DurationMs)
    $results.Add([pscustomobject]@{
        name = $Name
        status = $Status
        reason = $Reason
        durationMs = [math]::Round($DurationMs, 2)
    }) | Out-Null
}

function Invoke-Gate {
    param([string]$Name, [scriptblock]$Action)
    $started = Get-Date
    Write-Host "== $Name"
    try {
        & $Action
        Add-Result -Name $Name -Status "passed" -Reason "" -DurationMs (((Get-Date) - $started).TotalMilliseconds)
    } catch {
        $script:failed = $true
        Add-Result -Name $Name -Status "failed" -Reason $_.Exception.Message -DurationMs (((Get-Date) - $started).TotalMilliseconds)
        Write-Host "FAILED ${Name}: $($_.Exception.Message)"
    }
}

function Add-SkippedGate {
    param([string]$Name, [string]$Reason)
    Write-Host "== $Name skipped: $Reason"
    Add-Result -Name $Name -Status "skipped" -Reason $Reason -DurationMs 0
}

function Invoke-Checked {
    param([string]$FilePath, [string[]]$Arguments)
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath exited with code ${LASTEXITCODE}: $($Arguments -join ' ')"
    }
}

Push-Location $repo
try {
    $go = Resolve-GoCommand
    $env:GOWORK = "off"
    $nativeGOOS = (& $go env GOHOSTOS).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "go env GOHOSTOS failed"
    }
    if ($nativeGOOS -eq "linux") {
        if ([string]::IsNullOrWhiteSpace($RawBind)) {
            $RawBind = "0.0.0.0"
        }
        if ([string]::IsNullOrWhiteSpace($RawProtocol)) {
            $RawProtocol = "1"
        }
        if ([string]::IsNullOrWhiteSpace($L2Interface)) {
            $L2Interface = $env:GNALLOY_L2_INTERFACE
        }
        if ([string]::IsNullOrWhiteSpace($L2EtherType)) {
            $L2EtherType = "0"
        }
        Set-EnvValue "GNALLOY_RAW_PRIVILEGED" "1"
        Set-EnvValue "GNALLOY_RAW_BIND" $RawBind
        Set-EnvValue "GNALLOY_RAW_PROTOCOL" $RawProtocol
        Invoke-Gate -Name "raw-socket-open" -Action {
            Invoke-Checked -FilePath $go -Arguments @("test", "-count=1", "./transport/raw", "-run", "TestPrivilegedRawSocketOpen")
        }
        if ([string]::IsNullOrWhiteSpace($L2Interface)) {
            if ($AllowSkip) {
                Add-SkippedGate -Name "af-packet-open" -Reason "set GNALLOY_L2_INTERFACE"
            } else {
                throw "set GNALLOY_L2_INTERFACE"
            }
        } else {
            Set-EnvValue "GNALLOY_L2_INTERFACE" $L2Interface
            Set-EnvValue "GNALLOY_L2_ETHERTYPE" $L2EtherType
            Invoke-Gate -Name "af-packet-open" -Action {
                Invoke-Checked -FilePath $go -Arguments @("test", "-count=1", "./transport/l2", "-run", "TestPrivilegedAFPacketOpen")
            }
        }
        if ($RunIOUring) {
            Invoke-Gate -Name "iouring-sqpoll" -Action { Invoke-Checked -FilePath "sh" -Arguments @("./scripts/verify-iouring-sqpoll.sh") }
            Invoke-Gate -Name "iouring-fixed" -Action { Invoke-Checked -FilePath "sh" -Arguments @("./scripts/verify-iouring-fixed.sh") }
        } else {
            Add-SkippedGate -Name "iouring-runtime" -Reason "set -RunIOUring"
        }
    } else {
        Add-SkippedGate -Name "linux-raw-afpacket" -Reason "requires Linux"
    }
    if (@("darwin", "freebsd", "netbsd", "openbsd", "dragonfly") -contains $nativeGOOS) {
        if ([string]::IsNullOrWhiteSpace($BPFInterface)) {
            $BPFInterface = $env:GNALLOY_BPF_INTERFACE
        }
        if ([string]::IsNullOrWhiteSpace($BPFEtherType)) {
            $BPFEtherType = "0"
        }
        if ([string]::IsNullOrWhiteSpace($BPFInterface)) {
            if ($AllowSkip) {
                Add-SkippedGate -Name "bpf-open" -Reason "set GNALLOY_BPF_INTERFACE"
            } else {
                throw "set GNALLOY_BPF_INTERFACE"
            }
        } else {
            Set-EnvValue "GNALLOY_BPF_INTERFACE" $BPFInterface
            Set-EnvValue "GNALLOY_BPF_ETHERTYPE" $BPFEtherType
            Invoke-Gate -Name "bpf-open" -Action {
                Invoke-Checked -FilePath $go -Arguments @("test", "-count=1", "./transport/l2/bpf", "-run", "TestPrivilegedBPFOpen")
            }
        }
    } else {
        Add-SkippedGate -Name "bpf-open" -Reason "requires macOS/BSD"
    }
    if ($nativeGOOS -eq "windows") {
        if ([string]::IsNullOrWhiteSpace($NpcapInterface)) {
            $NpcapInterface = $env:GNALLOY_NPCAP_INTERFACE
        }
        if ([string]::IsNullOrWhiteSpace($NpcapEtherType)) {
            $NpcapEtherType = "0"
        }
        if ([string]::IsNullOrWhiteSpace($NpcapInterface)) {
            if ($AllowSkip) {
                Add-SkippedGate -Name "npcap-open" -Reason "set GNALLOY_NPCAP_INTERFACE"
            } else {
                throw "set GNALLOY_NPCAP_INTERFACE"
            }
        } else {
            Set-EnvValue "GNALLOY_NPCAP_INTERFACE" $NpcapInterface
            Set-EnvValue "GNALLOY_NPCAP_ETHERTYPE" $NpcapEtherType
            Invoke-Gate -Name "npcap-open" -Action {
                Invoke-Checked -FilePath $go -Arguments @("test", "-count=1", "./transport/l2/npcap", "-run", "TestPrivilegedNpcapOpen")
            }
        }
    } else {
        Add-SkippedGate -Name "npcap-open" -Reason "requires Windows"
    }
    if (-not [string]::IsNullOrWhiteSpace($DoQServer)) {
        Set-EnvValue "GNALLOY_DOQ_ADDR" $DoQServer
    }
    if (-not [string]::IsNullOrWhiteSpace($env:GNALLOY_DOQ_ADDR)) {
        Invoke-Gate -Name "external-doq" -Action { Invoke-Checked -FilePath $go -Arguments @("run", "./examples/doq-query", "-server", $env:GNALLOY_DOQ_ADDR) }
    } else {
        Add-SkippedGate -Name "external-doq" -Reason "set GNALLOY_DOQ_ADDR"
    }
    if (-not [string]::IsNullOrWhiteSpace($QuicInteropAddr)) {
        Set-EnvValue "GNALLOY_QUIC_INTEROP_ADDR" $QuicInteropAddr
    }
    if (-not [string]::IsNullOrWhiteSpace($env:GNALLOY_QUIC_INTEROP_ADDR)) {
        Invoke-Gate -Name "external-quic-interop" -Action { Invoke-Checked -FilePath $go -Arguments @("test", "-count=1", "./transport/quic/rfc9000", "-run", "TestExternalInteropHandshake") }
    } else {
        Add-SkippedGate -Name "external-quic-interop" -Reason "set GNALLOY_QUIC_INTEROP_ADDR"
    }
} finally {
    Restore-EnvValue "GOWORK" $oldGoWork
    Restore-EnvValue "GNALLOY_RAW_PRIVILEGED" $oldRawPrivileged
    Restore-EnvValue "GNALLOY_RAW_BIND" $oldRawBind
    Restore-EnvValue "GNALLOY_RAW_PROTOCOL" $oldRawProtocol
    Restore-EnvValue "GNALLOY_L2_INTERFACE" $oldL2Interface
    Restore-EnvValue "GNALLOY_L2_ETHERTYPE" $oldL2EtherType
    Restore-EnvValue "GNALLOY_BPF_INTERFACE" $oldBPFInterface
    Restore-EnvValue "GNALLOY_BPF_ETHERTYPE" $oldBPFEtherType
    Restore-EnvValue "GNALLOY_NPCAP_INTERFACE" $oldNpcapInterface
    Restore-EnvValue "GNALLOY_NPCAP_ETHERTYPE" $oldNpcapEtherType
    Restore-EnvValue "GNALLOY_DOQ_ADDR" $oldDoQ
    Restore-EnvValue "GNALLOY_QUIC_INTEROP_ADDR" $oldQuicInterop
    if (-not [string]::IsNullOrWhiteSpace($ReportPath)) {
        [pscustomobject]@{
            generatedAt = (Get-Date).ToString("o")
            results = $results
        } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $ReportPath -Encoding utf8
    }
    Pop-Location
}

if ($failed) {
    throw "privileged verification failed"
}
