# StratumMC

> 面向受邀高级玩家——TMC、红石与世界机制研究者——的以「项目/房间」为中心的协作式 Minecraft 技术测试控制平面。

<!-- README-I18N:START -->

[English](./README.md) | **汉语**

<!-- README-I18N:END -->

StratumMC **不是**通用的 Minecraft 托管面板。它在明确的资源上限下协调共享测试房间、临时分叉会话、语义化检查点以及经过审批的制品管理——为可复现的技术实验而设计，而非无限的个人沙盒。

---

## 为什么选择 StratumMC

传统服务器面板把每个 Minecraft 实例当作主要对象。StratumMC 反转了这一点:**协作单元**(Project → Room → Session)才是产品本身,而运行中的服务器只是其下层的执行目标。

默认工作流契合技术研究实际发生的节奏:

```text
加入 Project
  → 进入共享 Room
  → 在长生命周期会话中与他人协作
  → 为高风险实验分叉出一个临时 Fork Session
  → 当结果可行时保存语义化 Checkpoint
  → 可选:把该检查点提升为项目里程碑
```

一切优先以 CLI 脚本化驱动。未来的 Web UI 只是便捷层,而非主接口。

---

## 特性

**领域模型** — Project、Room、Session、Checkpoint、Artifact、Environment、ResourcePolicy 与 Operation,具备显式状态迁移、审计历史与持久化协调。

**多 Agent 运行时监管** — Controller 作为元数据与调度的真相源;Agent 负责进程生命周期、运行时目录与资源观测。Agent 自动注册并每 30 秒发送心跳。

**声明式 RuntimeProfile** — 启动行为以 JSON 描述(`runtime-profiles/*.json`),绝不通过 shell 命令或用户提交的可执行文件。文件监听器支持热重载。

**三种内置环境** — 1.12 Forge(Java 8)、1.17 Fabric + MCDR + Carpet(Java 17),以及 Latest Fabric(从 Mojang manifest 自动解析,Java 21)。

**Lucy 集成** — 内嵌 Go 库(`github.com/mclucy/lucy`)负责 manifest 生成、依赖解析、lock 文件、包安装与完整性校验。绝不作为运行时所有者。

**MCDR 监管** — MCDR 作为子 RuntimeProfile 在 Agent 所有权下运行:启动/停止/重启、stdin 命令注入、基于日志模式就绪检查、优雅停止、崩溃检测、config.yml 生成与 Python venv 引导。

**服务端 jar 供应** — Vanilla(Mojang)、Fabric(Fabric Maven)、Paper(Paper API)与 Forge 1.12.2(Forge Maven),支持代理。

**世界检查点** — 四种一致性级别(`metadata_only`、`stopped`、`best_effort`、`command_quiesced`),zip + SHA-256 快照、带 zip-slip 防护的恢复、世界配置合并,以及在重启与制品应用前的预操作检查点。

**制品工作流** — 上传、哈希(SHA-256)、审批、暂存、物化、应用、校验。未审批的制品绝不会附加到共享会话。

**资源感知调度** — 全局、按项目、按用户限额,带排队与拒绝原因。

**容器编排** — `Dockerfile.agent` 按 Java 版本参数化,`docker-compose.yml` 启动三个隔离的 Agent(Java 8/17/21)连接到主机上的 Controller。

---

## 架构

```text
┌──────────────────────────┐        ┌─────────────────────────────┐
│      Controller          │        │           Agent(s)          │
│  (source of truth)       │        │   (owns runtime lifecycle)  │
│                          │◄──────►│                             │
│  • Projects / Rooms      │ HTTP   │  • Process supervision      │
│  • Sessions metadata     │  +     │  • RuntimeProfile registry  │
│  • Checkpoints metadata  │ tokens │  • Lucy materialization     │
│  • Artifacts metadata    │        │  • MCDR / Java / server jar │
│  • Scheduling / audit    │        │  • World snapshot / restore │
└──────────────────────────┘        └─────────────────────────────┘
              │                                   │
              ▼                                   ▼
        CLI (`stratum`)                Runtime directories
                                       Artifacts / mods / worlds
```

