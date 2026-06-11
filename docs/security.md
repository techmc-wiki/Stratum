# Security

StratumMC manages arbitrary Minecraft server code and world data. The initial
skeleton records the required boundaries; production hardening remains future
work.

## Invariants

1. Base worlds are immutable and mounted or copied read-only.
2. Writable worlds belong to exactly one session workspace.
3. Shared-session changes require elevated project permissions.
4. Forks are temporary, resource-limited, and isolated from their source room.
5. Dangerous operations require a successful pre-operation checkpoint.
6. Every uploaded artifact receives SHA-256, size, uploader, compatibility, and
   approval metadata.
7. Unapproved artifacts cannot attach to shared sessions.
8. Review sessions isolate untrusted artifacts from base worlds, unrelated
   projects, controller files, checkpoint storage, and host secrets.
9. Lucy cannot start, stop, or command a JVM.
10. MCDR and process supervision are accessible only through explicit runtime
    interfaces.
11. Secrets must come from deployment configuration and must never be committed.
12. User-provided URL mixin source compilation is prohibited.

## Uploaded JAR threat model

JAR files are arbitrary code execution. Hashing is identification, not safety.
Approval must be explicit and attributable. A future review worker must use
OS-level isolation, narrow filesystem mounts, resource limits, restricted
network access, and disposable credentials. A mock approval or sandbox must be
named as such and never represented as a security boundary.

## Checkpoint and rollback safety

Checkpoint storage must be append-oriented and separated from live session
workspaces. Rollback must verify project/session ownership, freeze or stop the
target session, create a pre-operation checkpoint, and restore into only that
session's writable world. Base-world objects must never be overwritten.
