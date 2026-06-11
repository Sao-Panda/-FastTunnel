@echo off
chcp 65001 >nul
title 速联 FastTunnel — 跨境专线代理

echo.
echo   ╔════════════════════════════════╗
echo   ║    速联 FastTunnel v1.0        ║
echo   ║    跨境专线代理 / API 网关      ║
echo   ╚════════════════════════════════╝
echo.
echo   本地管理: http://127.0.0.1:8580
echo   代理地址: http://127.0.0.1:9080
echo.

cd /d "%~dp0"

where node >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo [错误] 未找到 Node.js，请安装 https://nodejs.org
    pause
    exit /b 1
)

echo [启动] 速联 FastTunnel 正在启动...
start "" http://127.0.0.1:8580
node server.js

pause
