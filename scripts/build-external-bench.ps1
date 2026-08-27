[CmdletBinding()]
param(
    [string]$Go = "D:\Program Files\go\bin\go.exe",
    [string]$Maven = "mvn",
    [switch]$SkipNetty
)

$ErrorActionPreference = "Stop"

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$External = Join-Path $Root "benchmarks\external"
$Bin = Join-Path $External "bin"
$GnalloyDir = Join-Path $External "gnalloy-bench"
$GnetDir = Join-Path $External "gnet-bench"
$NetpollDir = Join-Path $External "netpoll-bench"
$GnalloyOut = Join-Path $Bin "gnalloy-bench.exe"
$GnetOut = Join-Path $Bin "gnet-bench.exe"
$NetpollOut = Join-Path $Bin "netpoll-bench.exe"
$NettyPom = Join-Path $External "netty-bench\pom.xml"
$NettyJar = Join-Path $External "netty-bench\target\netty-bench.jar"
New-Item -ItemType Directory -Force -Path $Bin | Out-Null

# 外部 harness 必须隔离 go.work，避免把对标依赖注入 gnalloy 根模块。
$env:GOWORK = "off"
$env:GOTOOLCHAIN = "local"
[Environment]::SetEnvironmentVariable("GOWORK", "off", "Process")
[Environment]::SetEnvironmentVariable("GOTOOLCHAIN", "local", "Process")
if ([string]::IsNullOrWhiteSpace($env:GOCACHE)) {
    $env:GOCACHE = Join-Path $env:TEMP "goark-gocache-codex"
}
[Environment]::SetEnvironmentVariable("GOCACHE", $env:GOCACHE, "Process")
function Assert-LastExitCode {
    param([string]$File)
    if ($LASTEXITCODE -ne 0) {
        throw "$File exited with code $LASTEXITCODE"
    }
}

Push-Location -LiteralPath $GnalloyDir
try {
    & $Go "mod" "download"
    Assert-LastExitCode $Go
    & $Go "build" "-trimpath" "-o" $GnalloyOut
    Assert-LastExitCode $Go
}
finally {
    Pop-Location
}

Push-Location -LiteralPath $GnetDir
try {
    & $Go "mod" "download"
    Assert-LastExitCode $Go
    & $Go "build" "-trimpath" "-o" $GnetOut
    Assert-LastExitCode $Go
}
finally {
    Pop-Location
}

Push-Location -LiteralPath $NetpollDir
try {
    & $Go "mod" "download"
    Assert-LastExitCode $Go
    & $Go "build" "-trimpath" "-o" $NetpollOut
    Assert-LastExitCode $Go
}
finally {
    Pop-Location
}

if (-not $SkipNetty) {
    & $Maven "-q" "-f" $NettyPom "-DskipTests" "package"
    Assert-LastExitCode $Maven
    Copy-Item -Force $NettyJar (Join-Path $Bin "netty-bench.jar")
}

Write-Host "external benchmark harnesses built under $Bin"
