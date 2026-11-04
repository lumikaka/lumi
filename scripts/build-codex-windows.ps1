#requires -Version 5.1
[CmdletBinding()]
param([string]$Output = 'build/codex-windows')
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$timings = [ordered]@{}
Push-Location $repo
try {
    [void][IO.Directory]::CreateDirectory($Output)
    $timer = [Diagnostics.Stopwatch]::StartNew()
    pnpm --dir web install --frozen-lockfile
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    $timings.dependencies_seconds = $timer.Elapsed.TotalSeconds
    $timer.Restart()
    pnpm --dir web run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
    $timings.frontend_seconds = $timer.Elapsed.TotalSeconds
    $timer.Restart()
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $binary = Join-Path $Output 'lumi-windows-amd64.exe'
    go build -trimpath -tags embed_frontend -ldflags='-s -w' -o $binary ./cmd/lumi
    if ($LASTEXITCODE -ne 0) { throw 'Windows CLI build failed.' }
    $timings.go_build_seconds = $timer.Elapsed.TotalSeconds
    & ([IO.Path]::GetFullPath($binary)) --help
    if ($LASTEXITCODE -ne 0) { throw 'Native CLI startup failed.' }
    $hash = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText("$binary.sha256", "$hash  lumi-windows-amd64.exe`n", (New-Object Text.UTF8Encoding($false)))
} finally {
    $json = $timings | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $Output 'timings.json'), $json, (New-Object Text.UTF8Encoding($false)))
    if ($env:GITHUB_STEP_SUMMARY) { $json | Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY }
    Pop-Location
}
