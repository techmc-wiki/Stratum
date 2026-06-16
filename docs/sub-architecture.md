## 架构细化方案

---

### 1. **Fork Session 模块**

**问题**：TypeFork 存在但无专用创建逻辑，缺乏来源溯源。

**设计方案**：

```
internal/session/fork/
  ├── fork.go              # ForkProvenance struct, ForkOptions
  ├── service.go           # Service.CreateFork(ctx, opts) -> (Session, error)
  └── service_test.go

ForkProvenance {
  SourceType: checkpoint | session | room
  SourceID: string
  SourceCheckpointID: string (optional)
  CreatorID: string
  Reason: string
  PreForkCheckpointID: string
  InheritedEnvironmentID: string
  InheritedArtifactIDs: []string
  InheritedServerConfig: map[string]string
}
```

**接口契约**：
- `CreateFork(ctx, ForkOptions) (Session, error)` → 触发 checkpoint.Service.CreatePreOperationCheckpoint → session.Service.Create → 设置 ForkProvenance
- Fork Session 的 TTL 由 resourcepolicy 管理
- Fork 必须创建 pre-fork checkpoint（除非明确 `--skip-checkpoint`）

**依赖**：
- checkpoint.Service（创建检查点）
- session.Service（创建会话）
- permission.Service.CanFork

---

### 2. **World Manager 模块**

**问题**：完全未实现，Room.BaseWorldRef 无人管理。

**设计方案**：

```
internal/world/
  ├── world.go              # World, WorldProfile, WorldTemplate
  ├── repository.go         # Repository interface
  ├── service/
  │   ├── service.go        # Manager: Import/Export/Clone/Reset/Fork
  │   └── service_test.go
  └── validation.go

World {
  ID: string
  ProjectID: string
  Name: string
  Type: template | base | session_private
  StorageRef: string        # backend storage reference
  Seed: string
  GeneratorSettings: map[string]string
  SizeBytes: int64
  CreatedAt: time.Time
  Metadata: map[string]string
}

WorldProfile {
  ID: string
  Name: string
  GeneratorType: flat | void | normal | custom
  Seed: string
  GeneratorSettings: map[string]string
  PreloadChunks: bool
  PreloadRadius: int
}
```

**接口契约**：
- `Import(ctx, projectID, file) (World, error)` → 上传世界文件到存储后端
- `Clone(ctx, sourceWorldID, targetSessionID) (World, error)` → 为 fork/私有会话克隆世界
- `Reset(ctx, sessionID, worldTemplateID) error` → 从模板重置会话世界
- `Export(ctx, worldID, format) (io.ReadCloser, error)` → 导出世界文件
- `Fork(ctx, checkpointWorldRef, targetSessionID) (World, error)` → 从检查点世界状态创建分支

**存储边界**：
- 世界文件本身存储在 `integration/storage/Backend`
- World 元数据存储在 Controller repository
- Agent 通过 `AgentClient.RestoreWorld(ctx, storageRef, sessionID)` 拉取世界

---

### 3. **Checkpoint 一致性等级模块**

**问题**：架构定义了 6 个等级，代码只有 2 个状态。

**设计方案**：

```
internal/checkpoint/consistency/
  ├── consistency.go        # Level enum + validation
  └── consistency_test.go

// 明确定义一致性等级为独立类型
type Level string

const (
  LevelMetadataOnly     Level = "metadata_only"      // 仅记录元数据
  LevelStopped          Level = "stopped"            // 会话已停止
  LevelBestEffort       Level = "best_effort"        // Agent 文件快照
  LevelCommandQuiesced  Level = "command_quiesced"   // 写盘静默
  LevelPluginBackup     Level = "plugin_backup"      // MCDR 插件备份钩子
  LevelMCBridgePrepared Level = "mc_bridge_prepared" // MC Bridge 准备
)

// Checkpoint struct 添加字段
Checkpoint {
  ...
  ConsistencyLevel: consistency.Level
  ConsistencyMetadata: map[string]string  // 存储等级特定的元数据
}
```

