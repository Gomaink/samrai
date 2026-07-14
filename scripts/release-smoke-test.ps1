param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [switch]$DownloadBackup
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$credential = New-Object System.Management.Automation.PSCredential($Username, $securePassword)
$password = $credential.GetNetworkCredential().Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    $health = Invoke-RestMethod -Method Get -Uri "$BaseUrl/health"
    if ($health.status -ne "ok") { throw "Health check failed." }

    Invoke-RestMethod `
        -Method Post `
        -Uri "$BaseUrl/api/v1/auth/login" `
        -WebSession $session `
        -ContentType "application/json" `
        -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null

    $me = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/auth/me" -WebSession $session
    $settings = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/settings" -WebSession $session
    $dashboard = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/dashboard" -WebSession $session
    $ocr = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/system/ocr" -WebSession $session

    Write-Host "Server $($health.version) healthy." -ForegroundColor Green
    Write-Host "Instance: $($settings.name)"
    Write-Host "Account: $($me.user.username) ($($me.user.role))"
    Write-Host "Library: $($dashboard.stats.total_books) book(s), $($dashboard.stats.total_series) series"
    if ($ocr.available) {
        Write-Host "OCR: available ($($ocr.languages -join ', '))" -ForegroundColor Green
    } else {
        Write-Host "OCR: unavailable — $($ocr.message)" -ForegroundColor Yellow
    }

    if ($me.user.role -eq "admin") {
        $users = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/users" -WebSession $session
        $system = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/system/info" -WebSession $session
        Write-Host "Administration: $($users.items.Count) account(s), $($system.storage.library_bytes) bytes in the library."

        if ($DownloadBackup) {
            $path = Join-Path $PWD "samrai-smoke-backup.zip"
            Invoke-WebRequest -Method Get -Uri "$BaseUrl/api/v1/system/backup" -WebSession $session -OutFile $path
            if ((Get-Item $path).Length -le 0) { throw "Backup is empty." }
            Write-Host "Test backup saved to $path" -ForegroundColor Green
        }
    }

    Write-Host "v0.2.0-rc.6 smoke test complete." -ForegroundColor Green
}
finally {
    $password = $null
}
