# Mush Harness Job Lifecycle

## Overview

This document describes how the watch harness claims jobs, executes them, captures output, and reports results back to the platform.

The harness is intentionally **watch-only**: it requires a real terminal (TTY) and renders live output in a scroll region with a status bar pinned to the top.

See `internal/harness/model.go` for the source of truth.

## Surfaces

Only one surface exists:
- **Watch (harness)**: raw-terminal UI + ANSI scroll region + (optionally) a Claude PTY session.

There is no headless/daemon mode.

## Job Loop (High-Level)

1. Validate we are in a TTY, enter raw mode, set up scroll region
2. Register a link with the platform and start link heartbeat
3. Poll `ClaimJob(...)` in a loop
4. For each job:
   - validate supported agent type
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
