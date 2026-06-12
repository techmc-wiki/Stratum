Implement the next atomic task:

runtime: load trusted RuntimeProfiles from local configuration

Context:
- Atomic workflow policy has been added to AGENTS.md and docs/workflow.md.
- Managed Terminal Executor already exists.
- RuntimeProfile model, validation, and built-in dummy-process profile already exist.
- Current registry only has built-in/default profiles.
- We now want trusted local RuntimeProfile config loading.
- Keep this task atomic.

Goals:
1. Add trusted local RuntimeProfile config loading.
2. Do not implement reconciliation.
3. Do not implement MCDR runtime yet.
4. Do not implement Minecraft launching.
5. Do not implement Lucy integration.
6. Do not modify unrelated docs or architecture rules.
7. Do not change lifecycle semantics except what is needed to select loaded profiles.

Required behavior:
- Add support for loading RuntimeProfiles from a trusted local config file.
- The config path should be provided to stratum-agent, not normal user lifecycle commands.
- Keep built-in `dummy-process` always available.
- Loaded profiles must pass existing RuntimeProfile validation.
- Disabled profiles should not be returned as enabled/usable.
- Invalid profiles should cause clear startup/config errors.
- Profile IDs must be unique.
- If a local profile ID conflicts with a built-in profile, reject it or clearly define precedence. Prefer rejecting conflicts.
- Do not expose command argv/env/working_dir in normal public profile listing if current design intentionally hides sensitive profile details.

Suggested config format:
Use JSON unless the repo already has a preferred config format.

Example conceptual file:

{
  "runtime_profiles": [
    {
      "id": "terminal-test",
      "name": "Trusted Terminal Test",
      "runtime_type": "terminal",
      "command_argv": ["...trusted argv..."],
      "working_dir": ".stratum/runtime-root/terminal-test",
      "env": {
        "STRATUM_PROFILE": "terminal-test"
      },
      "stop_strategy": "terminate",
      "graceful_stop_timeout": "5s",
      "force_kill_timeout": "2s",
      "log_mode": "combined",
      "enabled": true,
      "notes": "Trusted local test profile"
    }
  ]
}

Implementation details:
- Add a small loader function/package if appropriate.
- Keep validation separate from loading.
- Keep registry behavior simple and testable.
- The Agent server should accept a flag such as:
  - `--runtime-profiles <path>`
- If the flag is omitted, use built-in profiles only.
- If the flag is provided and invalid, agent startup should fail clearly.
- Keep command execution restricted to trusted profiles only.

CLI / HTTP:
- Update `stratum-agent serve` to accept `--runtime-profiles <path>`.
- Existing `agents runtime-profiles --id local` should show loaded enabled profiles.
- Do not add broad profile management commands.
- Do not allow normal users to create/edit runtime profiles through CLI.

Tests:
Add focused tests for:
- loading valid profile config.
- invalid JSON/config returns clear error.
- invalid RuntimeProfile fails loading.
- duplicate profile ID fails.
- conflict with built-in dummy-process fails.
- disabled profile is loaded but not usable, or is excluded, according to your chosen registry behavior.
- agent runtime-profiles endpoint lists built-in + loaded profiles.
- starting a session with a loaded enabled profile works if safe test setup exists.
- starting with disabled/unknown profile fails without changing Controller Session state.
- existing dummy-process tests still pass.

Keep tests cross-platform.
Do not depend on Unix-only commands.
If a terminal execution test is needed, use the existing Go-native test helper approach.

Documentation:
Update only the narrowly relevant docs:
- docs/runtime.md
- docs/agent.md if necessary
- README only if a short dev example is useful

Do not rewrite architecture docs broadly.

Documentation should explain:
- RuntimeProfile config is trusted local agent config.
- It is not user-submitted command execution.
- Built-in dummy-process remains default.
- MCDR/minecraft profiles remain future work unless explicitly configured by trusted operators.
- Shell execution remains forbidden.

Verification:
- Run gofmt.
- Run go test -count=1 ./...
- Run git diff --check.

Manual smoke test:
1. Create a small trusted runtime profile config under a temp/dev path.
2. Start agent with:
   `go run ./cmd/stratum-agent serve --listen 127.0.0.1:8787 --runtime-profiles <path>`
3. Confirm:
   `go run ./cmd/stratum --agent-url http://127.0.0.1:8787 agents runtime-profiles --id local`
   shows the loaded profile.
4. Confirm dummy-process still works.

Final response format:
Follow AGENTS.md Atomic Change / Commit Policy.

Report:
1. Verification
2. Atomic commit summary
3. Behavior changes
4. Remaining TODOs
5. Suggested next atomic task