@echo off
REM NogoChain Node Startup Script for Windows
REM Usage: start-node.bat [miner_address] [admin_token]

setlocal enabledelayedexpansion

REM Configuration
set CONFIG_FILE=node-config.json
set MINER_ADDRESS=%1
set ADMIN_TOKEN=%2

REM Check if miner address is provided
if "%MINER_ADDRESS%"=="" (
    echo ERROR: Miner address is required
    echo Usage: start-node.bat [miner_address] [admin_token]
    echo Example: start-node.bat NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mytoken123
    exit /b 1
)

REM Check if admin token is provided
if "%ADMIN_TOKEN%"=="" (
    echo WARNING: Admin token not provided, using default
    set ADMIN_TOKEN=test123
)

echo ========================================
echo NogoChain Node Starting...
echo ========================================
echo Configuration File: %CONFIG_FILE%
echo Miner Address: %MINER_ADDRESS%
echo Admin Token: %ADMIN_TOKEN%
echo ========================================
echo.

REM Set required environment variables
set CHAIN_ID=1
set MINER_ADDRESS=%MINER_ADDRESS%
set ADMIN_TOKEN=%ADMIN_TOKEN%

REM Build if necessary
if not exist nogochain.exe (
    echo Building NogoChain...
    go build -o nogochain.exe .
    if errorlevel 1 (
        echo ERROR: Build failed
        exit /b 1
    )
    echo Build completed successfully
    echo.
)

REM Start the node
echo Starting NogoChain node...
echo.
nogochain.exe server

endlocal