**接口契约**：
- `checkpoint.Service.Create(ctx, params)` → 检查 `params.ConsistencyLevel`
- Level >= `LevelBestEffort` → 调用 `agent.CreateCheckpoint(sessionID, level)`
- Level >= `LevelCommandQuiesced` → 通过 MCDR 发送静默命令
- Level == `LevelMCBridgePrepared` → 调用 MC Bridge.Prepare() → 返回 `PrepareToken`

**验证逻辑**：
- 如果请求的 level 需要 MCDR/MC Bridge 但环境未启用 → 降级到 `LevelBestEffort` + 警告
- 恢复时检查 consistency level → 显示可靠性警告

---

### 4. **Permission 模块完整化**

**问题**：只有 2 个检查函数，架构列举了 15 个权限。

**设计方案**：

```
internal/permission/
  ├── permission.go         # Permission enum + PermissionSet
  ├── policy.go             # Policy struct + default policies
  ├── service/
  │   ├── service.go        # Unified Check(ctx, actor, permission, target) error
  │   └── service_test.go
  └── repository.go         # Policy storage (future)

type Permission string

const (
  PermProjectAdmin         Permission = "project.admin"
  PermProjectView          Permission = "project.view"
  PermRoomCreate           Permission = "room.create"
  PermRoomConfigure        Permission = "room.configure"
  PermSessionCreate        Permission = "session.create"
  PermSessionFork          Permission = "session.fork"
  PermSessionJoin          Permission = "session.join"
  PermCheckpointCreate     Permission = "checkpoint.create"
  PermCheckpointRestore    Permission = "checkpoint.restore"
  PermArtifactUpload       Permission = "artifact.upload"
  PermArtifactApprove      Permission = "artifact.approve"
  PermDebugUse             Permission = "debug.use"
  PermWorldModify          Permission = "world.modify"
)

type Policy struct {
  ProjectID: string
  RolePermissions: map[project.Role][]Permission
}
```

**接口契约**：
- `Check(ctx, actorID, permission, targetResourceID) error` → 统一权限检查入口
- 服务层调用: `permission.Check(ctx, actor, PermSessionFork, sessionID)`
- 默认策略通过 `DefaultPolicy()` 返回，与角色绑定
- 未来支持 `SavePolicy(ctx, Policy)` 自定义项目策略

**迁移路径**：
- `CanCreateSession` → `Check(ctx, actor, PermSessionCreate, projectID)`
- `CanAttachArtifact` → `Check(ctx, actor, PermArtifactUpload, projectID)`

---

### 5. **Audit 自动发送模块**

**问题**：每个服务手动调用 `s.audit()`，非自动化。

**设计方案**：

```
internal/audit/interceptor/
  ├── interceptor.go        # Repository wrapper
  └── interceptor_test.go

type AuditInterceptor struct {
  inner storage.Repository
  audit audit.Logger
  actor func(context.Context) string  # 从 ctx 提取 actor
}

func (a *AuditInterceptor) SaveSession(ctx context.Context, s session.Session) error {
  prev, _ := a.inner.GetSession(ctx, s.ID)
  err := a.inner.SaveSession(ctx, s)
  if err == nil {
    a.audit.Log(ctx, audit.NewEvent(
      a.actor(ctx), "session.update", s.ID,
      metadata with prev/next state,
    ))
  }
  return err
}
```

**接口契约**：
- `NewAuditInterceptor(repo, auditLogger, actorExtractor) Repository` → 返回包装后的 Repository
- 在 CLI/HTTP handler 初始化时包装 repository
- Actor 从 context 中提取（`context.WithValue(ctx, actorKey, userID)`）

**trade-off**：
- 优点：自动化，无需服务层记住调用
- 缺点：性能（每次保存都记录），可通过采样或异步批处理优化

---

### 6. **MCDR Integration 完整化**

**问题**：只有配置存根和启动计划，无真实守护进程管理。

**设计方案**：

