$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$temporaryDirectory = Join-Path $projectRoot ".gotmp"
$cacheDirectory = Join-Path $projectRoot ".gocache"

New-Item -ItemType Directory -Force -Path $temporaryDirectory | Out-Null
New-Item -ItemType Directory -Force -Path $cacheDirectory | Out-Null

$env:GOTMPDIR = $temporaryDirectory
$env:GOCACHE = $cacheDirectory

Push-Location $projectRoot
try {
    go clean -testcache
    go test ./...
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
