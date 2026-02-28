# Configuration and Data Storage

Mush stores data across three root locations following the XDG Base Directory Specification: a **config root** for configuration and credentials, a **state root** for logs, transcript history, and update-check state, and a **cache root** for downloaded bundle assets. Path resolution checks `XDG_CONFIG_HOME` / `XDG_STATE_HOME` / `XDG_CACHE_HOME` first on all platforms, then falls back to OS-specific defaults (config/cache) or `$HOME/.local/state` (state). During bundle install operations, Mush also writes project-level files into the current working directory.

## Directory Layout

### Config Root

`~/.config/mush/` (Linux default; `$XDG_CONFIG_HOME/mush` when set)

- `config.yaml` — user configuration
- `api-key` — API key file fallback (when OS keyring is unavailable)

### State Root

`~/.local/state/mush/` (Linux default; `$XDG_STATE_HOME/mush` when set)

- `logs/`
  - `mush.log` — structured log file
  - `mush.log.1` through `mush.log.5` — rotated backups
- `history/` — transcript history
  - `{session-id}/`
    - `events.live.jsonl` — live event stream (flushed per-event; removed after close)
    - `events.jsonl.gz` — compressed event archive (created on close)
    - `meta.json` — session metadata
- `update-check.json` — cached update state

### Cache Root

`~/.cache/mush/` (Linux default; `$XDG_CACHE_HOME/mush` when set)

- `bundles/`
  - `{workspace-id}/{slug}/{version}/`
    - `manifest.json` — resolved bundle manifest
    - `assets/` — downloaded bundle files

### Project-Level

- `{project}/.mush/` — project-level tracking
  - `installed.json` — installed bundle registry

### Path Resolution

Path resolution follows this order for each root:

1. **XDG env var** — `XDG_CONFIG_HOME` / `XDG_STATE_HOME` / `XDG_CACHE_HOME` if set to an absolute path
2. **OS-specific default** — `os.UserConfigDir()` / `os.UserCacheDir()` for config/cache; no OS API for state (skipped)
3. **Home-dir fallback** — `$HOME/.config` / `$HOME/.local/state` / `$HOME/.cache`

Relative XDG paths are ignored per the XDG Base Directory Specification. If none of these resolve (e.g., `$HOME` is unset in a container), Mush returns an error.

## Configuration File

Mush reads `config.yaml` from the config root. The file is created automatically by `mush config set` or `mush init`.

### Config Keys

| Key | Type | Default | Env Override | Description |
|-----|------|---------|-------------|-------------|
| `api.url` | string | `https://api.musher.dev` | `MUSH_API_URL` | Musher platform API endpoint |
| `worker.poll_interval` | duration | `30s` | `MUSH_WORKER_POLL_INTERVAL` | Job poll interval (e.g. `30s`, `1m`) |
| `worker.heartbeat_interval` | duration | `30s` | `MUSH_WORKER_HEARTBEAT_INTERVAL` | Heartbeat interval (e.g. `30s`, `1m`) |
| `history.enabled` | bool | `true` | `MUSH_HISTORY_ENABLED` | Enable transcript history recording |
| `history.dir` | string | `<state root>/history` | `MUSH_HISTORY_DIR` | Transcript storage directory |
| `history.scrollback_lines` | int | `10000` | `MUSH_HISTORY_SCROLLBACK_LINES` | In-memory scrollback ring buffer size (lines) |
| `history.retention` | duration | `720h` (30 days) | `MUSH_HISTORY_RETENTION` | Retention period for `mush history prune` |

Environment variables use the `MUSH_` prefix with dots replaced by underscores (e.g., `api.url` becomes `MUSH_API_URL`). Environment variables take precedence over the config file.

### Precedence

Configuration is resolved in this order (highest priority first):

1. CLI flags (`--api-url`)
2. Environment variables (`MUSH_*`)
3. Config file (`config.yaml`)
4. Built-in defaults

`--api-url` is a global flag and applies to any `mush` command. It overrides
`MUSH_API_URL` and `api.url` for that command process.
`--api-key` is not global; it is available on `mush auth login --api-key`,
with `MUSH_API_KEY` preferred for non-interactive environments.

### CLI API URL override

```bash
# Use staging API for a single run
mush --api-url https://api.staging.musher.dev worker start --dry-run

# Point at a local dev API
mush --api-url http://localhost:8080 doctor
```

### Example

```yaml
api:
  url: https://api.musher.dev
worker:
  poll_interval: 30s
  heartbeat_interval: 30s
history:
  enabled: true
  scrollback_lines: 10000
  retention: 720h
```

## Credentials

Mush resolves the API key from the following sources in order:

1. **Environment variable** — `MUSH_API_KEY`
2. **OS Keyring** — stored under service `dev.musher.mush`, account `api-key`
3. **File fallback** — `<config root>/api-key`

### Keyring Backends

| Platform | Backend |
|----------|---------|
| macOS | Keychain (`security` framework) |
| Windows | Credential Manager |
| Linux | Secret Service (D-Bus, e.g., GNOME Keyring, KWallet) |

