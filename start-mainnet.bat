@echo off
echo Starting NogoChain Node in Mainnet Mode...
echo.

set ADMIN_TOKEN=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
set MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

echo Miner Address: %MINER_ADDRESS%
echo Network: Mainnet
echo Auto Mining: Enabled
echo.

nogo.exe server %MINER_ADDRESS% mine

pause
