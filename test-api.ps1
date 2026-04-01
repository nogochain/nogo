# Test NogoChain API (No Proxy)
Write-Host "======================================" -ForegroundColor Cyan
Write-Host "  NogoChain Node Status Test" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
Write-Host ""

# Disable proxy
[System.Net.WebRequest]::DefaultWebProxy = $null

# Test 1: Chain Info
Write-Host "[1/4] Testing Chain Info API..." -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri "http://localhost:8080/chain/info" -Method Get -ErrorAction Stop
    Write-Host "✓ Chain Info API: OK" -ForegroundColor Green
    Write-Host "  Block Height: $($response.height)" -ForegroundColor White
    Write-Host "  Total Supply: $($response.totalSupply)" -ForegroundColor White
    Write-Host "  Difficulty: $($response.difficultyBits)" -ForegroundColor White
    Write-Host "  Peers: $($response.peersCount)" -ForegroundColor White
} catch {
    Write-Host "✗ Chain Info API: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Test 2: Web Wallet
Write-Host "[2/4] Testing Web Wallet..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/webwallet/" -Method Get -ErrorAction Stop -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Web Wallet: OK (Status: $($response.StatusCode))" -ForegroundColor Green
    } else {
        Write-Host "✗ Web Wallet: FAILED (Status: $($response.StatusCode))" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Web Wallet: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Test 3: Explorer
Write-Host "[3/4] Testing Block Explorer..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/explorer/" -Method Get -ErrorAction Stop -UseBasicParsing
    if ($response.StatusCode -eq 200) {
        Write-Host "✓ Block Explorer: OK (Status: $($response.StatusCode))" -ForegroundColor Green
    } else {
        Write-Host "✗ Block Explorer: FAILED (Status: $($response.StatusCode))" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Block Explorer: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# Test 4: Health Check
Write-Host "[4/4] Testing Health Check..." -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri "http://localhost:8080/health" -Method Get -ErrorAction Stop
    Write-Host "✓ Health Check: OK" -ForegroundColor Green
    Write-Host "  Status: $($response.status)" -ForegroundColor White
} catch {
    Write-Host "✗ Health Check: FAILED - $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "======================================" -ForegroundColor Cyan
Write-Host "  Test Complete!" -ForegroundColor Cyan
Write-Host "======================================" -ForegroundColor Cyan
