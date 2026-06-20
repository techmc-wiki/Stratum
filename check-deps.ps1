# 快速验证测试

Write-Host "=== 快速验证 Stratum 组件 ===" -ForegroundColor Cyan

# 1. 验证 Java
Write-Host "`n[1/4] 检测 Java..." -ForegroundColor Green
java -version 2>&1 | Select-Object -First 3

# 2. 验证 Python
Write-Host "`n[2/4] 检测 Python..." -ForegroundColor Green
python --version

# 3. 验证 mcdreforged
Write-Host "`n[3/4] 检测 mcdreforged..." -ForegroundColor Green
python -c "import mcdreforged; print(f'MCDReforged version: {mcdreforged.__version__}')"

if ($LASTEXITCODE -ne 0) {
    Write-Host "mcdreforged 未安装! 运行: pip install mcdreforged" -ForegroundColor Red
    exit 1
}

# 4. 验证代理（可选）
Write-Host "`n[4/4] 测试代理连接..." -ForegroundColor Green
try {
    $proxy = [System.Net.WebProxy]::new("http://127.0.0.1:10808")
    $client = [System.Net.WebClient]::new()
    $client.Proxy = $proxy
    $client.DownloadString("https://www.google.com") | Out-Null
    Write-Host "代理 127.0.0.1:10808 可用" -ForegroundColor Green
} catch {
    Write-Host "代理 127.0.0.1:10808 不可用，将尝试直连" -ForegroundColor Yellow
}

Write-Host "`n=== 所有组件就绪 ===" -ForegroundColor Green
Write-Host "现在可以运行: .\test-e2e.ps1" -ForegroundColor Cyan
