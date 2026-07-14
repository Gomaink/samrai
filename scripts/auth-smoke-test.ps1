param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [string]$Username = "admin"
)

$ErrorActionPreference = "Stop"

$securePassword = Read-Host "Password for $Username" -AsSecureString
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
try {
    $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)

    $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $status = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/auth/status" -WebSession $session

    $body = @{
        username = $Username
        password = $password
    } | ConvertTo-Json

    if ($status.setup_required) {
        Write-Host "Creating the first administrator..."
        $null = Invoke-RestMethod `
            -Method Post `
            -Uri "$BaseUrl/api/v1/auth/setup" `
            -ContentType "application/json" `
            -Body $body `
            -WebSession $session
    }
    else {
        Write-Host "Signing in..."
        $null = Invoke-RestMethod `
            -Method Post `
            -Uri "$BaseUrl/api/v1/auth/login" `
            -ContentType "application/json" `
            -Body $body `
            -WebSession $session
    }

    $me = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/auth/me" -WebSession $session
    Write-Host "Signed in as $($me.user.username); role $($me.user.role)."

    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/logout" -WebSession $session
    Write-Host "Sign-out complete."
}
finally {
    if ($passwordPointer -ne [IntPtr]::Zero) {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
    }
    Remove-Variable password -ErrorAction SilentlyContinue
}
