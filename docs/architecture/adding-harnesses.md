# Adding a New Harness

This guide explains how to add a new harness provider to Mush using the current module-based provider architecture.

## Architecture overview

Each harness is implemented as a provider module under:

`internal/harness/providers/{name}/`

A provider module contains:

1. `spec.yaml`
   Declarative provider metadata (binary, status checks, MCP and bundle mapping settings).
2. `module.go`
   Embeds and parses `spec.yaml`, then exports a `Module` (`harnesstype.Module`).
3. `executor.go` (unix build)
   Runtime implementation that satisfies `harnesstype.Executor`.

At startup:

- On unix, provider modules are registered in `internal/harness/builtins.go`.
- On non-unix, only provider specs are registered in `internal/harness/builtins_nonunix.go`.

This separation keeps health/status metadata available cross-platform while execution remains unix-only.

## File checklist

Create these files:

- `internal/harness/providers/{name}/spec.yaml`
- `internal/harness/providers/{name}/module.go`
- `internal/harness/providers/{name}/executor.go` (`//go:build unix`)
- `internal/harness/providers/{name}/executor_test.go` (recommended)

Update these registries:

- `internal/harness/builtins.go` (add module to unix built-ins)
- `internal/harness/builtins_nonunix.go` (register spec for non-unix)

## Step 1: Write `spec.yaml`

`spec.yaml` is parsed into `harnesstype.ProviderSpec`.

```yaml
name: myharness
displayName: My Harness
description: My AI coding assistant
binary: myharness

directories:
  project: .myharness
  user: ~/.myharness

bundleDir:
  mode: add_dir
  flag: --add-dir

cli:
  mcpConfig: --mcp-config

assets:
  skillDir: .myharness/skills
  agentDir: .myharness/agents
  toolConfigFile: .myharness/config.json

mcp:
  format: json
  configPath: .myharness/mcp.json

status:
  versionArgs: ["--version"]
  installHint: npm install -g myharness
  installCommand: ["npm", "install", "-g", "myharness"]
  configDir: ~/.myharness
  authCheck:
    path: ~/.myharness/credentials.json
    description: Credentials
```

Validation is enforced by `harnesstype.MustParseSpec`:

- `name` is required
- `bundleDir.mode` must be one of `add_dir`, `cd_flag`, `cwd`
- `mcp.format` must be `json` or `toml`

## Step 2: Export `Module` in `module.go`

```go
//go:build unix

package myharness

import (
    _ "embed"

    "github.com/musher-dev/mush/internal/harness/harnesstype"
)

//go:embed spec.yaml
var specData []byte

var spec = harnesstype.MustParseSpec(specData)

var Module = harnesstype.Module{
    Spec: spec,
    NewExecutor: func() harnesstype.Executor {
        return NewExecutor()
    },
    MCPSpec: &harnesstype.MCPSpec{
        Def:         spec.MCP,
        BuildConfig: harnesstype.BuildJSONMCPConfig,
    },
}
```

Use `MCPSpec: nil` if the harness does not support MCP.

## Step 3: Implement `executor.go`

The executor must satisfy `harnesstype.Executor`:

- `Setup(ctx, opts)`
- `Execute(ctx, job)`
- `Reset(ctx)`
- `Teardown()`

Optional interfaces are detected via type assertions:

- `harnesstype.Resizable`
- `harnesstype.InputReceiver`
- `harnesstype.Refreshable`
- `harnesstype.SignalDirConsumer`
- `harnesstype.TranscriptSource`
- `harnesstype.InterruptHandler`

Use `harnesstype.GetPromptFromJob(job)` for prompt extraction and return `*harnesstype.ExecError` for classified failures.

## Step 4: Register built-ins

Add the new module to unix registration in `internal/harness/builtins.go`.

Add spec registration for non-unix in `internal/harness/builtins_nonunix.go`.

If you skip non-unix registration, provider metadata will be missing from non-unix flows that rely on `GetProvider`/`ProviderNames`.

## Step 5: Test and verify

Recommended:

```bash
go test ./internal/harness/...
go test ./internal/harness/providers/{name}/...
```

Full checks:

```bash
task build
task check
```

## Gotchas

1. Module wiring and builtins registration are separate.
A valid provider folder is not active until `builtins.go`/`builtins_nonunix.go` imports and registers it.

2. `binary: ""` means always available.
`harnesstype.AvailableFunc` returns `true` when no binary is set.

3. Keep unix constraints consistent.
Executor runtime uses unix facilities for PTY/signal behavior; non-unix should only expose metadata.