```
internal/agent/mcdr/
  ├── supervisor.go         # Supervisor: Start/Stop/Restart/SendCommand
  ├── supervisor_test.go
  ├── config.go             # 现有 config_stub.go 重命名
  └── process.go            # 进程监控逻辑

type Supervisor struct {
  runtimeRoot: string
  profile: RuntimeProfile
  process: *process.RuntimeProcess  # 复用 agent/process 的 Supervisor
  stdin: io.Writer
}

func (s *Supervisor) Start(ctx) error
func (s *Supervisor) Stop(ctx, gracePeriod) error
func (s *Supervisor) SendCommand(ctx, command string) error
func (s *Supervisor) Logs(ctx) (io.ReadCloser, error)
```

**接口契约**：
- `RuntimeProfile` 为 MCDR 定义专用类型：`mcdr-python`
- `agent.ProcessAgent.StartSession` → 检查环境是否启用 MCDR → 启动 `mcdr.Supervisor` 作为子进程
- MCDR 崩溃 → `Supervisor` 检测 → 调用 `session.Service.MarkCrashed`

**配置示例**：

```yaml
runtimeProfiles:
  - id: mcdr-python-1.17
    type: mcdr-python
    command: python3
    args: ["-m", "mcdreforged"]
    workdir: "{sessionRoot}/mcdr"
    env:
      MCDR_CONFIG: "{sessionRoot}/mcdr/config.yml"
```

---

### 7. **MC Bridge / Debug Mod 接口定义**

**问题**：完全未实现，但是未来关键功能。

**设计方案（接口先行）**：

```
internal/integration/mcbridge/
  ├── bridge.go             # Bridge interface
  ├── noop.go               # NoopBridge (default)
  └── client.go             # 未来 HTTP/WebSocket client

type Bridge interface {
  // Checkpoint 准备
  Prepare(ctx, sessionID) (PrepareToken, error)
  Commit(ctx, token) error
  Abort(ctx, token) error
  
  // 调试功能（未来）
  FreezeWorld(ctx, sessionID) error
  UnfreezeWorld(ctx, sessionID) error
  QueryEntities(ctx, sessionID, selector) ([]Entity, error)
  InspectBlock(ctx, sessionID, pos BlockPos) (BlockState, error)
}

type NoopBridge struct{}

func (NoopBridge) Prepare(ctx, sessionID) (PrepareToken, error) {
  return PrepareToken{}, errors.New("mc bridge not available")
}
```

**接口契约**：
- `checkpoint.Service` 在 `ConsistencyLevel >= LevelMCBridgePrepared` 时调用 `Bridge.Prepare`
- 如果 Bridge 返回错误 → 降级到 `LevelBestEffort`
- 默认注入 `NoopBridge`

**未来实现路径**：
1. 开发 Minecraft mod（Fabric/Forge）监听 WebSocket
2. 实现 `WebSocketBridge` client
3. Agent 在环境物化时检查是否启用 bridge → 注入配置到 Minecraft mods 目录

---

### 8. **Container Runtime 接口定义**

**问题**：完全未实现，但架构已描述。

**设计方案（接口先行）**：

```
internal/agent/container/
  ├── runtime.go            # Runtime interface
  ├── docker.go             # DockerRuntime (future)
  └── noop.go               # NoopRuntime (current)

type Runtime interface {
  Create(ctx, Config) (ContainerID, error)
  Start(ctx, ContainerID) error
  Stop(ctx, ContainerID, timeout) error
  Remove(ctx, ContainerID) error
  Logs(ctx, ContainerID) (io.ReadCloser, error)
  Exec(ctx, ContainerID, cmd []string) (io.ReadCloser, error)
  Inspect(ctx, ContainerID) (Status, error)
}

type Config struct {
  Image: string
  Volumes: map[string]string  # host:container
  Env: map[string]string
  Command: []string
  ResourceLimits: ResourceLimits
}

type NoopRuntime struct{}  // 所有方法返回 ErrNotSupported
```

**接口契约**：
- `RuntimeProfile` 添加 `ContainerConfig` 字段（可选）
- `agent.ProcessAgent.StartSession` → 如果 profile 有容器配置 → 调用 `container.Runtime.Create`
- 默认注入 `NoopRuntime`

---

### 9. **Resource Scheduler 高级模块**

**问题**：只有计数模型，缺 CPU/内存/磁盘检查。

**设计方案**：

