# E2E Test: Start a real MCDR-managed Minecraft server.

$ErrorActionPreference = "Stop"

$DATA_DIR = ".stratum\e2e-mcdr\data"
$RUNTIME_ROOT = ".stratum\e2e-mcdr\runtime"
$AGENT_URL = "http://127.0.0.1:8787"
$PROXY = "http://127.0.0.1:10808"

Write-Host "=== E2E Test: Real Minecraft Server via MCDR ===" -ForegroundColor Cyan

Get-Process | Where-Object { $_.ProcessName -like "*stratum*" -or $_.ProcessName -like "java*" } | Stop-Process -Force -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $DATA_DIR -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $RUNTIME_ROOT -ErrorAction SilentlyContinue

$env:STRATUM_HTTP_PROXY = $PROXY
$agentProcess = Start-Process -FilePath ".\stratum-agent.exe" -ArgumentList @(
    "serve",
    "--listen", "127.0.0.1:8787",
    "--runtime-root", $RUNTIME_ROOT,
    "--runtime-profiles", "runtime-profiles\mcdr-fabric-1.17.json",
    "--runtime-mode", "mcdr",
    "--http-proxy", $PROXY
) -PassThru -NoNewWindow

Start-Sleep -Seconds 3
Write-Host "Agent PID: $($agentProcess.Id)" -ForegroundColor Cyan

try {
    .\stratum.exe --data-dir $DATA_DIR environments create --id fabric-1.17-mcdr --name "Fabric 1.17 MCDR" --minecraft-version 1.17.1 --loader fabric --server-core fabric --mcdr-required --runtime-profile mcdr-fabric-1.17 --runtime-profile-required
    .\stratum.exe --data-dir $DATA_DIR projects create --id e2e-project --name "E2E Project"
    .\stratum.exe --data-dir $DATA_DIR rooms create --id e2e-room --project e2e-project --name "E2E Room" --environment fabric-1.17-mcdr --base-world "world:default"
    .\stratum.exe --data-dir $DATA_DIR sessions create --id e2e-mc --project e2e-project --room e2e-room --type shared --owner e2e-user

    Write-Host "Starting session. This may take several minutes on first run..." -ForegroundColor Yellow
    .\stratum.exe --data-dir $DATA_DIR --agent-url $AGENT_URL --agent-timeout 20m sessions start --id e2e-mc --runtime-profile mcdr-fabric-1.17 --actor e2e-user --operation-timeout 20m

    Write-Host "Inspecting session..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR --agent-url $AGENT_URL --agent-timeout 2m sessions inspect --id e2e-mc

    Write-Host "Recent logs..." -ForegroundColor Green
    .\stratum.exe --data-dir $DATA_DIR --agent-url $AGENT_URL --agent-timeout 2m sessions logs --id e2e-mc

    Write-Host "=== SUCCESS: e2e-mc start command returned success ===" -ForegroundColor Green
    Write-Host "Runtime root: $RUNTIME_ROOT" -ForegroundColor Cyan
    Write-Host "Agent remains running for inspection. Stop it with: Stop-Process -Id $($agentProcess.Id)" -ForegroundColor Yellow
} catch {
    Write-Host "=== ERROR ===" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    Write-Host "Runtime root: $RUNTIME_ROOT" -ForegroundColor Yellow
    Stop-Process -Id $agentProcess.Id -Force -ErrorAction SilentlyContinue
    exit 1
}
