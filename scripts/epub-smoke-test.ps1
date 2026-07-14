param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [string]$Query = ""
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$credential = New-Object System.Management.Automation.PSCredential($Username, $securePassword)
$password = $credential.GetNetworkCredential().Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null

    if ($BookId -le 0) {
        $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=500" -WebSession $session
        $epub = $library.items | Where-Object { $_.format -eq "epub" } | Select-Object -First 1
        if (-not $epub) { throw "No EPUB was found. Import one from the browser first." }
        $BookId = [long]$epub.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    if ($book.format -ne "epub") { throw "Book $BookId is not an EPUB." }

    $publication = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub" -WebSession $session
    if (-not $publication.spine -or $publication.spine.Count -lt 1) { throw "The EPUB reading order is empty." }

    $firstUrl = "$BaseUrl$($publication.spine[0].content_url)"
    $resource = Invoke-WebRequest -Method Get -Uri $firstUrl -WebSession $session
    if ($resource.StatusCode -ne 200) { throw "The first document returned HTTP $($resource.StatusCode)." }

    Write-Host "EPUB: '$($book.title)'" -ForegroundColor Green
    Write-Host "Version: $($publication.version)"
    Write-Host "Layout: $($publication.layout)"
    Write-Host "Direction: $($publication.reading_direction)"
    Write-Host "Spine items: $($publication.spine.Count)"
    Write-Host "Table of contents items: $($publication.toc.Count)"
    Write-Host "First resource: HTTP $($resource.StatusCode) · $($resource.Headers.'Content-Type')"

    if ($Query.Trim().Length -ge 2) {
        $encoded = [System.Uri]::EscapeDataString($Query.Trim())
        $results = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub/search?q=$encoded&limit=50" -WebSession $session
        Write-Host "Search for '$Query': $($results.items.Count) section(s)."
        foreach ($item in $results.items) {
            Write-Host "  Section $([int]$item.spine_index + 1): $($item.excerpt)"
        }
    }

    Write-Host "Reader: $BaseUrl/read/$BookId" -ForegroundColor Green
}
finally {
    $password = $null
}
