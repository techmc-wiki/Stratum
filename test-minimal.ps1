# 最小化测试 - 只测试环境物化

Write-Host "=== 测试环境物化 ===" -ForegroundColor Cyan

$DATA_DIR = ".stratum\minimal-test"
$RUNTIME_ROOT = ".stratum\minimal-runtime"

Remove-Item -Recurse -Force $DATA_DIR -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force $RUNTIME_ROOT -ErrorAction SilentlyContinue

# 创建环境
Write-Host "`n创建 Environment..." -ForegroundColor Green
.\stratum.exe --data-dir $DATA_DIR environments create `
    --id fabric-test `
    --name "Fabric Test" `
    --minecraft-version 1.17.1 `
    --loader fabric `
    --server-core fabric

# 创建 project/room/session
.\stratum.exe --data-dir $DATA_DIR projects create --id proj1 --name "Project 1"
.\stratum.exe --data-dir $DATA_DIR rooms create --id room1 --project proj1 --name "Room 1" --environment fabric-test --base-world "world:default"
.\stratum.exe --data-dir $DATA_DIR sessions create --id session1 --project proj1 --room room1

Write-Host "`n=== Test Complete ===" -ForegroundColor Green
Write-Host "Data dir: $DATA_DIR" -ForegroundColor Cyan
Write-Host "Now you can run Agent and start Session to test full flow" -ForegroundColor Yellow
