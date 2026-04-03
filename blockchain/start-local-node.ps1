# NogoChain 本地节点启动脚本
# 在 PowerShell 中运行此脚本

Write-Host "=== NogoChain 本地节点启动 ===" -ForegroundColor Cyan
Write-Host ""

# 设置环境变量
Write-Host "配置环境变量..." -ForegroundColor Yellow
$env:CHAIN_ID="1"
$env:AUTO_MINE="true"
$env:MINER_ADDRESS="NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048"
$env:WS_ENABLE="true"
$env:P2P_ENABLE="true"
$env:P2P_LISTEN_ADDR=":9090"
$env:P2P_PEERS="main.nogochain.org:9090"
$env:P2P_ADVERTISE_SELF="true"
$env:HTTP_PORT="8081"
$env:HTTP_HOST="127.0.0.1"
$env:ADMIN_TOKEN="test123"
$env:SYNC_ENABLE="true"
$env:MINE_FORCE_EMPTY_BLOCKS="true"
$env:MINE_INTERVAL_MS="20000"

Write-Host "  P2P_PEERS: $env:P2P_PEERS"
Write-Host "  HTTP_PORT: $env:HTTP_PORT"
Write-Host "  MINER_ADDRESS: $($env:MINER_ADDRESS.Substring(0, 16))..."
Write-Host ""

# 检查端口占用
Write-Host "检查端口占用..." -ForegroundColor Yellow
$port8081 = Get-NetTCPConnection -LocalPort 8081 -ErrorAction SilentlyContinue
if ($port8081) {
    Write-Host "  ⚠️  端口 8081 被占用，PID: $($port8081.OwningProcess)" -ForegroundColor Yellow
    Write-Host "  建议停止占用端口的进程或更改 HTTP_PORT"
    Write-Host ""
}

$port9090 = Get-NetTCPConnection -LocalPort 9090 -ErrorAction SilentlyContinue
if ($port9090) {
    Write-Host "  ⚠️  端口 9090 被占用，PID: $($port9090.OwningProcess)" -ForegroundColor Yellow
    Write-Host ""
}

# 启动节点
Write-Host "启动 NogoChain 节点..." -ForegroundColor Cyan
Write-Host ""

# 使用 Start-Process 启动，不阻塞当前终端
$process = Start-Process -FilePath ".\nogochain.exe" `
    -ArgumentList "server" `
    -PassThru `
    -RedirectStandardOutput "nogochain.log" `
    -RedirectStandardError "nogochain.err"

Write-Host "节点已启动，PID: $($process.Id)" -ForegroundColor Green
Write-Host "日志文件：nogochain.log" -ForegroundColor Cyan
Write-Host ""
Write-Host "等待 10 秒让服务启动..." -ForegroundColor Yellow
Start-Sleep -Seconds 10

Write-Host ""
Write-Host "检查节点状态..." -ForegroundColor Cyan
try {
    $chainInfo = Invoke-RestMethod -Uri "http://localhost:8081/chain/info" -Method Get -ErrorAction Stop
    Write-Host "  ✅ 节点运行正常" -ForegroundColor Green
    Write-Host "  链高度：$($chainInfo.height)" -ForegroundColor Cyan
    Write-Host "  P2P 连接数：$($chainInfo.peersCount)" -ForegroundColor Cyan
    Write-Host "  最新块哈希：$($chainInfo.latestBlockHash.Substring(0, 16))..." -ForegroundColor Cyan
} catch {
    Write-Host "  ❌ 无法连接到节点" -ForegroundColor Red
    Write-Host "  错误：$($_.Exception.Message)" -ForegroundColor Red
    Write-Host ""
    Write-Host "请查看日志文件：" -ForegroundColor Yellow
    Write-Host "  - nogochain.log (标准输出)"
    Write-Host "  - nogochain.err (错误输出)"
    Write-Host ""
    Write-Host "查看最近日志：" -ForegroundColor Cyan
    Write-Host "  Get-Content nogochain.log -Tail 50"
}

Write-Host ""
Write-Host "=== 启动完成 ===" -ForegroundColor Cyan
