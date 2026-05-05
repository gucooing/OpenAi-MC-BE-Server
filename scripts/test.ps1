param(
    [switch]$Race,
    [switch]$RunCheck
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$goFiles = Get-ChildItem -Path "cmd", "internal" -Recurse -Filter "*.go" -File
if ($goFiles.Count -gt 0) {
    gofmt -w @($goFiles.FullName)
}

go test ./...

if ($Race) {
    go test -race ./...
}

if ($RunCheck) {
    go run ./cmd/bds -data-path .runtime -check
    $runtimePath = Resolve-Path -LiteralPath ".runtime" -ErrorAction SilentlyContinue
    if ($runtimePath) {
        if ($runtimePath.Path.StartsWith($repoRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
            Remove-Item -LiteralPath $runtimePath.Path -Recurse -Force
        } else {
            throw "Refusing to remove outside repository: $($runtimePath.Path)"
        }
    }
}

