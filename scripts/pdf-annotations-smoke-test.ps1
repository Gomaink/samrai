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
        $pdf = $library.items | Where-Object { $_.format -eq "pdf" } | Select-Object -First 1
        if (-not $pdf) { throw "No ready PDF was found. Upload one from the web interface first." }
        $BookId = [long]$pdf.id
    }

    $book = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId" -WebSession $session
    if ($book.format -ne "pdf") { throw "Book $BookId is not a PDF." }

    $range = Invoke-WebRequest -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/file" -WebSession $session -Headers @{ Range = "bytes=0-7" }
    if ($range.StatusCode -ne 206 -and $range.StatusCode -ne 200) { throw "Streaming PDF returned HTTP $($range.StatusCode)." }

    $created = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/books/$BookId/annotations" -WebSession $session -ContentType "application/json" -Body (@{
        page_number = 0
        kind = "area"
        color = "yellow"
        selected_text = ""
        note = "Highlight created by the smoke test; it will be deleted next."
        anchor = @{ rects = @(@{ x = 0.1; y = 0.1; width = 0.2; height = 0.05 }) }
    } | ConvertTo-Json -Depth 6)

    $annotations = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/books/$BookId/annotations" -WebSession $session
    Invoke-RestMethod -Method Delete -Uri "$BaseUrl/api/v1/annotations/$($created.id)" -WebSession $session | Out-Null

    Write-Host "PDF and annotations are working for '$($book.title)'." -ForegroundColor Green
    Write-Host "Streaming: HTTP $($range.StatusCode), $($range.Headers['Content-Type'])"
    Write-Host "Annotations visible before cleanup: $($annotations.items.Count)"
    Write-Host "Open: $BaseUrl/read/$BookId"
}
finally {
    $password = $null
}
