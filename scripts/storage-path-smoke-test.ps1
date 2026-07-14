param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$credential = New-Object Microsoft.PowerShell.Commands.PSCredential($Username, $securePassword)
$password = $credential.GetNetworkCredential().Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null
    if ($BookId -le 0) {
        $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=500" -WebSession $session
        $book = $library.items | Where-Object { $_.format -eq "epub" } | Select-Object -First 1
        if (-not $book) { throw "No EPUB was found." }
        $BookId = [long]$book.id
    }
    $publication = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub" -WebSession $session
    if (-not $publication.spine -or $publication.spine.Count -lt 1) { throw "The EPUB has no spine." }
    $url = "$BaseUrl$($publication.spine[0].content_url)"
    $response = Invoke-WebRequest -Method Get -Uri $url -WebSession $session
    if ($response.StatusCode -ne 200) { throw "The first resource returned HTTP $($response.StatusCode)." }
    Write-Host "Library path and EPUB reading are working." -ForegroundColor Green
    Write-Host "Book: $BookId · resource: $($publication.spine[0].path)"
    Write-Host "Reader: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
