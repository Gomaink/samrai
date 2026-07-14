param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [long]$BookId = 0,
    [ValidateSet("", "auto", "off", "on_demand", "background", "full")]
    [string]$SetMode = ""
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
        $pdf = $library.items | Where-Object { $_.format -eq "pdf" } | Select-Object -First 1
        if (-not $pdf) { throw "No PDF was found." }
        $BookId = [long]$pdf.id
    }

    if ($SetMode) {
        Invoke-RestMethod -Method Patch -Uri "$BaseUrl/api/v1/books/$BookId/pdf/ocr-mode" -WebSession $session -ContentType "application/json" -Body (@{ mode = $SetMode } | ConvertTo-Json) | Out-Null
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    foreach ($property in @("pdf_document_kind", "pdf_classification_status", "pdf_ocr_mode", "pdf_sampled_pages", "pdf_indexed_pages")) {
        if ($null -eq $book.$property) { throw "Missing response field: $property" }
    }

    Write-Host "PDF: $($book.title) (#$BookId)" -ForegroundColor Cyan
    Write-Host "Classification: $($book.pdf_classification_status)"
    Write-Host "Type: $($book.pdf_document_kind)"
    Write-Host "OCR mode: $($book.pdf_ocr_mode) · status $($book.pdf_ocr_status)"
    Write-Host "Sampled: $($book.pdf_sampled_pages) · indexed: $($book.pdf_indexed_pages) of $($book.page_count) · OCR: $($book.pdf_ocr_pages)"
    Write-Host "OCR policy is available: test passed." -ForegroundColor Green
}
finally {
    $password = $null
}
