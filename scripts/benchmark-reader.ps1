param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [int]$Requests = 10
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$password = [System.Net.NetworkCredential]::new("", $securePassword).Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null
if ($BookId -le 0) {
    $books = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=100" -WebSession $session
    $book = $books.items | Where-Object { $_.format -in @('cbz','cbr','cb7','cbt') } | Select-Object -First 1
    if (-not $book) { throw "No comic with image pages was found." }
    $BookId = [long]$book.id
}
$times = @()
for ($i = 0; $i -lt $Requests; $i++) {
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $response = Invoke-WebRequest -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pages/0/image?width=1280&format=webp" -WebSession $session
    $watch.Stop()
    $times += $watch.Elapsed.TotalMilliseconds
    Write-Host ("{0,2}: {1,7:N1} ms · cache {2}" -f ($i + 1), $watch.Elapsed.TotalMilliseconds, $response.Headers['X-Page-Cache'])
}
$average = ($times | Measure-Object -Average).Average
$sorted = $times | Sort-Object
$p95Index = [Math]::Min($sorted.Count - 1, [Math]::Ceiling($sorted.Count * 0.95) - 1)
Write-Host "Average: $([Math]::Round($average, 1)) ms"
Write-Host "P95: $([Math]::Round($sorted[$p95Index], 1)) ms"
