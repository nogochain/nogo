@echo off
echo Starting NogoChain Node in Local Mode (Whitelist)...
echo.

set ADMIN_TOKEN=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
set MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
set HTTP_ADDR=127.0.0.1:8080
set P2P_ADDR=127.0.0.1:9090

echo Miner Address: %MINER_ADDRESS%
echo Network: Mainnet (Local Mode)
echo HTTP: %HTTP_ADDR%
echo P2P: %P2P_ADDR%
echo Auto Mining: Enabled
echo.

nogo.exe server %MINER_ADDRESS% mine

pause
