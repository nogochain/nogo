@echo off
chcp 65001 >nul
echo ========================================
echo   NogoChain API Test
echo ========================================
echo.

echo Testing APIs...
echo.

REM Test 1: Health Check
echo [1/4] Testing Health Check...
curl -s http://localhost:8080/health >nul 2>&1
if %errorlevel% == 0 (
    echo ✓ Health Check: OK
) else (
    echo ✗ Health Check: FAILED
)

REM Test 2: Chain Info
echo.
echo [2/4] Testing Chain Info...
curl -s http://localhost:8080/chain/info >nul 2>&1
if %errorlevel% == 0 (
    echo ✓ Chain Info: OK
) else (
    echo ✗ Chain Info: FAILED
)

REM Test 3: Web Wallet
echo.
echo [3/4] Testing Web Wallet...
curl -s http://localhost:8080/webwallet/ >nul 2>&1
if %errorlevel% == 0 (
    echo ✓ Web Wallet: OK
) else (
    echo ✗ Web Wallet: FAILED
)

REM Test 4: Favicon
echo.
echo [4/4] Testing Favicon...
curl -s -I http://localhost:8080/explorer/favicon.ico >nul 2>&1
if %errorlevel% == 0 (
    echo ✓ Favicon: OK
) else (
    echo ✗ Favicon: FAILED
)

echo.
echo ========================================
echo   Test Complete!
echo ========================================
pause
