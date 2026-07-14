param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$password = [System.Net.NetworkCredential]::new("", $securePassword).Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null
    $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=10" -WebSession $session
    $book = $library.items | Select-Object -First 1
    if (-not $book) { throw "No ready book was found." }

    Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/books/$($book.id)/favorite" -WebSession $session | Out-Null
    $favorites = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/favorites" -WebSession $session
    if (-not ($favorites.books | Where-Object { $_.id -eq $book.id })) { throw "The book did not appear in favorites." }

    $sessionId = "smoke-$([guid]::NewGuid().ToString())"
    $page = [Math]::Max(0, [Math]::Min([int]$book.current_page, [int]$book.page_count - 1))
    Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/books/$($book.id)/progress" -WebSession $session -ContentType "application/json" -Body (@{ current_page = $page; location = @{}; session_id = $sessionId } | ConvertTo-Json -Depth 4) | Out-Null
    Start-Sleep -Milliseconds 1100
    Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/books/$($book.id)/progress" -WebSession $session -ContentType "application/json" -Body (@{ current_page = $page; location = @{}; session_id = $sessionId } | ConvertTo-Json -Depth 4) | Out-Null
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/reading-sessions/end" -WebSession $session -ContentType "application/json" -Body (@{ session_id = $sessionId } | ConvertTo-Json) | Out-Null

    $history = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/history" -WebSession $session
    if (-not ($history.items | Where-Object { $_.book_id -eq $book.id })) { throw "The session did not appear in reading history." }

    $annotations = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/annotations?limit=5" -WebSession $session
    Invoke-RestMethod -Method Delete -Uri "$BaseUrl/api/v1/books/$($book.id)/favorite" -WebSession $session | Out-Null

    Write-Host "Favorites, reading history, and the notebook are working." -ForegroundColor Green
    Write-Host "Tested book: $($book.title)"
    Write-Host "Visible sessions: $($history.items.Count)"
    Write-Host "Annotations returned: $($annotations.items.Count) of $($annotations.total)"
}
finally {
    $password = $null
}