```
internal/scheduler/capacity/
  ├── capacity.go           # HostCapacity, Requirement
  ├── checker.go            # CapacityChecker interface
  └── checker_test.go

type HostCapacity struct {
  HostID: string
  CPUCores: int
  MemoryMB: int64
  DiskMB: int64
  RunningSessionsCount: int
}

type Requirement struct {
  CPUCores: int
  MemoryMB: int64
  DiskMB: int64
}

type CapacityChecker interface {
  Check(ctx, hostID, req Requirement) (Decision, error)
}

// 计数模型保留为 CountingChecker
// 新增 ResourceChecker (查询 agent 资源使用)
```

**接口契约**：
- `scheduler.Service.Evaluate` → 先检查计数 → 再检查容量（如果启用）
- Agent 通过 `AgentClient.ReportResources(ctx) (ResourceUsage, error)` 上报资源
- Controller 缓存 Agent 资源状态（TTL 30s）

**配置**：

```yaml
resourcePolicy:
  mode: counting | capacity
  capacityLimits:
    cpuCoresPerSession: 2
    memoryMBPerSession: 4096
    diskMBPerSession: 10240
```

---

### 10. **Checkpoint Orchestrator 完整化**

**问题**：worldcheckpoint.Worker 是空存根。

**设计方案**：

```
internal/agent/worldcheckpoint/
  ├── checkpoint.go         # Worker: Create/Restore
  ├── worker_test.go
  └── strategy.go           # CopyStrategy vs SnapshotStrategy

type Worker struct {
  storage storage.Backend
  logger Logger
}

func (w *Worker) Create(ctx, params CreateParams) (CheckpointRef, error) {
  // 1. 调用 storage.CreateWorldSnapshot(sessionID)
  // 2. 打包环境清单
  // 3. 打包制品应用记录
  // 4. 返回存储引用
}

func (w *Worker) Restore(ctx, ref CheckpointRef, targetSessionID) error {
  // 1. 从存储拉取检查点
  // 2. 恢复世界文件到 target session
  // 3. 恢复环境清单
  // 4. 触发制品重新应用
}
```

**接口契约**：
- `checkpoint.Service.Create` → 如果 `ConsistencyLevel >= LevelBestEffort` → 调用 `agent.CreateCheckpoint`
- `agent.AgentClient.CreateCheckpoint` → 调用 `worldcheckpoint.Worker.Create`
- `checkpoint.Service.Restore` → 调用 `agent.RestoreCheckpoint`

---

### 11. **Secrets Management 模块**

**问题**：只有明文 Bearer token。

**设计方案（MVP）**：

```
internal/secrets/
  ├── manager.go            # Manager interface
  ├── env.go                # EnvManager (read from env vars)
  └── file.go               # FileManager (read from .secrets.json)

type Manager interface {
  Get(ctx, key string) (string, error)
  Set(ctx, key, value string) error
  Delete(ctx, key string) error
}

type EnvManager struct{}

func (EnvManager) Get(ctx, key string) (string, error) {
  val := os.Getenv(key)
  if val == "" {
    return "", errors.New("secret not found")
  }
  return val, nil
}
```

**接口契约**：
- CLI/Controller 通过 `secrets.Manager.Get("AGENT_TOKEN")` 获取 token
- 不在代码中硬编码或传递明文
- 未来可扩展到 Vault / AWS Secrets Manager

---

## 模块化原则总结

所有设计遵循以下原则：

1. **接口优先** - 未实现模块先定义接口（Bridge, Container, Secrets）
2. **Noop 实现** - 提供无操作实现，保证现有代码不破坏（NoopBridge, NoopRuntime）
3. **依赖注入** - 通过服务构造函数注入依赖（Service.New(..., bridge Bridge, container Runtime)）
4. **边界清晰** - Domain (policy) vs Agent (execution) vs Integration (external systems)
5. **渐进式迁移** - 保留现有实现，新功能通过新模块增量添加
6. **测试优先** - 每个模块都有对应 `_test.go`

这个方案确保了：
- 现有代码继续工作
- 新功能可独立开发和测试
- 模块间通过接口解耦
- 未来扩展不破坏现有结构