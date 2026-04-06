@echo off
chcp 65001 >nul
echo ========================================
echo   NogoChain 节点启动器
echo ========================================
echo.

REM 检查 nogo.exe 是否存在
if not exist "nogo.exe" (
    echo [错误] 未找到 nogo.exe，正在编译...
    echo.
    go build -o nogo.exe ./blockchain/cmd
    if errorlevel 1 (
        echo [错误] 编译失败！
        pause
        exit /b 1
    )
    echo [成功] 编译完成！
    echo.
)

echo [信息] 正在启动 NogoChain 节点...
echo.

REM 后台启动节点
start "NogoChain Node" nogo.exe

echo [信息] 节点启动中，请稍候...
timeout /t 3 /nobreak >nul

echo.
echo ========================================
echo   服务已启动
echo ========================================
echo.
echo [信息] 节点地址：http://localhost:8080
echo [信息] 区块浏览器：http://localhost:8080/explorer/
echo [信息] Web 钱包：http://localhost:8080/webwallet/
echo [信息] 提案管理：http://localhost:8080/proposals/
echo.
echo [提示] 正在打开 Web 钱包页面...
echo.

REM 等待节点完全启动
timeout /t 2 /nobreak >nul

REM 自动打开 Web 钱包页面
start http://localhost:8080/webwallet/

echo ========================================
echo   按 Ctrl+C 停止节点
echo ========================================
echo.

pause
