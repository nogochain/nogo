# Test explorer endpoint
$uri = "http://localhost:8080/explorer/"
try {
    $client = New-Object System.Net.Http.HttpClient
    $response = $client.GetAsync($uri).Result
    Write-Host "Status Code: $($response.StatusCode)"
    Write-Host "Content-Type: $($response.Content.Headers.ContentType)"
    $content = $response.Content.ReadAsStringAsync().Result
    Write-Host "Content Length: $($content.Length)"
    if ($content -match "NogoChain") {
        Write-Host "✓ Explorer page loaded successfully!"
    } else {
        Write-Host "✗ Explorer page content incorrect"
    }
} catch {
    Write-Host "Error: $_"
}
