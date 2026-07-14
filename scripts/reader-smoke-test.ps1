param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0
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

    if ($BookId -le 0) {
        $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books" -WebSession $session
        if (-not $library.items -or $library.items.Count -eq 0) {
            throw "No ready book was found in the library."
        }
        $BookId = [long]$library.items[0].id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    $page = Invoke-WebRequest -Method Head -Uri "$BaseUrl/api/v1/books/$BookId/pages/0/image" -WebSession $session

    Write-Host "Reader ready for '$($book.title)' ($($book.page_count) pages)." -ForegroundColor Green
    Write-Host "Page 1: HTTP $($page.StatusCode), $($page.Headers['Content-Type'])"
    Write-Host "Open: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
