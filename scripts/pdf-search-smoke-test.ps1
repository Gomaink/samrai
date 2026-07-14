param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [string]$Query = "the"
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
        if (-not $pdf) {
            throw "No indexed PDF was found. Open a PDF in the reader and wait for the “Search ready” message."
        }
        $BookId = [long]$pdf.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    if ($book.format -ne "pdf") { throw "Book $BookId is not a PDF." }
    if ($book.pdf_analysis_status -ne "complete") {
        throw "The PDF has not been indexed yet (status: $($book.pdf_analysis_status)). Open it in the reader and wait for analysis."
    }

    $encoded = [System.Uri]::EscapeDataString($Query)
    $results = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pdf/search?q=$encoded&limit=10" -WebSession $session

    Write-Host "Indexed PDF: '$($book.title)'" -ForegroundColor Green
    Write-Host "Pages: $($book.page_count); text layer: $($book.pdf_text_layer)"
    Write-Host "Search for '$Query': $($results.items.Count) page(s) returned."
    foreach ($item in $results.items | Select-Object -First 3) {
        Write-Host "  Page $($item.page_number + 1) [$($item.text_source)/$($item.match_type)]: $($item.excerpt)"
    }
    Write-Host "Open: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
