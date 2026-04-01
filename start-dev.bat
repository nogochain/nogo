@echo off
chcp 65001 >nul
echo ========================================
echo   NogoChain Development Node
echo ========================================
echo.

cd /d "%~dp0"

echo [1/3] Building NogoChain...
go build -o nogo.exe .
if errorlevel 1 (
    echo.
    echo ERROR: Build failed!
    pause
    exit /b 1
)
echo ✓ Build completed successfully
echo.

echo [2/3] Creating configuration...
if not exist ".env" (
    echo Creating .env file...
    (
        echo # NogoChain Configuration
        echo ADMIN_TOKEN=test
        echo MINER_ADDRESS=NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf
        echo AUTO_MINE=true
        echo MINE_FORCE_EMPTY_BLOCKS=true
        echo CHAIN_ID=1
        echo RATE_LIMIT_REQUESTS=100
        echo RATE_LIMIT_BURST=20
    ) > ".env"
    echo ✓ Configuration created
) else (
    echo ✓ Using existing .env configuration
)
echo.

echo [3/3] Starting NogoChain node with mining...
echo.
echo Configuration:
type ".env"
echo.
echo ========================================
echo Starting server...
echo ========================================
echo.

nogo.exe server

pause
