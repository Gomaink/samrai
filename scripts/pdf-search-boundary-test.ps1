param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [string]$Query = "love",
    [int]$ForbiddenPage = 0
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
        $pdf = $library.items | Where-Object { $_.format -eq "pdf" -and $_.pdf_analysis_status -eq "complete" } | Select-Object -First 1
        if (-not $pdf) { throw "No indexed PDF was found." }
        $BookId = [long]$pdf.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    if ($book.format -ne "pdf") { throw "Book $BookId is not a PDF." }
    if ($book.pdf_analysis_status -ne "complete") { throw "The PDF has not finished indexing." }

    $encoded = [System.Uri]::EscapeDataString($Query)
    $results = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pdf/search?q=$encoded&limit=100" -WebSession $session

    Write-Host "PDF: '$($book.title)'" -ForegroundColor Green
    Write-Host "Search for '$Query': $($results.items.Count) page(s)."
    foreach ($item in $results.items) {
        $page = [int]$item.page_number + 1
        $flags = @()
        if ($item.text_source -eq "ocr") { $flags += "OCR" }
        if ($item.match_type -eq "repaired") { $flags += "reconstructed" }
        $suffix = if ($flags.Count) { " [$($flags -join ', ')]" } else { "" }
        Write-Host "  Page $page$suffix: $($item.excerpt)"
    }

    if ($ForbiddenPage -gt 0) {
        $forbidden = $results.items | Where-Object { ([int]$_.page_number + 1) -eq $ForbiddenPage }
        if ($forbidden) {
            throw "Regression detected: page $ForbiddenPage unexpectedly appeared for query '$Query'."
        }
        Write-Host "Forbidden page $ForbiddenPage absent: test passed." -ForegroundColor Green
    }
}
finally {
    $password = $null
}