If the OS keyring is unavailable (headless servers, containers, CI), `mush auth login` automatically falls back to file storage.

### File Fallback

The credentials file stores the API key as a single line of plaintext. It is created with `0o600` permissions (owner read/write only) inside a `0o700` directory. The key is written with a trailing newline; whitespace is trimmed on read.

**Security note:** The file fallback is intended for non-interactive environments where no keyring is available. On shared machines, prefer `MUSH_API_KEY` or ensure the config directory has restrictive permissions.

## Logs

Mush writes structured logs to `<state root>/logs/mush.log` by default. When running interactive commands (`worker start`, `bundle load`), logs go to the file; in non-interactive / CI contexts, logs go to stderr.

### Rotation

The default log file is rotated when it exceeds **10 MB**. Up to **5 backups** are kept (`mush.log.1` through `mush.log.5`). The oldest backup is deleted when a new rotation occurs.

### Format

Two formats are supported:

- **`json`** (default) — structured JSON, one object per line
- **`text`** — human-readable `slog` text format

### Environment Overrides

| Variable | Default | Description |
|----------|---------|-------------|
| `MUSH_LOG_FILE` | `<state root>/logs/mush.log` | Log file path |
| `MUSH_LOG_LEVEL` | `info` | Log level: `error`, `warn`, `info`, `debug` |
| `MUSH_LOG_FORMAT` | `json` | Log format: `json`, `text` |
| `MUSH_LOG_STDERR` | `auto` | Stderr logging: `auto`, `on`, `off` |

With `--log-stderr auto` (the default), stderr logging is enabled for non-interactive commands and disabled for interactive ones (where the watch UI owns the terminal).

### Redaction

Log attributes with sensitive key names are automatically replaced with `[REDACTED]`. This includes keys containing: `token`, `api_key`, `apikey`, `secret`, `credential`, `password`, and the exact key `authorization`.

## Transcript History

Each job execution session creates a directory under `<state root>/history/{session-id}/` containing:

| File | Format | Description |
|------|--------|-------------|
| `events.live.jsonl` | plain JSONL | Live event stream (flushed per-event for tailing; removed after close) |
| `events.jsonl.gz` | gzip-compressed JSONL | Compressed event archive (created on close from live file) |
| `meta.json` | JSON | Session metadata (`sessionId`, `startedAt`, `closedAt`) |

During an active session, events are written only to `events.live.jsonl`. On close, the live file is compressed to `events.jsonl.gz` and the live file is removed. If a session crashes before close, `ReadEvents` falls back to reading the live file directly.

### Event Format

Each event in the JSONL files is a JSON object:

```json
{
  "sessionId": "a1b2c3d4-...",
  "seq": 1,
  "ts": "2026-01-15T10:30:00Z",
  "stream": "stdout",
  "rawBase64": "SGVsbG8gd29ybGQ=",
  "text": "Hello world"
}
```

### Retention

The default retention period is **30 days** (`720h`). Sessions older than the retention period are deleted by `mush history prune`. The in-memory ring buffer holds the most recent **10,000 lines** per session for the watch UI scroll-back.

### Permissions

Session directories are created with `0o700`. Event and metadata files are created with `0o600`.

### Security Considerations

Transcript history captures raw PTY output and may contain secrets such as API keys, tokens, passwords, or `.env` contents that appear in stdout/stderr during job execution. Unlike structured logs (which apply key-based redaction), transcripts record unfiltered output for full debugging context.

**Mitigations:**

- Session directories and event files use restrictive permissions (`0o700` / `0o600`)
- `mush history prune` deletes sessions older than the configured retention period (default: 30 days)
- Set `MUSH_HISTORY_ENABLED=false` or `history.enabled: false` in `config.yaml` to disable transcript recording entirely

In sensitive environments (shared machines, compliance-scoped workloads), consider disabling transcript history or reducing the retention window.

## Bundle Cache

Downloaded bundles are cached at `<cache root>/bundles/{workspace-id}/{slug}/{version}/`.

### Structure

```
~/.cache/mush/bundles/
  bf9a0291-.../
    my-bundle/
      1.0.0/
        manifest.json       # Resolved bundle metadata
        assets/
          skill.md          # Downloaded asset files
          agent.md
```

### Cache Hit Detection

A bundle version is considered cached if `manifest.json` exists in the version directory. On cache hit, Mush skips the download step entirely.

### Atomicity

Downloads use a staging directory (`{version}.partial.*`) alongside the final version directory. Assets are written into the staging directory, the manifest is written last, and the staging directory is atomically renamed to the final path. This ensures that:

