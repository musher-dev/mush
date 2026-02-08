# Mush

Local worker runtime for the Musher platform.

Mush connects your machine to the Musher job stream, claims jobs, executes handlers using Claude Code, and reports results back.

## Installation

```bash
# macOS (Homebrew)
brew install musher-dev/tap/mush

# Linux/macOS (direct download)
curl -sSL https://get.mush.dev | bash

# From source
go install github.com/musher-dev/mush/cmd/mush@latest
```

## Quick Start

```bash
# Authenticate
mush auth login

# View available habitats
mush habitat list

# Link to a habitat and start processing jobs (watch mode)
mush link
```

## Commands

Mush uses a **Resource-First (Noun-Verb)** command structure:

```
mush init                      Interactive onboarding wizard
mush doctor                    Run diagnostic checks

mush link                      Link to habitat and start processing
mush link --habitat <slug>     Link to specific habitat
mush link --dry-run            Verify connection without claiming jobs
mush link status               Show link status
mush unlink                    Gracefully disconnect

mush habitat list              List available habitats

mush auth login                Authenticate with your API key
mush auth status               Show authentication status
mush auth logout               Clear stored credentials

mush config list               List configuration
mush config get <key>          Get configuration value
mush config set <key> <value>  Set configuration value

mush version                   Show version info
mush --help                    Show help
```

## Global Flags

| Flag | Purpose |
|------|---------|
| `--json` | Output in JSON format |
| `--quiet` | Minimal output (for CI) |
| `--no-color` | Disable colored output |
| `--no-input` | Disable interactive prompts |
| `--verbose` / `-v` | Enable debug logging |

## Configuration

Mush looks for configuration in this order:

1. **Environment variables** (highest priority)
   - `MUSHER_API_KEY` — API key
   - `MUSH_API_URL` — Platform API URL

2. **Config file** (`~/.config/mush/config.yaml`)
   ```yaml
   api:
     url: https://api.musher.dev
   worker:
     poll_interval: 30
     heartbeat_interval: 30
   ```

## How It Works

```
Linear Issue → Musher Queue → Mush (linked to habitat) → Claude Code → Result
```

1. **Authenticate**: Mush authenticates with the Musher platform
2. **Select Habitat**: Choose an execution context for job routing
3. **Link**: Mush links to the habitat and polls for jobs
4. **Claim**: When a job is available, Mush claims it (acquires a lease)
5. **Execute**: Mush executes jobs in an interactive watch UI (Claude via PTY; Bash via subprocess)
6. **Heartbeat**: While executing, Mush sends heartbeats to maintain the lease
7. **Complete**: Results are reported back to the platform

## Repository Structure

```
mush/
├── cmd/mush/          # CLI entry point and commands
│   ├── main.go         # Root command setup, global flags
│   ├── link_unix.go    # link command (watch mode, Unix)
│   ├── link_other.go   # link command stub (non-Unix)
│   ├── link_common.go  # shared link helpers + status/unlink
│   ├── habitat.go      # habitat list commands
│   ├── auth.go         # auth login/status/logout
│   ├── config.go       # config list/get/set
│   ├── init.go         # Onboarding wizard
│   ├── doctor.go       # Diagnostic command
│   └── completion.go   # shell completion output
├── internal/
│   ├── auth/           # Credential storage (keyring + file fallback)
│   ├── client/         # API client for Musher platform
│   ├── claude/         # Claude Code CLI availability checks
│   ├── config/         # Viper configuration management
│   ├── doctor/         # Diagnostic check framework
│   ├── errors/         # CLI error types and handling
│   ├── harness/        # Watch UI (PTY + scroll region)
│   ├── output/         # CLI output handling (colors, spinners)
│   ├── prompt/         # Interactive user prompts
│   ├── terminal/       # TTY detection and capabilities
│   └── wizard/         # Init wizard flow
├── testdata/           # Test fixtures
├── go.mod
├── Taskfile.yml        # Build automation
└── .goreleaser.yaml    # Release configuration
```

## Running Locally

For development, you can run mush without installing.

### Prerequisites

- Go 1.25+
- [Task](https://taskfile.dev) (optional, for convenience commands)

### Quick Start

```bash
cd apps/platform/mush

# Install dependencies
go mod download

# Run directly with go run
go run ./cmd/mush --help
go run ./cmd/mush auth login
go run ./cmd/mush link

# Or use Task
task setup              # Install dependencies and tools
task run -- --help      # Run with arguments
task run -- auth login  # Authenticate
task run -- link        # Link to a habitat

# Build and run binary
task build              # Creates ./mush binary
./mush --help
```

### Development Workflow

```bash
task fmt                # Format code
task check              # Run all checks (fmt + lint + vuln + test)
task test               # Run tests only
```

## Development

See [CLAUDE.md](./CLAUDE.md) for development patterns.

```bash
# Setup
task setup

# Build
task build

# Run checks
task check
```

## License

Copyright 2024 Musher, Inc.
