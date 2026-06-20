# StratumMC 端到端启动测试

Write-Host "=== StratumMC 端到端测试: 启动 Minecraft 服务器 ===" -ForegroundColor Cyan

# 配置
$DATA_DIR = ".stratum\e2e-test-data"
$RUNTIME_ROOT = ".stratum\e2e-test-runtime"
$AGENT_LISTEN = "127.0.0.1:8787"
$HTTP_PROXY = "http://127.0.0.1:10808"  # 如果不需要代理，设置为空字符串

# 清理旧数据（可选）
if (Test-Path $DATA_DIR) {
    Write-Host "清理旧数据目录..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force $DATA_DIR
}
if (Test-Path $RUNTIME_ROOT) {
    Write-Host "清理旧运行时目录..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force $RUNTIME_ROOT
}

# 1. 创建 Environment
Write-Host "`n[1/7] 创建 Environment (Fabric 1.17)..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR environments create `
    --id fabric-1.17-test `
    --name "Fabric 1.17 Test" `
    --minecraft-version 1.17.1 `
    --loader fabric `
    --server-core fabric

if ($LASTEXITCODE -ne 0) {
    Write-Host "Environment 创建失败!" -ForegroundColor Red
    exit 1
}

# 2. 创建 Project
Write-Host "`n[2/7] 创建 Project..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR projects create `
    --id test-project `
    --name "E2E Test Project"

# 3. 创建 Room
Write-Host "`n[3/7] 创建 Room..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR rooms create `
    --id test-room `
    --project test-project `
    --name "Test Room" `
    --environment fabric-1.17-test `
    --base-world "world:default"

# 4. 创建 Session
Write-Host "`n[4/7] 创建 Session..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR sessions create `
    --id minecraft-test `
    --project test-project `
    --room test-room `
    --type shared `
    --owner test-user

# 5. 启动 Agent (后台)
Write-Host "`n[5/7] 启动 Agent..." -ForegroundColor Green
$agentArgs = @(
    "serve"
    "--listen", $AGENT_LISTEN
    "--runtime-root", $RUNTIME_ROOT
    "--runtime-profiles", "runtime-profiles\mcdr-fabric-1.17.json"
    "--runtime-mode", "process"
)

if ($HTTP_PROXY) {
    $agentArgs += "--http-proxy", $HTTP_PROXY
    Write-Host "使用 HTTP 代理: $HTTP_PROXY" -ForegroundColor Yellow
}

$agentJob = Start-Process -FilePath ".\stratum-agent.exe" -ArgumentList $agentArgs -PassThru -NoNewWindow
Write-Host "Agent 已启动 (PID: $($agentJob.Id))" -ForegroundColor Cyan
Start-Sleep -Seconds 2

# 6. 启动 Session
Write-Host "`n[6/7] 启动 Minecraft Session..." -ForegroundColor Green
Write-Host "这将:" -ForegroundColor Yellow
Write-Host "  - 检测 Java 运行时" -ForegroundColor Yellow
Write-Host "  - 下载 Fabric server jar" -ForegroundColor Yellow
Write-Host "  - 生成 Lucy manifest 和 lock" -ForegroundColor Yellow
Write-Host "  - 下载 Fabric API 和 Carpet mods" -ForegroundColor Yellow
Write-Host "  - 启动 Fabric 服务器进程" -ForegroundColor Yellow
Write-Host "`n请稍候，这可能需要几分钟..." -ForegroundColor Cyan

.\stratum.exe --data-dir $DATA_DIR `
    --agent-url "http://$AGENT_LISTEN" `
    sessions start `
    --id minecraft-test `
    --runtime-profile dummy-process `
    --actor test-user

if ($LASTEXITCODE -ne 0) {
    Write-Host "`nSession 启动失败!" -ForegroundColor Red
    Stop-Process -Id $agentJob.Id -Force
    exit 1
}

# 7. 检查状态
Write-Host "`n[7/7] 检查 Session 状态..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR `
    --agent-url "http://$AGENT_LISTEN" `
    sessions inspect --id minecraft-test

Write-Host "`n=== 查看日志 ===" -ForegroundColor Cyan
.\stratum.exe --data-dir $DATA_DIR `
    --agent-url "http://$AGENT_LISTEN" `
    sessions logs --id minecraft-test | Select-Object -Last 50

Write-Host "`n=== 测试完成 ===" -ForegroundColor Green
Write-Host "Agent PID: $($agentJob.Id)" -ForegroundColor Cyan
Write-Host "运行时目录: $RUNTIME_ROOT\minecraft-test" -ForegroundColor Cyan
Write-Host "`n要停止服务器:" -ForegroundColor Yellow
Write-Host "  .\stratum.exe --data-dir $DATA_DIR --agent-url http://$AGENT_LISTEN sessions stop --id minecraft-test --actor test-user" -ForegroundColor Gray
Write-Host "`n要停止 Agent:" -ForegroundColor Yellow
Write-Host "  Stop-Process -Id $($agentJob.Id)" -ForegroundColor Gray
