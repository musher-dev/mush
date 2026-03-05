# Adding a New Harness

This guide walks through adding a new execution harness to Mush. A harness connects Mush's job execution system to an external tool (like Claude Code or Codex CLI).

## Architecture overview

The harness system has two layers that must stay in sync:

1. **YAML provider spec** (`internal/harness/providers/{name}.yaml`) — Declarative metadata: binary name, CLI flags, asset paths, MCP config format, and health-check info. Loaded at package init via `//go:embed`.

2. **Go executor** (`internal/harness/{name}_executor.go`) — The runtime implementation. Registers itself in `init()` via `harness.Register()`, wiring the provider spec to a concrete `Executor` implementation.

The provider spec drives health checks (`mush doctor`), availability detection, and asset mapping. The executor drives job execution. Both are required — a YAML without a matching `Register()` call will appear in health checks but cannot execute jobs, and a `Register()` without matching YAML will panic at startup.

## File checklist

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/harness/providers/{name}.yaml` | Provider spec (metadata, flags, health) |
| Create | `internal/harness/{name}_executor.go` | Executor implementation |
| Create | `internal/harness/{name}_executor_test.go` | Tests |

No other files need modification — the `//go:embed providers/*.yaml` directive and `init()` registration handle discovery automatically.

## Step 1: Create the YAML provider spec

Create `internal/harness/providers/{name}.yaml`. Here is an annotated template with every field:

```yaml
# Required. Must be unique across all providers. This is the harness type
# identifier used in Register(), Lookup(), and job execution configs.
name: myharness

# Required for display. Shown in `mush doctor` output and UI.
displayName: My Harness

# Short description of the tool.
description: My AI coding assistant

# The CLI binary name. Used by AvailableFunc() to check if the tool is
# installed via exec.LookPath(). If empty, the harness is always considered
# available — useful for built-in executors that don't depend on an
# external binary.
binary: myharness

# Optional. Harness-specific config directory paths.
directories:
  project: .myharness        # Project-level config directory
  user: ~/.myharness          # User-level config directory

# Optional. How bundle directories are passed to the harness CLI.
bundleDir:
  mode: add_dir               # One of: "add_dir", "cd_flag", "cwd"
  flag: "--add-dir"            # CLI flag (required for add_dir and cd_flag modes)

# Optional. CLI flag names for MCP config injection.
cli:
  mcpConfig: "--mcp-config"   # Flag to pass the ephemeral MCP config file path

# Optional. Where bundle assets are mapped in the harness's native structure.
assets:
  skillDir: .myharness/skills
  agentDir: .myharness/agents
  toolConfigFile: .myharness/config.json

# Optional. MCP configuration support.
mcp:
  format: json                 # "json" or "toml" (see gotchas below)
  configPath: .myharness/mcp.json

# Optional. Health-check and install metadata for `mush doctor`.
status:
  versionArgs: ["--version"]   # Args passed to binary to get version string
  installHint: "npm install -g myharness"  # Shown when binary not found
  installCommand: ["npm", "install", "-g", "myharness"]  # Programmatic install
  configDir: "~/.myharness"    # Checked for existence
  authCheck:                   # Optional file-based credential check
    path: "~/.myharness/credentials.json"
    description: "Credentials"
```

**Validation rules** (enforced at startup, panic on violation):

- `name` is required and must be unique across all provider YAML files
- `bundleDir.mode` must be one of `add_dir`, `cd_flag`, `cwd` (if specified)
- `mcp.format` must be `json` or `toml` (if specified)

## Step 2: Create the executor

