# NogoChain 本地节点 P2P 诊断脚本
# 在 PowerShell 中运行此脚本

Write-Host "=== NogoChain P2P 连接诊断 ===" -ForegroundColor Cyan
Write-Host ""

# 配置
$HTTP_PORT = 8081
$BASE_URL = "http://localhost:$HTTP_PORT"

Write-Host "HTTP 端口：$HTTP_PORT"
Write-Host "API 地址：$BASE_URL"
Write-Host ""

# 1. 检查节点是否运行
Write-Host "1. 检查节点状态..." -ForegroundColor Yellow
try {
    $chainInfo = Invoke-RestMethod -Uri "$BASE_URL/chain/info" -Method Get -ErrorAction Stop
    Write-Host "   ✅ 节点正在运行" -ForegroundColor Green
    Write-Host "   链高度：$($chainInfo.height)"
    Write-Host "   P2P 连接数：$($chainInfo.peersCount)"
    Write-Host "   最新块哈希：$($chainInfo.latestBlockHash.Substring(0, 16))..."
} catch {
    Write-Host "   ❌ 节点未运行或无法访问" -ForegroundColor Red
    Write-Host "   错误：$($_.Exception.Message)"
    Write-Host ""
    Write-Host "请启动节点：.\nogochain.exe server"
    exit 1
}

Write-Host ""

# 2. 检查 P2P Peers
Write-Host "2. 检查 P2P Peers..." -ForegroundColor Yellow
try {
    $peers = Invoke-RestMethod -Uri "$BASE_URL/p2p/peers" -Method Get -ErrorAction Stop
    
    if ($peers.peers -eq $null -or $peers.peers.Count -eq 0) {
        Write-Host "   ❌ 没有 P2P 连接！" -ForegroundColor Red
        Write-Host ""
        Write-Host "可能原因：" -ForegroundColor Yellow
        Write-Host "   - P2P_PEERS 配置错误"
        Write-Host "   - 目标节点未运行"
        Write-Host "   - 防火墙阻止连接"
        Write-Host "   - 网络不通"
        Write-Host ""
        Write-Host "当前配置的 P2P_PEERS: $env:P2P_PEERS" -ForegroundColor Cyan
    } else {
        Write-Host "   ✅ 已连接 P2P Peers" -ForegroundColor Green
        foreach ($peer in $peers.peers) {
            Write-Host "   - $peer" -ForegroundColor Cyan
        }
    }
} catch {
    Write-Host "   ❌ 无法获取 P2P 信息" -ForegroundColor Red
    Write-Host "   错误：$($_.Exception.Message)"
}

Write-Host ""

# 3. 检查 P2P 配置
Write-Host "3. 检查环境变量配置..." -ForegroundColor Yellow
Write-Host "   P2P_ENABLE: $env:P2P_ENABLE"
Write-Host "   P2P_LISTEN_ADDR: $env:P2P_LISTEN_ADDR"
Write-Host "   P2P_PEERS: $env:P2P_PEERS" -ForegroundColor Cyan
Write-Host "   P2P_ADVERTISE_SELF: $env:P2P_ADVERTISE_SELF"
Write-Host ""

# 检查配置是否正确
if ($env:P2P_ENABLE -ne "true") {
    Write-Host "   ⚠️  警告：P2P_ENABLE 应该设置为 'true'" -ForegroundColor Yellow
}

if ([string]::IsNullOrWhiteSpace($env:P2P_PEERS)) {
    Write-Host "   ⚠️  警告：P2P_PEERS 为空，无法连接到其他节点" -ForegroundColor Yellow
} else {
    # 测试 P2P_PEERS 配置
    $peerAddress = $env:P2P_PEERS
    Write-Host "   正在测试 P2P 连接：$peerAddress" -ForegroundColor Cyan
    
    # 解析 IP 和端口
    if ($peerAddress -match '^(?<ip>[\d\.a-zA-Z\-]+):(?<port>\d+)$') {
        $peerIp = $matches.ip
        $peerPort = $matches.port
        
        Write-Host "   解析结果：IP=$peerIp, Port=$peerPort"
        
        # 测试连通性
        try {
            $tcpClient = New-Object System.Net.Sockets.TcpClient
            $asyncResult = $tcpClient.BeginConnect($peerIp, $peerPort, $null, $null)
            $wait = $asyncResult.AsyncWaitHandle.WaitOne(5000) # 5 秒超时
            
            if ($wait) {
                $tcpClient.EndConnect($asyncResult)
                Write-Host "   ✅ P2P 端口 $peerPort 可达" -ForegroundColor Green
            } else {
                Write-Host "   ❌ P2P 端口 $peerPort 连接超时" -ForegroundColor Red
                Write-Host "      可能原因：防火墙阻止、目标节点未运行、网络不通"
            }
            $tcpClient.Close()
        } catch {
            Write-Host "   ❌ 无法连接到 P2P 地址：$_" -ForegroundColor Red
        }
    } else {
        Write-Host "   ⚠️  P2P_PEERS 格式可能不正确 (应该是 IP:PORT 格式)" -ForegroundColor Yellow
    }
}

Write-Host ""

# 4. 检查最近的挖矿和广播日志
Write-Host "4. 检查最近的日志..." -ForegroundColor Yellow
$logFiles = Get-ChildItem -Path "." -Filter "*.log" -ErrorAction SilentlyContinue

if ($logFiles.Count -eq 0) {
    Write-Host "   未找到日志文件" -ForegroundColor Gray
} else {
    # 读取最新的日志
    $latestLog = $logFiles | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    $logContent = Get-Content $latestLog.FullName -Tail 50
    
    # 查找关键日志
    $broadcastLogs = $logContent | Select-String "broadcast|mining|peer"
    
    if ($broadcastLogs.Count -eq 0) {
        Write-Host "   未找到相关日志" -ForegroundColor Gray
    } else {
        Write-Host "   最近 10 条相关日志：" -ForegroundColor Cyan
        $broadcastLogs | Select-Object -Last 10 | ForEach-Object {
            Write-Host "   $_"
        }
    }
}

Write-Host ""
Write-Host "=== 诊断完成 ===" -ForegroundColor Cyan
Write-Host ""

# 5. 提供修复建议
Write-Host "建议操作：" -ForegroundColor Yellow
Write-Host ""

if ([string]::IsNullOrWhiteSpace($env:P2P_PEERS)) {
    Write-Host "1. 设置正确的 P2P_PEERS：" -ForegroundColor Cyan
    Write-Host "   `$env:P2P_PEERS=`"创世节点公网 IP:9090`"" -ForegroundColor White
    Write-Host ""
}

Write-Host "2. 如果 P2P 连接数为 0：" -ForegroundColor Cyan
Write-Host "   - 停止节点 (Ctrl+C)" -ForegroundColor White
Write-Host "   - 修正 P2P_PEERS 配置" -ForegroundColor White
Write-Host "   - 重新启动：.\nogochain.exe server" -ForegroundColor White
Write-Host ""

Write-Host "3. 如果还是无法同步：" -ForegroundColor Cyan
Write-Host "   - 检查创世节点是否运行" -ForegroundColor White
Write-Host "   - 检查防火墙是否开放 9090 端口" -ForegroundColor White
Write-Host "   - 提供诊断输出以便进一步帮助" -ForegroundColor White
Write-Host ""
