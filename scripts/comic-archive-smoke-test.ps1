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

    if ($BookId -gt 0) {
        $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    }
    else {
        $response = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?status=all&sort=recent&limit=500" -WebSession $session
        $book = $response.items |
            Where-Object { $_.format -in @("cbr", "cb7", "cbt") } |
            Select-Object -First 1
    }

    if (-not $book) {
        throw "No CBR, CB7, or CBT book was found. Import one and try again."
    }
    if ($book.format -notin @("cbr", "cb7", "cbt")) {
        throw "Book $($book.id) has format '$($book.format)', not a prepared archive."
    }
    if ($book.page_count -lt 1) {
        throw "The book has no pages."
    }

    $pageUrl = "$BaseUrl/api/v1/books/$($book.id)/pages/0/image"
    $response = Invoke-WebRequest -Method Get -Uri $pageUrl -WebSession $session
    if ($response.StatusCode -ne 200) {
        throw "The first page returned HTTP $($response.StatusCode)."
    }
    $contentType = [string]$response.Headers["Content-Type"]
    if (-not $contentType.StartsWith("image/")) {
        throw "Unexpected content type on the first page: $contentType"
    }

    Write-Host "Comic archive is working." -ForegroundColor Green
    Write-Host "Book: $($book.title)"
    Write-Host "Format: $($book.format.ToUpperInvariant())"
    Write-Host "Pages: $($book.page_count)"
    Write-Host "First page: HTTP $($response.StatusCode), $contentType, $($response.RawContentLength) byte(s)"
    Write-Host "Open: $BaseUrl/read/$($book.id)"
}
finally {
    $password = $null
}
