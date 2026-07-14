param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$credential = New-Object System.Management.Automation.PSCredential($Username, $securePassword)
$password = $credential.GetNetworkCredential().Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    Invoke-RestMethod `
        -Method Post `
        -Uri "$BaseUrl/api/v1/auth/login" `
        -WebSession $session `
        -ContentType "application/json" `
        -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null

    $dashboard = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/dashboard" -WebSession $session
    $books = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?status=all&sort=title&limit=10" -WebSession $session
    $series = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/series?sort=title&limit=10" -WebSession $session

    Write-Host "Catalog is available." -ForegroundColor Green
    Write-Host "Books: $($dashboard.stats.total_books)"
    Write-Host "In progress: $($dashboard.stats.reading_books)"
    Write-Host "Completed: $($dashboard.stats.completed_books)"
    Write-Host "Series: $($dashboard.stats.total_series)"
    Write-Host "Book query returned $($books.items.Count) item(s)."
    Write-Host "Series query returned $($series.items.Count) item(s)."
}
finally {
    $password = $null
}