Create `internal/harness/{name}_executor.go`. The file must have a `//go:build unix` constraint (see [gotchas](#gotchas)).

```go
//go:build unix

package harness

import (
	"context"
	"fmt"

	"github.com/musher-dev/mush/internal/client"
)

// MyHarnessExecutor runs jobs via MyHarness.
type MyHarnessExecutor struct {
	opts SetupOptions
}

func init() {
	Register(Info{
		Name:      "myharness",                                   // Must match YAML name
		Available: AvailableFunc("myharness"),                    // Must match YAML name
		New:       func() Executor { return &MyHarnessExecutor{} },

		// Include MCPSpec only if your harness supports MCP.
		// Set BuildConfig to BuildJSONMCPConfig or BuildTOMLMCPConfig
		// depending on your provider's mcp.format.
		MCPSpec: &MCPSpec{
			Def:         mustGetProvider("myharness").MCP,
			BuildConfig: BuildJSONMCPConfig,
		},
	})
}

func (e *MyHarnessExecutor) Setup(ctx context.Context, opts *SetupOptions) error {
	e.opts = *opts

	// Initialize your harness (start process, verify binary, etc.)

	if opts.OnReady != nil {
		opts.OnReady()
	}
	return nil
}

func (e *MyHarnessExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	prompt, err := getPromptFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	// Run the job using `prompt` as the instruction.
	// Use job.Execution for working directory, environment, etc.
	_ = prompt

	return &ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     "result text",
			"durationMs": 0,
		},
	}, nil
}

func (e *MyHarnessExecutor) Reset(_ context.Context) error {
	// Prepare for the next job. No-op for one-shot executors.
	return nil
}

func (e *MyHarnessExecutor) Teardown() {
	// Release all resources (close PTY, kill processes, remove temp files).
}

// Compile-time interface checks.
var _ Executor = (*MyHarnessExecutor)(nil)
```

Key details:

- The `init()` name **must** match the YAML `name` field exactly. `mustGetProvider()` will panic if they don't match.
- Use `getPromptFromJob(job)` to extract the rendered instruction from a job — it handles nil checks and server-side render errors.
- Always return `*ExecError` (not plain errors) from `Execute` so the worker can classify failures and decide on retries.

## Step 3: Implement optional interfaces

The worker and TUI check for these via type assertions. Implement only what your harness needs.

| Interface | Methods | When to implement |
|-----------|---------|-------------------|
| `Resizable` | `Resize(rows, cols int)` | Harness runs in a PTY that should respond to terminal resize |
| `InputReceiver` | `WriteInput(p []byte) (int, error)` | Harness has an interactive session that accepts stdin (bundle load mode, TUI passthrough) |
| `Refreshable` | `NeedsRefresh(cfg) bool`, `ApplyRefresh(ctx, cfg) error` | Harness should hot-reload when MCP config changes at runtime |
| `SignalDirConsumer` | `SetSignalDir(dir string)` | Harness uses signal files for completion detection (persistent PTY pattern) |
| `TranscriptSource` | `WantsTranscript() bool` | Harness output should be saved to job transcript history |
| `InterruptHandler` | `Interrupt() error` | Harness should forward Ctrl+C to its underlying process |

**Reference**: `ClaudeExecutor` implements all 6 optional interfaces. `CodexExecutor` implements only `InputReceiver`.

## Step 4: Write tests

Follow the patterns in existing test files (`claude_executor_test.go`, `codex_executor_test.go`). Key testing patterns:

- Test `init()` registration: verify `Lookup("myharness")` returns your Info
- Test `Setup` with a mock binary or test fixture
- Test `Execute` with constructed `client.Job` values
- Test error paths: missing prompt, context cancellation, process failures
- Use compile-time interface checks: `var _ Executor = (*MyHarnessExecutor)(nil)`

## Step 5: Verify

```bash
task build        # Confirms compilation (YAML embed + init registration)
task check        # Full quality gate: fmt + lint + vuln + test
mush doctor       # Should show your harness in health check output
```

## Execution patterns

The two existing executors demonstrate the two main patterns:

### Persistent PTY (Claude)

A long-lived PTY process stays running across jobs. Jobs are injected as text into the PTY stdin. Completion is detected via a signal file created by a Claude Code hook.

- **Pros**: Fast job-to-job transitions, no startup overhead per job
- **Cons**: Complex prompt detection, signal file coordination, PTY lifecycle management
- **Key interfaces**: All 6 optional interfaces

### One-shot subprocess (Codex)

Each job spawns a fresh process. Output is captured from a temp file. The process exit signals completion.

- **Pros**: Simple lifecycle, clean isolation between jobs
- **Cons**: Startup overhead per job
- **Key interfaces**: `InputReceiver` only (for bundle load mode)

## Result shape conventions

`Execute` should return `ExecResult.OutputData` as:

```go
map[string]any{
    "success":    true,           // bool: whether the job succeeded
    "output":     "result text",  // string: captured output (ANSI-stripped)
    "durationMs": 1234,           // int: wall-clock execution time in ms
}
```

On failure, return `*ExecError` with an appropriate `Reason`:

| Reason | Meaning | Retry |
|--------|---------|-------|
| `"prompt_error"` | Failed to extract prompt from job | No |
| `"timeout"` | Context deadline exceeded | Yes |
| `"execution_error"` | General execution failure | Depends |
| `"codex_error"` | Codex-specific process failure | Yes |

## Gotchas

1. **Provider spec and registry are decoupled.** Nothing enforces that every YAML provider has a matching `Register()` call. A YAML-only provider appears in `ProviderNames()` and health checks but will fail silently when the worker tries to execute a job. A `Register()` without matching YAML panics at `mustGetProvider()`. Always create both files together and verify with `task build`.

2. **Empty `binary` means always available.** `AvailableFunc` returns `true` when the provider spec has an empty `binary` field (`provider.go:169`). This is intentional for built-in executors but surprising if you forget to set `binary` in your YAML.

3. **No `binary` validation in `validateProviderSpec`.** The validator checks `name`, `bundleDir.mode`, and `mcp.format`, but does not validate `binary` or `displayName`. A provider with no `binary` and no `displayName` passes validation silently.

4. **Unix-only constraint.** Both existing executors require `//go:build unix` because they use PTY and signal APIs. Non-unix platforms stub out the worker command entirely (`cmd/mush/worker_other.go`). Your executor must include this build tag.

5. **MCP format limited to json/toml.** Adding a new MCP format requires changes in three places: a new `Build*MCPConfig` function in `mcp_config.go`, a validation case in `validateProviderSpec` (`provider.go`), and file extension handling in `CreateMCPConfigFile` (`mcp_config.go`).
