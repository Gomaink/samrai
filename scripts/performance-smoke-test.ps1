param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [int]$BookId = 1
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$password = [System.Net.NetworkCredential]::new("", $securePassword).Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

$loginBody = @{ username = $Username; password = $password } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body $loginBody | Out-Null

Write-Host "Requesting optimized page..."
$response = Invoke-WebRequest -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pages/0/image?width=1280&format=webp" -WebSession $session
Write-Host "Type: $($response.Headers['Content-Type'])"
Write-Host "Cache: $($response.Headers['X-Page-Cache'])"
Write-Host "Size: $($response.RawContentLength) bytes"

Write-Host "Requesting it again to validate the cache..."
$response2 = Invoke-WebRequest -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pages/0/image?width=1280&format=webp" -WebSession $session
Write-Host "Cache on second request: $($response2.Headers['X-Page-Cache'])"

$metrics = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/system/metrics" -WebSession $session
Write-Host "Hits: $($metrics.images.hits) | Misses: $($metrics.images.misses) | Generated: $($metrics.images.generated)"
Write-Host "Cache used: $([math]::Round($metrics.images.cache_bytes / 1MB, 2)) MiB"
