# MCDR RuntimeProfile Executor — 实现架构（已完成）

---

## 状态：✅ 已完成

MCDR RuntimeProfile Executor v0 已完整实现并测试通过。

---

## 实际架构

```
internal/agent/mcdr/
  └── supervisor.go              # MCDR 生命周期管理（Start/Stop/Restart/SendCommand）
                                   ↓ 调用
internal/agent/process/
  └── process.go                  # process.Supervisor（StartProcess -> startTerminal）
      ├── startTerminal()         # exec.Command 启动真实 OS 进程
      ├── waitTerminal()          # 监控进程退出，更新状态（Stopped/Exited/Crashed）
      ├── StopProcess()           # 优雅停止（stdin command → signal → force kill）
      ├── logBuffer              # 行级循环缓冲区（maxBytes 限制）
      ├── streamWriter           # 实时日志写入器
      └── WaitForLog()           # 就绪检测（轮询日志模式匹配）

internal/agent/local/
  └── process_agent.go            # StartSession → a.mcdr.Start()
                                   # StopSession  → a.mcdr.Stop()
                                   # RestartSession → a.mcdr.Restart()

runtime-profiles/
  └── mcdr-fabric-1.17.json       # RuntimeProfile 配置
      ├── command_argv: ["mcdreforged", "start"]
      ├── stop_strategy: stdin
      ├── stop_stdin_command: "!!MCDR stop"
      └── readiness_check: log-pattern "Done ("
```

---

## 核心数据流

### 启动流程

```
session.Service.Start()
  → agent.StartSession(request{RuntimeProfileID: "mcdr-fabric-1.17"})
    → ProcessAgent.StartSession()
      → profiles.Get("mcdr-fabric-1.17")  // 加载 RuntimeProfile JSON
      → a.mcdr.Start(ctx, sessionID, profile)
        → process.Supervisor.StartProcess(ctx, sessionID, actualProfile)
          → startTerminal(sessionID, item, workDir)
            → cmd := exec.Command(argv[0], argv[1:]...)
            → cmd.Dir = workDir            // e.g. sessions/<id>/work/mcdr/config
            → cmd.Stdout = streamWriter    // 捕获日志
            → cmd.Stderr = streamWriter
            → cmd.Start()                   // 真实 OS 进程启动
            → go waitTerminal()            // 后台监控退出
```

### 停止流程

```
session.Service.Stop()
  → agent.StopSession()
    → ProcessAgent.StopSession()
      → a.mcdr.Stop(ctx, sessionID)
        → process.Supervisor.StopProcess()
          1. StopStrategy=stdin: io.WriteString(stdin, "!!MCDR stop\n")
          2. 等待 GracefulStopTimeout (60s)
          3. 超时? → cmd.Process.Kill()
          4. 等待 ForceKillTimeout (15s)
```

### 崩溃检测

```
waitTerminal goroutine:
  → cmd.Wait() 返回
  → 检查 stopRequested 标志
    - stopRequested=true  → StatusStopped
    - exitCode=0          → StatusExited
    - exitCode≠0          → StatusCrashed
```

---

## 测试用 Helper Process 模式

测试使用 Go 的 `os.Executable()` 自引用模式模拟 MCDR 行为：

```go
func mcdrTestProfile(t *testing.T, mode string) runtimeprofile.Profile {
    executable, _ := os.Executable()
    return runtimeprofile.Profile{
        ID: "mcdr-test-" + mode,
        RuntimeType: runtimeprofile.TypeMCDRPython,
        CommandArgv: []string{executable, "-test.run=TestMCDRHelperProcess", "--"},
        StopStrategy: runtimeprofile.StopStdin,
        StopStdinCommand: "stop",
        ...
    }
}

func TestMCDRHelperProcess(t *testing.T) {
    if os.Getenv("STRATUM_MCDR_HELPER") != "1" { return }
    switch os.Getenv("STRATUM_MCDR_MODE") {
    case "stdin":
        os.Stdout.WriteString("helper-ready\n")
        // 读取 stdin，等待 "stop" 命令后退出
        ...
    }
}
```

**测试覆盖：**
- `TestMCDRSupervisorStartStop` — 完整启动/停止生命周期
- `TestMCDRSupervisorRestart` — 重启产生新 PID
- `TestMCDRSupervisorSendCommand` — stdin 命令注入 + 停止后拒绝命令
- `TestMCDRSupervisorRejectsNonMCDRProfile` — 类型验证
- `TestMCDRSupervisorInspectNotStarted` — 未启动状态查询
- `TestMCDRSupervisorStopIdle` — 空闲会话管理
- `TestMCDRSupervisorCrashedProcess` — 非零退出码检测
- `TestMCDRSupervisorForceKill` — ForceKillTimeout 触发

---

## 关键设计决策

1. **无单独 `processExecutor` 抽象层** — 直接使用 Go 标准库 `os/exec.Cmd`，比额外接口层更简单、更可测试
2. **MCDR 配置文件由 `mcdr.Supervisor.Start` 生成** — config.yml 的 `start_command` 由物化清单数据派生
3. **所有状态操作加锁** — `sync.RWMutex` 保护 `Supervisor.processes` map
4. **日志限制** — `logBuffer` 通过循环丢弃旧行控制内存使用
5. **Graceful → Force 两级停止** — 先尝试 stdin 命令，超时后 force kill

---

## 未来扩展

- **健康检查** — 定期检测进程是否存活（HealthCheck proc-alive）
- **文件日志轮转** — 输出到 `logs/<session-id>.log` 文件
- **进程资源监控** — CPU/内存使用率
- **容器化执行** — 可注入 `executor` 抽象层支持 Docker/Podman

---

## 下一步

当前重点是 **Phase 3: World Management** — 世界检查点备份和恢复（`internal/agent/worldcheckpoint/`）。

