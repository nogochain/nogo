# NogoChain 本地挖矿广播测试脚本
# 在 PowerShell 中运行

Write-Host "=== NogoChain 本地挖矿广播测试 ===" -ForegroundColor Cyan
Write-Host ""

$HTTP_PORT = 8080
$BASE_URL = "http://localhost:$HTTP_PORT"
$ADMIN_TOKEN = "test123"

# 1. 检查当前状态
Write-Host "1. 检查当前节点状态..." -ForegroundColor Yellow
try {
    $chainInfo = Invoke-RestMethod -Uri "$BASE_URL/chain/info" -Method Get
    Write-Host "   当前高度：$($chainInfo.height)" -ForegroundColor Cyan
    Write-Host "   最新块哈希：$($chainInfo.latestBlockHash.Substring(0, 16))..." -ForegroundColor Cyan
    Write-Host "   P2P 连接数：$($chainInfo.peersCount)" -ForegroundColor Cyan
} catch {
    Write-Host "   ❌ 无法获取节点状态" -ForegroundColor Red
    exit 1
}

Write-Host ""

# 2. 查看当前日志
Write-Host "2. 查看最近的日志..." -ForegroundColor Yellow
$logContent = Get-Content startup.log -Tail 20
$lastHeight = ($logContent | Select-String "height=(\d+)" | Select-Object -Last 1).Matches.Groups[1].Value
Write-Host "   日志中看到的最后高度：$lastHeight" -ForegroundColor Cyan

Write-Host ""

# 3. 手动触发挖矿
Write-Host "3. 触发挖矿..." -ForegroundColor Yellow
Write-Host "   发送挖矿请求..." -ForegroundColor Gray

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/mine/one" `
        -Method Post `
        -Headers @{
            "Authorization" = "Bearer $ADMIN_TOKEN"
            "Content-Type" = "application/json"
        } `
        -TimeoutSec 30
    
    $result = $response.Content | ConvertFrom-Json
    
    if ($result.success) {
        Write-Host "   ✅ 挖矿成功" -ForegroundColor Green
        if ($result.block) {
            Write-Host "   块高度：$($result.block.height)" -ForegroundColor Cyan
            Write-Host "   块哈希：$($result.block.hash.Substring(0, 16))..." -ForegroundColor Cyan
        }
    } else {
        Write-Host "   ⚠️  挖矿返回但无块：$($result.message)" -ForegroundColor Yellow
    }
} catch {
    Write-Host "   ❌ 挖矿请求失败：$($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""

# 4. 等待并查看日志
Write-Host "4. 等待 5 秒并查看日志..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

$logContent = Get-Content startup.log -Tail 50
Write-Host ""
Write-Host "   最近的日志（查找 broadcast）：" -ForegroundColor Cyan

$broadcastLogs = $logContent | Select-String "broadcast|mining|AddBlock"
if ($broadcastLogs.Count -eq 0) {
    Write-Host "   未找到相关日志" -ForegroundColor Gray
} else {
    $broadcastLogs | ForEach-Object {
        $line = $_.Line
        if ($line -match "broadcast") {
            Write-Host "   📡 $line" -ForegroundColor Yellow
        } elseif ($line -match "AddBlock") {
            Write-Host "   ➕ $line" -ForegroundColor Green
        } elseif ($line -match "mining") {
            Write-Host "   ⛏️  $line" -ForegroundColor Cyan
        }
    }
}

Write-Host ""

# 5. 检查挖矿后的高度
Write-Host "5. 检查挖矿后的高度..." -ForegroundColor Yellow
try {
    $chainInfo = Invoke-RestMethod -Uri "$BASE_URL/chain/info" -Method Get
    Write-Host "   当前高度：$($chainInfo.height)" -ForegroundColor Cyan
    Write-Host "   最新块哈希：$($chainInfo.latestBlockHash.Substring(0, 16))..." -ForegroundColor Cyan
} catch {
    Write-Host "   ❌ 无法获取节点状态" -ForegroundColor Red
}

Write-Host ""
Write-Host "=== 测试完成 ===" -ForegroundColor Cyan
Write-Host ""

Write-Host "下一步操作：" -ForegroundColor Yellow
Write-Host "1. 在同步节点上查看日志：tail -f *.log | grep 'received block|block accepted'"
Write-Host "2. 检查同步节点的高度是否增加"
Write-Host "3. 如果同步节点没有收到块，请提供日志输出"
Write-Host ""