- A crash mid-download never leaves a corrupt cache entry (no `manifest.json` = no cache hit)
- Concurrent downloads of the same version are safe (loser detects winner's completed cache)
- Stale staging directories from interrupted downloads are cleaned up on the next pull

### Safe to Delete

The bundle cache is safe to delete at any time. Bundles will be re-downloaded from the platform on the next `bundle load` or `bundle install`.

```bash
rm -rf ~/.cache/mush/bundles
```

### Permissions

The top-level cache root (`~/.cache/mush/`) is created with `0o700` to restrict access. Subdirectories within the cache are created with `0o755`. Asset files are written with `0o644`. The manifest is written with `0o644`.

**Note:** On Windows, POSIX permission bits are best-effort and may not be enforced by the filesystem.

## Update State

Mush caches the result of update checks in `<state root>/update-check.json` to avoid hitting the GitHub Releases API on every invocation.

### Format

```json
{
  "lastCheckedAt": "2026-01-15T10:30:00Z",
  "latestVersion": "3.1.0",
  "currentVersion": "3.0.0",
  "releaseURL": "https://github.com/musher-dev/mush/releases/tag/v3.1.0"
}
```

### Behavior

- Mush checks for updates at most once every **24 hours**.
- The state file is written atomically (temp file + rename) to prevent corruption from concurrent processes.
- If the file is missing or corrupted, Mush treats it as empty and performs a fresh check.
- Set `MUSH_UPDATE_DISABLED=1` to disable all update checks.

## Project-Level Files

`mush bundle install` writes files into the current project directory:

- **`.mush/installed.json`** — tracks installed bundles (slug, version, harness, asset paths)
- **Harness-specific assets** — installed to `.claude/skills/`, `.claude/agents/`, or other harness-specific directories depending on the bundle configuration

### installed.json Format

```json
[
  {
    "slug": "my-bundle",
    "version": "1.0.0",
    "harness": "claude",
    "assets": [
      ".claude/skills/skill.md",
      ".claude/agents/agent.md"
    ],
    "timestamp": "2026-01-15T10:30:00Z"
  }
]
```

The `assets` array lists paths relative to the project root. `mush bundle uninstall` uses this list to remove installed files.

## Environment Variables

Summary of all environment variables that affect Mush behavior:

| Variable | Purpose |
|----------|---------|
| `MUSH_API_KEY` | API key for authentication (highest priority credential source) |
| `MUSH_API_URL` | Platform API URL override |
| `MUSH_LOG_FILE` | Structured log file path |
| `MUSH_LOG_LEVEL` | Log level (`error`, `warn`, `info`, `debug`) |
| `MUSH_LOG_FORMAT` | Log format (`json`, `text`) |
| `MUSH_LOG_STDERR` | Stderr logging mode (`auto`, `on`, `off`) |
| `MUSH_HISTORY_DIR` | Transcript history directory override |
| `MUSH_HISTORY_ENABLED` | Enable/disable transcript history |
| `MUSH_HISTORY_SCROLLBACK_LINES` | In-memory scrollback ring buffer size |
| `MUSH_HISTORY_RETENTION` | History retention period (Go duration, e.g., `720h`) |
| `MUSH_WORKER_POLL_INTERVAL` | Job poll interval (Go duration, e.g., `30s`) |
| `MUSH_WORKER_HEARTBEAT_INTERVAL` | Heartbeat interval (Go duration, e.g., `30s`) |
| `MUSH_UPDATE_DISABLED` | Disable update checks (`1` or `true`) |
| `MUSH_JSON` | Enable JSON output (`1` or `true`) |
| `MUSH_QUIET` | Enable quiet mode (`1` or `true`) |
| `MUSH_NO_INPUT` | Disable interactive prompts (`1` or `true`) |
| `XDG_CONFIG_HOME` | Override config root (all platforms, checked first; must be absolute) |
| `XDG_STATE_HOME` | Override state root (all platforms, checked first; must be absolute) |
| `XDG_CACHE_HOME` | Override cache root (all platforms, checked first; must be absolute) |

## Platform Differences

| | Config Root | State Root | Cache Root | Keyring Backend |
|-|-------------|------------|------------|-----------------|
| **Linux** | `~/.config/mush` | `~/.local/state/mush` | `~/.cache/mush` | Secret Service (D-Bus) |
| **macOS** | `~/Library/Application Support/mush` | `~/.local/state/mush` | `~/Library/Caches/mush` | Keychain |
| **Windows** | `%AppData%\mush` | `~/.local/state/mush` | `%LocalAppData%\mush` | Credential Manager |

`XDG_CONFIG_HOME`, `XDG_STATE_HOME`, and `XDG_CACHE_HOME` are checked first on all platforms. When set, they override the OS-specific defaults shown above. The state root has no OS-specific default and always falls back to `$HOME/.local/state/mush`.

## Resetting Mush

**Clear everything** (config, credentials, state, cache):

```bash
rm -rf ~/.config/mush ~/.local/state/mush ~/.cache/mush
```

**Clear only the bundle cache** (re-downloaded on next use):

```bash
rm -rf ~/.cache/mush/bundles
```

**Clear only credentials:**

```bash
mush auth logout
```

Or manually:

```bash
rm ~/.config/mush/api-key
```

Note: `mush auth logout` also clears the OS keyring entry. The manual `rm` command only removes the file fallback.
