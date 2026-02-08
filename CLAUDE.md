# Mush CLI

Local worker runtime for the Musher platform. Connects developer machines to the job queue, executes handlers using Claude Code, and reports results.

## Architecture

```
mush/
├── cmd/mush/          # CLI entry point and commands
│   ├── main.go         # Root command setup, global flags
│   ├── link_unix.go    # link command (watch mode, Unix)
│   ├── link_other.go   # link command stub (non-Unix)
│   ├── link_common.go  # shared link helpers + status/unlink
│   ├── auth.go         # auth login/status/logout commands
│   ├── config.go       # config list/get/set commands
│   ├── init.go         # Onboarding wizard
│   └── doctor.go       # Diagnostic command
├── internal/
│   ├── auth/           # Credential storage (keyring + file fallback)
│   ├── client/         # API client for Musher platform
│   ├── config/         # Viper configuration management
│   ├── claude/         # Claude Code CLI availability checks
│   ├── harness/        # Watch UI (PTY + scroll region)
│   ├── output/         # CLI output handling (colors, spinners, TTY)
│   ├── terminal/       # TTY detection and capabilities
│   ├── prompt/         # Interactive user prompts
│   ├── wizard/         # Init wizard flow
│   └── doctor/         # Diagnostic check framework
├── go.mod
├── Taskfile.yml
└── .goreleaser.yaml
```

## Command Structure

Mush uses **Resource-First (Noun-Verb)** command taxonomy:

```bash
mush init               # Onboarding wizard
mush doctor             # Diagnostic checks

mush link               # Link to a habitat and start processing jobs (watch mode)
mush link status        # Check authentication/connectivity
mush unlink             # Disconnect placeholder

mush auth login         # Authenticate with API key
mush auth status        # Show auth status
mush auth logout        # Clear credentials

mush config list        # List all config
mush config get <key>   # Get config value
mush config set <k> <v> # Set config value
```

### Standard Verbs

- `list` - List multiple resources
- `get` - Get single resource (aliases: `show`, `describe`)
- `create` - Create resource
- `update` - Modify resource (alias: `set`)
- `delete` - Remove resource
- `logs` - Stream/view logs
- `start/stop` - Lifecycle control

## Essential Commands

```bash
# Build
task build              # Build mush binary
task build:all          # Build for all platforms

# Development
task run -- --help      # Run with arguments
task run -- link        # Run watch mode

# Code Quality
task check              # All checks (fmt + lint + test)
task check:lint         # Run golangci-lint
task check:test         # Run tests
task fmt                # Format code

# Diagnostics
task run -- doctor      # Run diagnostics
```

## Code Patterns

### Command Structure (Cobra)

Commands use noun-verb structure with output via context:

```go
func newExampleCmd() *cobra.Command {
    var someFlag string

    cmd := &cobra.Command{
        Use:   "example",
        Short: "Short description",
        Long:  `Longer description with details.`,
        RunE: func(cmd *cobra.Command, args []string) error {
            out := output.FromContext(cmd.Context())

            // Use out.Success(), out.Failure(), out.Warning(), out.Info()
            out.Success("Operation completed")
            return nil
        },
    }

    cmd.Flags().StringVar(&someFlag, "flag", "default", "Flag description")
    return cmd
}
```

### Output Package

All CLI output goes through `internal/output.Writer`:

```go
out := output.FromContext(cmd.Context())

// Status messages with symbols
out.Success("Connected")          // ✓ Connected
out.Failure("Failed to connect")  // ✗ Failed to connect
out.Warning("Token expires soon") // ⚠ Token expires soon
out.Info("Run mush doctor")      // ℹ Run mush doctor

// Spinners for async operations
spin := out.Spinner("Connecting")
spin.Start()
// ... do work ...
spin.StopWithSuccess("Connected")

// Debug output (--verbose only)
out.Debug("API response: %v", resp)

// Muted/gray text
out.Muted("Optional information")
```

### Global Flags

| Flag | Purpose |
|------|---------|
| `--json` | Output in JSON format |
| `--quiet` | Minimal output (for CI) |
| `--no-color` | Disable colored output |
| `--no-input` | Disable interactive prompts |
| `--verbose` / `-v` | Enable debug logging |

### Error Handling

- Return errors with context using `fmt.Errorf("context: %w", err)`
- Never panic; always return errors
- User-facing errors should be actionable
- Use `out.Failure()` for user-visible errors, return `nil`

### Configuration Priority

1. Environment variables (`MUSH_*`, `MUSHER_API_KEY`)
2. OS Keyring (for credentials)
3. Config file (`~/.config/mush/config.yaml`)
4. Built-in defaults

### Testing

- Unit tests in `*_test.go` files
- Integration tests use `_integration_test.go` suffix
- Mock external dependencies (API, Claude Code)
- Use `output.NewWriter()` with buffers for testing output

## Dependencies

Core dependencies:
- `github.com/spf13/cobra` — CLI framework
- `github.com/spf13/viper` — Configuration
- `github.com/zalando/go-keyring` — Secure credential storage
- `github.com/fatih/color` — Colored terminal output
- `github.com/briandowns/spinner` — Terminal spinners

## Build & Release

Uses GoReleaser for cross-platform builds:

```bash
# Local build
task build

# Release (via CI)
goreleaser release --clean
```

## Quick Reference

- **Binary**: `mush`
- **Config dir**: `~/.config/mush/`
- **Credentials**: OS Keyring or `~/.config/mush/credentials`
- **API endpoint**: Configured via `api.url` or `MUSH_API_URL`
