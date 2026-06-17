# Lucy Integration

StratumMC 将 Lucy 作为子模块嵌入在 `tools/lucy/`，可以直接一起编译。

## 构建

```bash
# 构建所有组件（包括 Lucy）
task build

# 或单独构建 Lucy
go build -o lucy.exe ./cmd/lucy

# Linux 构建
task build:linux-amd64
```

编译后的二进制：
- `dist/local/lucy` (或 `dist/local/lucy.exe` on Windows)
- `dist/linux-amd64/lucy`

## 核心概念

### Manifest (lucy.yaml)

Manifest 表达用户意图：

- 游戏版本 (1.12, 1.17, latest)
- 平台 (fabric, forge, neoforge)
- 所需的模组和插件
- MCDR 配置

### Lock (lucy-lock.yaml)

Lock 记录精确的已解析状态：

- 每个包的确切版本
- 下载 URL 和文件哈希
- 依赖关系图
- 安装路径

Checkpoint 元数据会存储 Lock 文件的 SHA-256 哈希以确保可重现性。

### Artifact 分析

Lucy 可以从上传的 JAR/ZIP/MCDR 插件文件中提取元数据：

- 模组 ID 和版本
- 依赖关系
- 兼容性信息

## API 使用

### 直接调用（推荐）

Lucy 的核心功能可以通过 Go API 直接调用，无需 CLI 子进程：

```go
import (
	lucystate "github.com/mclucy/lucy/state"
	"github.com/stratummc/stratum/internal/integration/lucy"
)

// 创建 manifest
manifestSvc := lucy.NewManifestService(sessionDir)
manifest := lucy.CreateDefault("1.17.1", "fabric", "0.14.0", true)
manifest.Packages = []lucystate.ManifestPackage{
	{
		ID:      "fabric/carpet",
		Version: "1.4.83",
		Role:    lucystate.RoleRequired,
		Side:    lucystate.SideServer,
	},
}
manifestSvc.Write(ctx, manifest)

// 安装包（直接调用）
installSvc := lucy.NewInstallService(sessionDir)
installSvc.Install(ctx, lucy.PackageRequest{
	Platform: "fabric",
	Name:     "carpet",
	Scope:    "modrinth",
	Version:  "1.4.83",
})

// 批量安装
installSvc.InstallMany(ctx, []lucy.PackageRequest{
	{Platform: "fabric", Name: "carpet", Scope: "modrinth", Version: "1.4.83"},
	{Platform: "fabric", Name: "lithium", Scope: "modrinth", Version: "latest"},
})

// 探测服务器环境
probeSvc := lucy.NewProbeService(sessionDir)
info, _ := probeSvc.ServerInfo()
fmt.Printf("Game version: %s\n", info["game_version"])

// 读取 lock 并计算哈希用于 checkpoint
lockSvc := lucy.NewLockService(sessionDir)
lock, _ := lockSvc.Read(ctx)
lockHash, _ := lucy.Hash(lock)

// 分析上传的 artifact
artifactSvc := lucy.NewArtifactService()
infos, _ := artifactSvc.Analyze(ctx, "/path/to/uploaded.jar")
for _, info := range infos {
	fmt.Printf("Found: %s/%s@%s\n", info.Platform, info.Name, info.Version)
}
```

### CLI 方式（仅在必要时使用）

对于复杂的交互式操作，可以调用编译好的 Lucy CLI：

```bash
dist/local/lucy init
dist/local/lucy add fabric/carpet
dist/local/lucy install
```

## 架构边界

Lucy 严格非侵入：

- ✅ 管理依赖清单和锁文件
- ✅ 提取 artifact 元数据
- ✅ 依赖解析（通过 Lucy CLI）
- ❌ **不**管理 JVM 进程
- ❌ **不**控制服务器运行时
- ❌ **不**替代 MCDR 或 Agent 监督

进程生命周期由 `internal/agent/process` 管理。  
MCDR 集成在 `internal/integration/mcdr`。
