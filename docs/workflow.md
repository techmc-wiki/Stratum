# Development Workflow

Keep each task focused on one reviewable goal. Separate implementation,
architecture alignment, broad refactoring, and schema migration unless the task
explicitly requires them together.

## Verify changes

```bash
gofmt -w <changed-go-files>
go test -count=1 ./...
git diff --check
```

Prefer focused package tests while developing, then run the full suite before
finishing.

## Smoke tests

Use a task-specific disposable data directory so existing metadata is not
modified:

```powershell
$data = ".stratum/smoke-<task-name>"
Remove-Item -Recurse -Force $data -ErrorAction SilentlyContinue
```

Keep smoke-test binaries and runtime files under `.stratum/` and stop any
background Agent process when the test completes.

## Review and commits

Review `git status`, `git diff --check`, and the diff grouped by logical change.
Confirm unrelated files were not reformatted or rewritten. Documentation-only
tasks should not change behavior; implementation tasks should update only the
specific docs made inaccurate by that implementation.

When commits are allowed, create small commits that compile and pass relevant
tests. Recommended prefixes are `docs:`, `domain:`, `storage:`, `lifecycle:`,
`agent:`, `runtime:`, `cli:`, `test:`, `refactor:`, and `chore:`. When commits
are not created, report the same grouping as proposed atomic commits.

When reviewing Codex output, check the stated file groups, behavior changes,
verification results, remaining TODOs, and whether the suggested next task is
narrow enough to review independently.
