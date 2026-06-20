# Minecraft 服务器启动指南

## 快速开始

StratumMC 现在可以真正启动 Minecraft 服务器了！

### 前置条件

1. **Java 17+** — 运行 Fabric 1.17 需要
2. **Python 3.8+** — MCDR 需要
3. **mcdreforged** — 安装命令: `pip install mcdreforged`
4. **HTTP 代理** (可选) — 如果需要代理访问 Mojang/Fabric API，配置 127.0.0.1:10808

### 验证环境

```powershell
.\check-deps.ps1
```

### 端到端测试

```powershell
.\test-e2e.ps1
```

这将：
1. 创建 Environment (Fabric 1.17)
2. 创建 Project 和 Room
3. 创建 Session
4. 启动 Agent（支持 MCDR RuntimeProfile）
5. 启动 Session，自动执行：
   - 检测 Java 运行时
   - 下载 Fabric server jar
   - 生成 Lucy manifest 和 lock 文件
   - 下载 Fabric API 和 Carpet mods
   - 生成 MCDR config.yml
   - 启动 MCDR 进程
   - MCDR 启动 Minecraft 服务器
6. 检查状态和日志

## 实现的功能

### ✅ 已实现

#### 1. HTTP 代理支持
- Agent 支持 `--http-proxy` 标志
- 环境变量 `STRATUM_HTTP_PROXY`
- 用于 serverjar 下载

#### 2. ServerJar 下载
- Vanilla (Mojang 官方)
- Fabric (meta.fabricmc.net)
- Paper (papermc.io API)
- 自动 SHA-256 验证

#### 3. Java 运行时检测
- 自动检测 JAVA_HOME
- 扫描常见安装路径
- 解析 `java -version`
- 验证 Minecraft 版本兼容性

#### 4. Lucy 集成
- **EmbeddedAdapter 默认启用**
- 自动生成 lucy.yaml 从 Environment
- PlanEnvironment + LockEnvironment
- InstallPackages 到 mods/
- VerifyIntegrity 完整性检查

#### 5. MCDR RuntimeProfile 执行器
- `mcdr-python` RuntimeProfile 类型
- Agent 启动和监督 MCDR 进程
- 自动生成 config.yml
- Readiness check: "Done ("
- stdin stop strategy: `!!MCDR stop`

#### 6. 完整启动流程
- Environment materialization
- Session start → MCDR → Minecraft
- 进程监督和日志采集
- 状态检查和命令发送

### 架构变更

#### Agent Runtime Mode
原来只支持 `dummy-process`，现在支持：
- `dummy-process` — Go dummy 进程测试
- `process` — 通用进程执行
- `mcdr` — MCDR 监督模式

#### Lucy Adapter 初始化
- **旧行为**: `detectLucyAdapter` 检查 lucy.yaml 是否存在，不存在返回 NoopAdapter
- **新行为**: `createDefaultLucyAdapter` 总是创建 EmbeddedAdapter (除非 `STRATUM_LUCY_WORKSPACE=none`)
- **原因**: MaterializeEnvironment 会自己生成 lucy.yaml，不需要预先存在

## 手动启动示例

### 1. 启动 Agent

```powershell
.\stratum-agent.exe serve `
  --listen 127.0.0.1:8787 `
  --runtime-root .stratum/runtime `
  --runtime-profiles runtime-profiles/mcdr-fabric-1.17.json `
  --runtime-mode mcdr `
  --http-proxy http://127.0.0.1:10808
```

### 2. 创建 Environment

```powershell
.\stratum.exe --data-dir .stratum/data environments create `
  --id fabric-1.17 `
  --name "Fabric 1.17" `
  --minecraft-version 1.17.1 `
  --java-version 17 `
  --loader fabric `
  --server-core fabric `
  --mcdr-required
```

### 3. 创建 Project/Room/Session

```powershell
.\stratum.exe --data-dir .stratum/data projects create --id proj1 --name "Project 1"
.\stratum.exe --data-dir .stratum/data rooms create --id room1 --project proj1 --name "Room 1"
.\stratum.exe --data-dir .stratum/data sessions create `
  --id session1 `
  --project proj1 `
  --room room1 `
  --environment fabric-1.17
```

### 4. 启动 Session

```powershell
.\stratum.exe --data-dir .stratum/data `
  --agent-url http://127.0.0.1:8787 `
  sessions start `
  --id session1 `
  --runtime-profile mcdr-fabric-1.17 `
  --actor admin
```

### 5. 检查状态

