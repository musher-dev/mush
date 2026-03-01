# Mush CLI

## What This Is

Mush is a **consumer-side agent runtime** — the local worker that connects developer machines to the Musher job queue. It claims jobs, executes them locally via Claude Code or Bash harnesses, and reports results back to the platform.

**IS**: Agent runtime, job consumer, bundle installer, local executor.
**IS NOT**: Platform management CLI. Mush does not create workspaces, manage routes, configure queues, or publish bundles — that is the Musher web UI and API.

Think `docker run`, not `docker hub` — Mush consumes and executes, it does not publish or administer.

## Directory Overview

Organized into three architectural layers (see `@docs/architecture/boundaries.md` for dependency rules):

### CLI Entry — `cmd/mush/`

Command wiring, flags, user interaction orchestration, exit semantics.

- `main.go` — Root command setup, global flags, `CLIError` rendering
- `worker_unix.go` / `worker_other.go` / `worker_common.go` — Worker command (start/status/stop)
- `auth.go` — Auth login/status/logout
- `config.go` — Config list/get/set
- `bundle.go` — Bundle load/install/list/info/uninstall
- `habitat.go` — Habitat list
- `history.go` — Transcript history list/view/prune
- `init.go` — Onboarding wizard
- `update.go` — Self-update
- `doctor.go` — Diagnostic checks
- `completion.go` — Shell completions

### Feature / Orchestration — `internal/`

Workflow orchestration, user-facing interaction behavior, diagnostics.

- `harness/` — Watch UI (PTY + scroll region), Claude and Bash execution harnesses
- `wizard/` — Init wizard flow
- `doctor/` — Diagnostic check framework (output-agnostic — must not import `output`)
- `prompt/` — Interactive user prompts
- `output/` — CLI output handling (colors, spinners, TTY detection)
- `bundle/` — Bundle caching, installation, asset mapping

### Platform / Core — `internal/`

API transport, credential/config state, platform operations, shared primitives.

- `client/` — HTTP client for Musher API
- `auth/` — Credential storage (keyring + file fallback)
- `config/` — Viper-based configuration management
- `worker/` — Worker registration, heartbeat, and deregistration
- `errors/` — `CLIError` type (message + hint + exit code)
- `update/` — Self-update from GitHub Releases
- `buildinfo/` — Build metadata (version, commit, date)
- `terminal/` — TTY detection and capabilities
- `paths/` — XDG-style path resolution
- `ansi/` — ANSI escape sequence utilities
- `observability/` — Structured logging setup
- `transcript/` — Job transcript storage and retrieval
- `testutil/` — Test helpers (not imported by production code)

## Stable Code Patterns

**Output via context** — All user-facing output goes through `output.FromContext(cmd.Context())`. Never use `fmt.Print*` directly in commands.

**Command factory** — Every command is a `newXxxCmd() *cobra.Command` function returning a configured command. Flags are declared as local variables in the factory closure.

**Error handling** — Use `CLIError` from `internal/errors` for user-facing errors (message + hint + exit code). Wrap lower-level errors with `fmt.Errorf("context: %w", err)`. Never panic.

**Context injection** — The command context carries the output writer and logger. Pass it through to internal packages; don't create global state.

**Architecture boundaries** — Enforced by `depguard` linter rules. Platform/Core packages must not import `output` or `prompt`. The `doctor` package must not import `output`. No `internal/` package may import `cmd/mush`.

**Noun-verb commands** — Commands follow resource-first structure (`mush <resource> <verb>`). Discovery via `mush <command> --help`.

## Development

See `@CONTRIBUTING.md` for setup, workflow, commit conventions (Conventional Commits), and code style.
See `@Taskfile.yml` for all available build/test/lint targets.

Essential commands:

```bash
task check        # All quality checks (fmt + lint + vuln + test)
task build        # Build mush binary
task check:test   # Run tests only
task fmt          # Format code
```

## Quick Reference

- **Binary**: `mush`
- **Config dir**: `~/.config/mush/` (XDG)
- **State dir**: `~/.local/state/mush/` (XDG)
- **Credentials**: OS Keyring (`dev.musher.mush`), falls back to `~/.config/mush/api-key`
- **Logs**: `~/.local/state/mush/logs/mush.log` (default sink)
- **History**: `~/.local/state/mush/history/` (transcript sessions)
- **Update state**: `~/.local/state/mush/update-check.json`
- **Bundle cache**: `~/.cache/mush/bundles/{workspace_id}/{slug}/{version}/`
- **API endpoint**: `api.url` config key or `MUSH_API_URL` env var
- **Auth**: `MUSH_API_KEY` env var or `mush auth login`
