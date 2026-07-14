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
        if (-not $epub) { throw "No ready EPUB was found. Upload one from the web interface first." }
        $BookId = [long]$epub.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    if ($book.format -ne "epub") { throw "Book $BookId is not an EPUB." }

    $publication = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub" -WebSession $session
    $spine = $publication.spine | Where-Object { $_.searchable -eq $true } | Select-Object -First 1
    if (-not $spine) { $spine = $publication.spine | Select-Object -First 1 }
    if (-not $spine) { throw "The EPUB has no section in its reading order." }

    $created = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/books/$BookId/epub/annotations" -WebSession $session -ContentType "application/json" -Body (@{
        spine_index = [int]$spine.index
        resource_path = [string]$spine.path
        kind = "highlight"
        color = "yellow"
        selected_text = "Temporary test highlight"
        note = "Temporary note; it will be removed next."
        anchor = @{
            start_path = "0"
            start_offset = 0
            end_path = "0"
            end_offset = 1
            prefix = ""
            suffix = ""
        }
    } | ConvertTo-Json -Depth 6)

    $annotations = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/epub/annotations" -WebSession $session
    $updated = Invoke-RestMethod -Method Patch -Uri "$BaseUrl/api/v1/epub-annotations/$($created.id)" -WebSession $session -ContentType "application/json" -Body (@{ note = "Note updated by smoke test."; color = "blue" } | ConvertTo-Json)
    Invoke-RestMethod -Method Delete -Uri "$BaseUrl/api/v1/epub-annotations/$($created.id)" -WebSession $session | Out-Null

    Write-Host "EPUB highlights and notes are working." -ForegroundColor Green
    Write-Host "Book: $($book.title)"
    Write-Host "Section tested: $($spine.index + 1) — $($spine.title)"
    Write-Host "Annotations visible before cleanup: $($annotations.items.Count)"
    Write-Host "Color after update: $($updated.color)"
    Write-Host "Open: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
