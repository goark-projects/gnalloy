param(
    [switch]$SkipCrossCompile,
    [switch]$SkipBench
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$oldGoWork = $env:GOWORK
$oldGoOS = $env:GOOS
$oldGoArch = $env:GOARCH
Push-Location $repo
try {
    $env:GOWORK = "off"

    Write-Host "== windows/native: go test ./..."
    go test ./...

    if (-not $SkipCrossCompile) {
        $targets = @(
            @{ GOOS = "linux"; GOARCH = "amd64" },
            @{ GOOS = "linux"; GOARCH = "arm64" },
            @{ GOOS = "darwin"; GOARCH = "arm64" },
            @{ GOOS = "windows"; GOARCH = "amd64" }
        )

        foreach ($target in $targets) {
            Write-Host "== cross-compile: $($target.GOOS)/$($target.GOARCH)"
            $env:GOOS = $target.GOOS
            $env:GOARCH = $target.GOARCH
            $packages = go list ./...
            foreach ($package in $packages) {
                go test -c -o NUL $package
            }
        }
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    }

    if (-not $SkipBench) {
        Write-Host "== benchmarks: buffer/channel/codec/queue/timer/transport/tcp"
        foreach ($package in @("./buffer", "./channel", "./codec", "./queue", "./timer", "./transport/tcp")) {
            & go @("test", "-run", "^$", "-bench", ".", $package)
        }
    }

    Write-Host "== source scan: no direct stdlib syscall imports"
    if (rg -n '^\s*"syscall"\s*$|\bsyscall\.' .) {
        throw "direct syscall usage found; use golang.org/x/sys instead"
    }
} finally {
    if ($null -eq $oldGoWork) {
        Remove-Item Env:\GOWORK -ErrorAction SilentlyContinue
    } else {
        $env:GOWORK = $oldGoWork
    }
    if ($null -eq $oldGoOS) {
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    } else {
        $env:GOOS = $oldGoOS
    }
    if ($null -eq $oldGoArch) {
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $oldGoArch
    }
    Pop-Location
}
