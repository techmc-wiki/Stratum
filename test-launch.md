# Minecraft 启动测试计划

## 目标
端到端测试：从 Session 创建到 Minecraft 服务器真实启动

## 前置条件

1. Java 17+ 已安装
2. Python 3.8+ 已安装（MCDR 需要）
3. mcdreforged 已安装：`pip install mcdreforged`
4. HTTP 代理运行在 127.0.0.1:10808（可选，用于下载）

## 启动流程

### 1. 准备 Environment

```bash
# 创建 Environment （1.17.1 Fabric）
stratum --data-dir .stratum/test-data environments create \
  --id fabric-1.17 \
  --name "Fabric 1.17 Test" \
  --minecraft-version 1.17.1 \
  --java-version 17 \
  --loader fabric \
  --server-core fabric \
  --mcdr-required

# 验证
stratum --data-dir .stratum/test-data environments inspect --id fabric-1.17
```

### 2. 准备 Project 和 Room

```bash
stratum --data-dir .stratum/test-data projects create --id test-project --name "Test Project"
stratum --data-dir .stratum/test-data rooms create --id test-room --project test-project --name "Test Room"
```

### 3. 创建 Session

```bash
stratum --data-dir .stratum/test-data sessions create \
  --id test-session \
  --project test-project \
  --room test-room \
  --environment fabric-1.17
```

### 4. 启动 Agent (带代理)

```bash
stratum-agent serve \
  --listen 127.0.0.1:8787 \
  --runtime-root .stratum/test-runtime \
  --runtime-profiles runtime-profiles/mcdr-fabric-1.17.json \
  --http-proxy http://127.0.0.1:10808 \
  --runtime-mode mcdr
```

### 5. 启动 Session

```bash
stratum --data-dir .stratum/test-data \
  --agent-url http://127.0.0.1:8787 \
  sessions start \
  --id test-session \
  --runtime-profile mcdr-fabric-1.17 \
  --actor test-user
```

## 预期行为

1. **Environment 物化**
   - 创建 session 运行时目录：`.stratum/test-runtime/test-session/`
   - 下载 Fabric server jar
   - 生成 lucy.yaml 和 lucy-lock.yaml
   - 使用 Lucy 下载 Fabric API 和 Carpet mods 到 `mods/`
   - 准备 MCDR 目录结构

2. **MCDR 启动**
   - Agent 读取 MCDR launch plan
   - 执行 `mcdreforged --start` (在 session 工作目录)
   - MCDR 读取 config.yml
   - MCDR 执行 `start_command`: `java -Xmx2G -jar server.jar nogui`
   - Minecraft 服务器启动

3. **验证**
   ```bash
   stratum --data-dir .stratum/test-data sessions inspect --id test-session
   # 应显示 state=running, runtimeMessage="Done (XX.XXXs)! ..."
   
   stratum --data-dir .stratum/test-data sessions logs --id test-session
   # 应显示 MCDR 和 Minecraft 服务器日志
   ```

## 当前缺失的关键部分

### ✅ 已有
- ServerJar 下载（Fabric 实现完整）
- Java 检测（完整实现）
- Lucy adapter（EmbeddedAdapter 可用）
- MCDR bridge（BuildLaunchPlan 可用）
- RuntimeProfile 配置（mcdr-fabric-1.17.json 存在）

### ❌ 需要实现

#### A. 修改 Agent runtime-mode
当前只支持 `dummy-process`，需要添加 `mcdr` 模式

#### B. 实现 MCDR RuntimeProfile 执行器
`internal/agent/process/process.go` 添加 `startMCDR` 方法：
- 读取 MCDR launch plan manifest
- 执行 mcdreforged 进程
- 监督进程生命周期

#### C. Environment 物化调用 Lucy
`process.MaterializeEnvironment` 需要：
- 生成 lucy.yaml 从 Environment 元数据
- 调用 Lucy PlanEnvironment
- 调用 Lucy InstallPackages

#### D. MCDR config.yml 生成
创建 config.yml，设置：
- `working_directory`: session 工作目录
- `start_command`: `java -Xmx2G -jar server.jar nogui`
- `handler`: `vanilla_handler` 或 `basic_handler`

#### E. server.properties 生成
基本的 server.properties：
```properties
server-port=25565
gamemode=creative
difficulty=peaceful
spawn-protection=0
max-players=20
online-mode=false
enable-command-block=true
```

## 最小实现优先级

1. **MCDR RuntimeProfile 执行器** （核心）
2. **Environment 物化集成** （生成 lucy.yaml + 调用 Lucy）
3. **MCDR config.yml 生成** （配置 MCDR 启动 Minecraft）
4. **Runtime mode 扩展** （支持 mcdr 模式）
5. **基础 server.properties** （最小配置）

## 简化方案（最快验证）

如果要最快看到效果，可以：

1. 手动准备一个 session 运行时目录：
   ```
   .stratum/test-runtime/manual-test/
   ├── server.jar  （手动下载 Fabric server）
   ├── mods/
   │   ├── fabric-api.jar （手动下载）
   │   └── carpet.jar （手动下载）
   ├── config/
   │   └── mcdreforged/
   │       └── config.yml （手动创建）
   └── eula.txt （echo "eula=true" > eula.txt）
   ```

2. 实现最小 MCDR 执行器
3. 通过 Agent 启动，验证流程

完整自动化可以逐步添加。
