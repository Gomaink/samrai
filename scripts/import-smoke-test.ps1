param(
    [Parameter(Mandatory = $true)]
    [string]$Path,

    [string]$BaseUrl = "http://127.0.0.1:8080",

    [string]$Username = "samuel"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "File not found: $Path"
}

$credential = Get-Credential -UserName $Username -Message "Enter the samrai administrator credentials"
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

$loginBody = @{
    username = $credential.UserName
    password = $credential.GetNetworkCredential().Password
} | ConvertTo-Json

Invoke-RestMethod `
    -Method Post `
    -Uri "$BaseUrl/api/v1/auth/login" `
    -WebSession $session `
    -ContentType "application/json" `
    -Body $loginBody | Out-Null

Write-Host "Uploading $Path ..."
$job = Invoke-RestMethod `
    -Method Post `
    -Uri "$BaseUrl/api/v1/uploads" `
    -WebSession $session `
    -Form @{ file = Get-Item -LiteralPath $Path }

Write-Host "Job created: $($job.id)"

for ($attempt = 0; $attempt -lt 120; $attempt++) {
    Start-Sleep -Seconds 1
    $jobs = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/jobs" -WebSession $session
    $current = $jobs.items | Where-Object { $_.id -eq $job.id } | Select-Object -First 1
    if (-not $current) {
        continue
    }

    Write-Host ("Status: {0} ({1}%)" -f $current.status, $current.progress)
    if ($current.status -eq "completed") {
        Write-Host "Import complete."
        exit 0
    }
    if ($current.status -eq "failed") {
        throw "Import failed: $($current.error_message)"
    }
}

throw "Timed out waiting for the import."
