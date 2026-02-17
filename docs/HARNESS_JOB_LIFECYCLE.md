# Mush Harness Job Lifecycle

## Overview

This document describes how the watch harness claims jobs, executes them, captures output, and reports results back to the platform.

The harness is intentionally **watch-only**: it requires a real terminal (TTY) and renders live output in a scroll region with a status bar pinned to the top.

See `internal/harness/model.go` for the source of truth.

## Surfaces

Only one surface exists:
- **Watch (harness)**: raw-terminal UI + ANSI scroll region + (optionally) a Claude PTY session.

There is no headless/daemon mode.

## Operator Controls and Shutdown

In watch mode, terminal input is read in raw mode and handled locally by the harness:

- `Ctrl+C` with an active Claude job:
  1. First press sends an interrupt to Claude.
  2. Second press within 2 seconds exits the harness.
- `Ctrl+C` when no Claude job is active: exits immediately.
- `Ctrl+Q`: exits immediately.
- `Ctrl+S`: toggles copy mode (Esc returns to live mode).

Shutdown is hardened with a bounded lifecycle:

1. Command context cancellation propagates into harness shutdown.
2. Claude PTY process group gets `SIGTERM` first (graceful attempt).
3. If still running after a short deadline, `SIGKILL` is sent.
4. Terminal state is restored and link deregistration is attempted before exit.

## PTY Startup Notes and Troubleshooting

Claude PTY startup uses `pty.StartWithSize`, which sets `setsid` and a controlling TTY.
The harness does **not** set `setpgid` explicitly at startup, because that can conflict with
session-leader rules and fail with `EPERM` before Claude executes.

If startup fails with `fork/exec ... operation not permitted`, check:

1. Claude path resolution:
   - `which claude`
2. Executable permissions:
   - `ls -l "$(which claude)"`
3. Filesystem mount options (Linux):
   - `findmnt -no OPTIONS "$(dirname "$(which claude)")"` and verify it is not `noexec`
4. Quarantine attributes (macOS):
   - `xattr -l "$(which claude)"`

## Job Loop (High-Level)

1. Validate we are in a TTY, enter raw mode, set up scroll region
2. Register a link with the platform and start link heartbeat
3. Poll `ClaimJob(...)` in a loop
4. For each job:
  - validate supported harness type (mapped from `execution.agentType` in the API contract)
   - call `StartJob(...)`
   - run the job
   - call `CompleteJob(...)` or `FailJob(...)`

## Claude Jobs (Interactive PTY)

Claude jobs run through an interactive `claude` process launched in a PTY:

1. Start `claude` in a PTY (once per harness run)
2. Install a Claude Stop hook that writes a completion marker file into a per-run temp dir
3. Inject the prompt into the PTY and press Enter
4. Capture PTY output while the job runs
5. Detect completion by polling for the completion marker file
6. Report output to the platform and send `/clear` to reset Claude for the next job

This approach prioritizes faithful rendering and operator visibility.

## Bash Jobs (Subprocess)

Bash jobs execute as `bash -c <script>`:

1. Extract the command from:
   - `execution.renderedInstruction` (preferred), else
   - `inputData.command`, else
   - `inputData.script`
2. Run with timeout derived from `execution.timeoutMs` (or the harness default)
3. Stream stdout/stderr into the scroll region while capturing them
4. Report results back to the platform (`output`, `stdout`, `stderr`, `exitCode`, `durationMs`)
