param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [string]$Query = "Petersburg"
)

$ErrorActionPreference = "Stop"
$securePassword = Read-Host "Password for $Username" -AsSecureString
$credential = New-Object System.Management.Automation.PSCredential($Username, $securePassword)
$password = $credential.GetNetworkCredential().Password
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

try {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -WebSession $session -ContentType "application/json" -Body (@{ username = $Username; password = $password } | ConvertTo-Json) | Out-Null

    $ocr = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/system/ocr" -WebSession $session
    if (-not $ocr.available) {
        throw "OCR unavailable: $($ocr.message)"
    }
    if ($ocr.languages -notcontains "por") {
        throw "Tesseract was found, but the 'por' language data is not installed. Languages: $($ocr.languages -join ', ')"
    }

    if ($BookId -le 0) {
        $library = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books?limit=500" -WebSession $session
        $pdf = $library.items | Where-Object { $_.format -eq "pdf" } | Select-Object -First 1
        if (-not $pdf) { throw "No PDF was found." }
        $BookId = [long]$pdf.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    Write-Host "Tesseract available; languages: $($ocr.languages -join ', ')" -ForegroundColor Green
    Write-Host "PDF: '$($book.title)'"
    Write-Host "Analysis: $($book.pdf_analysis_status); layer: $($book.pdf_text_layer); OCR: $($book.pdf_ocr_status)"
    Write-Host "OCR pages: $($book.pdf_ocr_pages); pages still invalid: $($book.pdf_invalid_pages)"

    if ($book.pdf_analysis_status -ne "complete" -or $book.pdf_ocr_status -in @("needed", "partial", "idle")) {
        Write-Host "Open $BaseUrl/read/$BookId and wait for analysis to finish before testing search." -ForegroundColor Yellow
        return
    }

    $encoded = [System.Uri]::EscapeDataString($Query)
    $results = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/pdf/search?q=$encoded&limit=10" -WebSession $session
    Write-Host "Search for '$Query': $($results.items.Count) page(s)."
    foreach ($item in $results.items | Select-Object -First 3) {
        Write-Host "  Page $($item.page_number + 1) [$($item.text_source)/$($item.match_type)]: $($item.excerpt)"
    }
}
finally {
    $password = $null
}
