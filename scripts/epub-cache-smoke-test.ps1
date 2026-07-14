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
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null

    if ($BookId -le 0) {
        $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=500" -WebSession $session
        $epub = $library.items | Where-Object { $_.format -eq "epub" } | Select-Object -First 1
        if (-not $epub) { throw "No EPUB was found." }
        $BookId = [long]$epub.id
    }

    $publication = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub" -WebSession $session
    if (-not $publication.spine -or $publication.spine.Count -lt 2) {
        throw "The EPUB needs at least two spine items to test navigation."
    }

    $chapterUrl = "$BaseUrl$($publication.spine[1].content_url)"
    $first = Invoke-WebRequest -Method Get -Uri $chapterUrl -WebSession $session
    $etag = [string]$first.Headers.ETag
    if ([string]::IsNullOrWhiteSpace($etag)) { throw "The chapter did not return an ETag." }

    $second = Invoke-WebRequest -Method Get -Uri $chapterUrl -WebSession $session -Headers @{ "If-None-Match" = $etag } -SkipHttpErrorCheck
    $xFrameOptions = [string]$second.Headers."X-Frame-Options"
    $csp = [string]$second.Headers."Content-Security-Policy"

    if ($second.StatusCode -ne 304) { throw "Revalidation returned HTTP $($second.StatusCode), expected 304." }
    if ($xFrameOptions -ne "SAMEORIGIN") { throw "The 304 response returned X-Frame-Options '$xFrameOptions'; expected SAMEORIGIN." }
    if ($csp -notmatch "frame-ancestors\s+'self'") { throw "The 304 response does not allow a same-origin iframe: $csp" }

    Write-Host "EPUB cache/frame test passed." -ForegroundColor Green
    Write-Host "Chapter: $($publication.spine[1].title)"
    Write-Host "First response: HTTP $($first.StatusCode) · ETag $etag"
    Write-Host "Revalidation: HTTP $($second.StatusCode) · X-Frame-Options $xFrameOptions"
    Write-Host "Reader: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