- **Controller**(`cmd/stratum-controller`)— 权威元数据、调度、Agent 注册表、审计历史
- **Agent**(`cmd/stratum-agent`)— 进程生命周期、RuntimeProfile 执行、物化、检查点 worker
- **CLI**(`cmd/stratum`)— 主用户接口,可脚本化,除 cobra 外不依赖框架
- **Lucy CLI**(`cmd/lucy`)— 内嵌 Lucy 库的独立封装

完整边界与所有权规则见 [`docs/architecture.md`](docs/architecture.md)。

> [!NOTE]
> MCDR 由 Agent 作为子 RuntimeProfile 监管——它绝不拥有 Stratum 顶层生命周期。Lucy 是规划/解析库,绝非运行时所有者。上传的 jar 受审批工作流隔离,绝不影响基础世界。

---

## 快速开始

### 前置条件

- Go 1.25+
- Java 8(用于 1.12)、Java 17(用于 1.17)、Java 21(用于 latest)——仅运行对应环境的 Agent 主机需要
- Python 3.9+ ——仅运行 MCDR 类型 profile 的 Agent 需要
- [Task](https://taskfile.dev)(`go install github.com/go-task/task/v3/cmd/task@latest`)——可选但推荐

### 1. 克隆

```bash
git clone --recurse-submodules https://github.com/stratummc/stratum.git
cd stratum
# 如果已克隆但未带子模块:
git submodule update --init --recursive
```

### 2. 构建

```bash
task build           # 二进制产物输出到 dist/local/
# 或直接构建:
go build -o dist/local/stratum ./cmd/stratum
go build -o dist/local/stratum-agent ./cmd/stratum-agent
go build -o dist/local/stratum-controller ./cmd/stratum-controller
```

### 3. 运行测试套件

```bash
go test -count=1 ./...
```

### 4. 启动 Controller 与 Agent

```bash
# 终端 1 —— Controller
go run ./cmd/stratum-controller serve --listen 127.0.0.1:8080 --data-dir .stratum/data

# 终端 2 —— Agent(自动向 Controller 注册)
go run ./cmd/stratum-agent serve \
  --listen 127.0.0.1:8787 \
  --controller-url http://127.0.0.1:8080 \
  --runtime-root .stratum/runtime
```

### 5. 创建项目、房间与会话

```bash
STRATUM="go run ./cmd/stratum --data-dir .stratum/data"

$STRATUM projects create --id demo --name "Demo Project"
$STRATUM rooms create --id lab --project demo --name "Lab Room"
$STRATUM sessions create --id sess-1 --project demo --room lab
```

### 6. 在 Agent 下启动会话

```bash
$STRATUM --agent-url http://127.0.0.1:8787 sessions start \
  --id sess-1 --actor researcher --runtime-profile mcdr-fabric-1.17

$STRATUM --agent-url http://127.0.0.1:8787 sessions inspect --id sess-1
$STRATUM --agent-url http://127.0.0.1:8787 sessions logs --id sess-1
$STRATUM --agent-url http://127.0.0.1:8787 sessions stop --id sess-1
```

使用 `stratum agents runtime-profiles --id <agent-id>` 查看可用 RuntimeProfile。

### Docker Compose(三个隔离 Agent)

```bash
cp .env.example .env   # 按需调整
docker compose up -d   # 启动 agent-java8 / java17 / java21
```

每个容器都会向主机 Controller 自动注册,地址为 `host.docker.internal:8080`。

---

## 工作流

一个典型的技术研究周期:

```bash
# 1. 建立共享房间
stratum rooms create --id 117-main-lab --project gtmc --name "1.17 Main Lab"

# 2. 为高风险实验分叉会话
stratum sessions create --id fork-rng-test --project gtmc --room 117-main-lab
stratum sessions start --id fork-rng-test --runtime-profile mcdr-fabric-1.17

# 3. 当结果可行时保存检查点
stratum checkpoints create \
  --id cp-working-piston-array \
  --session fork-rng-test \
  --actor alice \
  --consistency-level command_quiesced \
  --capture-world-profile \
  --notes "Stable piston array layout before scaling"

# 4. 为项目上传模组制品
stratum artifacts upload \
  --id carpet-fixes-1.0.jar \
  --project gtmc --file ./carpet-fixes-1.0.jar \
  --actor alice

# 5. 之后恢复到该检查点
stratum checkpoints restore --id cp-working-piston-array --session fork-rng-test --auto-stop --auto-start
```

涵盖实验分叉、配置回滚与可复现测试的端到端示例见 [`docs/workflows/`](docs/workflows/)。

---

## 文档

- [`docs/architecture.md`](docs/architecture.md) — 组件边界、所有权规则、领域模型
- [`docs/runtime.md`](docs/runtime.md) — Agent 运行时监管与 RuntimeProfile 系统
- [`docs/cli-reference.md`](docs/cli-reference.md) — 完整 CLI 命令参考
- [`docs/checkpoints.md`](docs/checkpoints.md) — 检查点创建、一致性级别与回滚
- [`docs/world-profile.md`](docs/world-profile.md) — 世界配置捕获与恢复
- [`docs/operations.md`](docs/operations.md) — 持久化操作、幂等性、审计
- [`docs/storage.md`](docs/storage.md) — 仓储抽象与元数据持久化
- [`docs/lucy-integration.md`](docs/lucy-integration.md) — Lucy 依赖管理
- [`docs/mcdr.md`](docs/mcdr.md) — MCDR 子 RuntimeProfile 契约
- [`docs/agent.md`](docs/agent.md) — Agent HTTP 传输与能力
- [`docs/security.md`](docs/security.md) — 安全规则与制品隔离
- [`docs/status.md`](docs/status.md) — 当前实现状态
- [`docs/routemap.md`](docs/routemap.md) — 分阶段开发路线图
- [`docs/mvp.md`](docs/mvp.md) — MVP 范围与明确的非目标
- [`MINECRAFT_LAUNCH.md`](MINECRAFT_LAUNCH.md) — 关于真实 Minecraft 启动已验证与未验证的内容

每个被持久化对象的 JSON schema 位于 [`schemas/`](schemas/)。

---

## 安全边界

- 基础世界为不可变 / 只读。
- 共享房间比 fork 或 private 会话执行更严格的权限。
- 危险操作(重启、制品应用)会先创建预操作检查点。
- 上传的 jar 需要元数据、哈希与审批,才能触及共享会话。
- RuntimeProfile 仅为声明式 JSON——CLI 绝不接受可执行文件或 shell 命令输入。
- 世界恢复使用 zip-slip 防护(拒绝符号链接、`..`、绝对路径)。
- Agent 容器以隔离的命名卷运行。

---

## 状态

阶段 1–4(核心基础设施、运行时执行、世界管理、多环境支持)与阶段 6a/6c(多 Agent 协调、容器编排)已完成。通过 MCDR 的真实 Minecraft 启动是当前重点——现有测试验证 Stratum 的运行时管道;真正的 Java/Minecraft 冒烟测试是下一个里程碑。

权威的带日期状态表见 [`docs/status.md`](docs/status.md)。

> [!IMPORTANT]
> 认证目前仅支持共享令牌。用户账户、RBAC 与项目成员(阶段 6b)尚未实现。

---

## 开发

```bash
task fmt     # go fmt
task vet     # go vet
task test    # go test -count=1 ./...
task ci      # deps + vet + test + linux-amd64 构建(对齐 GitHub Actions)
```

提交前对改动的 Go 文件运行 `gofmt`,并运行 `go test ./...`。遵循 [`docs/workflow.md`](docs/workflow.md) 与 [`AGENTS.md`](AGENTS.md) 中记录的原子化变更与提交信息前缀策略(`docs:`、`domain:`、`lifecycle:`、`agent:`、`cli:`、`test:`……)。

---

## 许可证

Apache 2.0 —— 见 [`LICENSE`](LICENSE)。
