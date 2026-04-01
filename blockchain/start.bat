@echo off
chcp 65001 >nul
echo ========================================
echo   NogoChain Mainnet Node
echo ========================================
echo.

REM Set environment variables
set MINER_ADDRESS=NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf
set GENESIS_FILE=..\genesis\mainnet.json
set ADMIN_TOKEN=test
set AUTO_MINE=true
set MINE_FORCE_EMPTY_BLOCKS=true

echo Configuration:
echo - Miner Address: %MINER_ADDRESS%
echo - Genesis File: %GENESIS_FILE%
echo - Auto Mining: Enabled
echo - Force Empty Blocks: Enabled
echo.

echo Starting node...
echo.

cd /d "%~dp0"
go run . server

pause
