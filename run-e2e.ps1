# E2E Test: Start Minecraft Server with Network

$ErrorActionPreference = "Stop"
$DATA_DIR = ".stratum\e2e\data"
$RUNTIME_ROOT = ".stratum\e2e\runtime"
$AGENT_URL = "http://127.0.0.1:8787"

Write-Host "=== E2E Test: Start Minecraft Server ===" -ForegroundColor Cyan

# Cleanup
Write-Host "`n[Cleanup] Removing old test data..." -ForegroundColor Yellow
Remove-Item -Recurse -Force $DATA_DIR -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $RUNTIME_ROOT -ErrorAction SilentlyContinue

# Start Agent
Write-Host "`n[1/6] Starting Agent..." -ForegroundColor Green
$env:STRATUM_HTTP_PROXY = "http://127.0.0.1:10808"
$agentProcess = Start-Process -FilePath ".\stratum-agent.exe" -ArgumentList "serve", "--listen", "127.0.0.1:8787", "--runtime-root", $RUNTIME_ROOT, "--runtime-mode", "process" -PassThru -NoNewWindow
Start-Sleep -Seconds 3
Write-Host "Agent PID: $($agentProcess.Id)" -ForegroundColor Cyan

try {
    # Create Environment
    Write-Host "`n[2/6] Creating Environment..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR environments create --id fabric-1.17 --name "Fabric 1.17" --minecraft-version 1.17.1 --loader fabric --server-core fabric

    # Create Project
    Write-Host "`n[3/6] Creating Project..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR projects create --id test-proj --name "Test Project"

    # Create Room
    Write-Host "`n[4/6] Creating Room..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR rooms create --id test-room --project test-proj --name "Test Room" --environment fabric-1.17 --base-world "world:default"

    # Create Session
    Write-Host "`n[5/6] Creating Session..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR sessions create --id mc-server --project test-proj --room test-room --type shared --owner test-user

    # Start Session
    Write-Host "`n[6/6] Starting Minecraft Server (this may take 5-10 minutes)..." -ForegroundColor Green
    Write-Host "Downloading server jar, installing MCDR, materializing environment..." -ForegroundColor Yellow
    .\stratum.exe --data-dir $DATA_DIR --agent-url $AGENT_URL --agent-timeout 15m sessions start --id mc-server --runtime-profile dummy-process --actor test-user --operation-timeout 15m

    Write-Host "`n=== SUCCESS ===" -ForegroundColor Green
    Write-Host "Server started! Check logs:" -ForegroundColor Cyan
    Write-Host "  .\stratum.exe --agent-url $AGENT_URL agents logs --session-id mc-server --tail 50" -ForegroundColor White

} catch {
    Write-Host "`n=== ERROR ===" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
} finally {
    Write-Host "`nPress any key to stop agent and cleanup..." -ForegroundColor Yellow
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    Stop-Process -Id $agentProcess.Id -Force -ErrorAction SilentlyContinue
    Write-Host "Agent stopped." -ForegroundColor Cyan
}
