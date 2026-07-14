param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ServerArguments
)

$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$temporaryDirectory = Join-Path $projectRoot ".gotmp"
$cacheDirectory = Join-Path $projectRoot ".gocache"
$binaryDirectory = Join-Path $projectRoot "bin"
$binaryPath = Join-Path $binaryDirectory "samrai-dev.exe"

function Import-DotEnv([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    foreach ($rawLine in Get-Content -LiteralPath $Path) {
        $line = $rawLine.Trim()
        if ($line.Length -eq 0 -or $line.StartsWith("#")) {
            continue
        }
        $separator = $line.IndexOf("=")
        if ($separator -le 0) {
            continue
        }
        $name = $line.Substring(0, $separator).Trim()
        $value = $line.Substring($separator + 1).Trim()
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        if ($name -match '^[A-Za-z_][A-Za-z0-9_]*$' -and -not (Test-Path "Env:$name")) {
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

Import-DotEnv (Join-Path $projectRoot ".env")

New-Item -ItemType Directory -Force -Path $temporaryDirectory | Out-Null
New-Item -ItemType Directory -Force -Path $cacheDirectory | Out-Null
New-Item -ItemType Directory -Force -Path $binaryDirectory | Out-Null

$env:GOTMPDIR = $temporaryDirectory
$env:GOCACHE = $cacheDirectory

Push-Location $projectRoot
try {
    Write-Host "Compiling samrai..." -ForegroundColor Cyan
    & go build -o $binaryPath ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        throw "The build failed with exit code $LASTEXITCODE."
    }

    Write-Host "Starting $binaryPath" -ForegroundColor Green
    & $binaryPath @ServerArguments
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