```powershell
.\stratum.exe --data-dir .stratum/data `
  --agent-url http://127.0.0.1:8787 `
  sessions inspect --id session1
```

### 6. 查看日志

```powershell
.\stratum.exe --data-dir .stratum/data `
  --agent-url http://127.0.0.1:8787 `
  sessions logs --id session1
```

### 7. 停止 Session

```powershell
.\stratum.exe --data-dir .stratum/data `
  --agent-url http://127.0.0.1:8787 `
  sessions stop --id session1 --actor admin
```

## 运行时目录结构

```
.stratum/runtime/<session-id>/
├── config/
│   ├── lucy.yaml                          # Lucy manifest
│   ├── lucy-lock.yaml                     # Lucy lock
│   └── environment-materialization.json   # 物化元数据
├── work/
│   └── mcdr/
│       ├── config/
│       │   └── config.yml                 # MCDR 配置
│       ├── server/                        # Minecraft 工作目录
│       │   ├── server.jar                 # Fabric server
│       │   ├── eula.txt
│       │   └── server.properties
│       ├── plugins/                       # MCDR 插件目录
│       └── logs/                          # MCDR 日志
├── mods/
│   ├── fabric-api-*.jar                   # Lucy 下载
│   └── carpet-*.jar                       # Lucy 下载
├── world/                                 # Minecraft 世界
└── logs/                                  # Session 日志
```

## RuntimeProfile 配置

`runtime-profiles/mcdr-fabric-1.17.json`:

```json
{
  "runtime_profiles": [
    {
      "id": "mcdr-fabric-1.17",
      "name": "MCDR + Fabric 1.17 Terminal",
      "runtime_type": "mcdr-python",
      "command_argv": ["mcdreforged", "--start"],
      "working_dir": "",
      "stop_strategy": "stdin",
      "stop_stdin_command": "!!MCDR stop",
      "graceful_stop_timeout": "60s",
      "force_kill_timeout": "15s",
      "log_mode": "combined",
      "enabled": true,
      "readiness_check": {
        "type": "log-pattern",
        "pattern": "Done (",
        "timeout": "180s"
      }
    }
  ]
}
```

## 下一步

当前实现已经可以启动 Minecraft 服务器！接下来的增强方向：

1. **World checkpoint 备份/恢复** — 实际复制 world/ 目录
2. **1.12 环境支持** — Forge 1.12 + 旧版 Carpet
3. **Latest 环境支持** — 追踪最新 Minecraft 版本
4. **Web UI** — REST API 已就绪，可以构建前端
5. **用户认证** — 替代 shared-token 模式

## 故障排除

### ServerJar 下载失败
- 检查网络连接
- 配置 HTTP 代理: `--http-proxy http://127.0.0.1:10808`
- 检查防火墙设置

### MCDR 启动失败
- 确认 mcdreforged 已安装: `pip install mcdreforged`
- 检查 Python 版本: `python --version` (需要 3.8+)
- 查看日志: `sessions logs --id <session-id>`

### Lucy 包安装失败
- 检查网络连接（访问 Modrinth/CurseForge）
- 查看 materialization manifest: `<runtime-root>/<session-id>/config/environment-materialization.json`
- 检查 mods/ 目录权限

### Java 检测失败
- 确认 Java 17+ 已安装: `java -version`
- 设置 JAVA_HOME 环境变量
- Fabric 1.17 需要 Java 16+

## 原子提交

### 1. agent: add HTTP proxy configuration for serverjar downloads
- 添加 `--http-proxy` CLI 标志
- 调用 `serverjar.SetProxy()`

### 2. agent: enable EmbeddedAdapter by default for Lucy
- 修改 `detectLucyAdapter` 为 `createDefaultLucyAdapter`
- 总是创建 EmbeddedAdapter (除非 STRATUM_LUCY_WORKSPACE=none)
- 移除 lucy.yaml 预先存在的检查

### 3. agent: support mcdr runtime mode
- 扩展 runtime-mode 支持: dummy-process, process, mcdr
- 移除 dummy-process 限制

### 4. test: update Lucy adapter tests
- `TestDefaultLucyAdapterIsNoop` → `TestDefaultLucyAdapterIsEmbedded`
- 修复 `detectLucyAdapter` → `createDefaultLucyAdapter` 调用

### 5. docs: add Minecraft launch guide and test scripts
- 创建 test-e2e.ps1 端到端测试脚本
- 创建 check-deps.ps1 依赖检查脚本
- 创建 MINECRAFT_LAUNCH.md 启动指南
