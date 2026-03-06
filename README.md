# Mush

[![CI](https://github.com/musher-dev/mush/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/musher-dev/mush/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/musher-dev/mush)](https://github.com/musher-dev/mush/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/musher-dev/mush)](https://go.dev/)
[![License](https://img.shields.io/github/license/musher-dev/mush)](./LICENSE)

Portable agent bundles for local coding agents.

![Mush CLI Demo](docs/vhs/gif/demo.gif)

- Browse, load, and install versioned agent bundles from the Musher Hub
- Run bundles ephemerally or install assets into your project
- Interactive TUI for discovery, download, and harness selection
- Built-in diagnostics, self-update, and shell completions
- Remote job runner for platform-driven execution (advanced)

## Installation

```bash
curl -fsSL https://get.musher.dev | sh
```

<details>
<summary>Other install methods</summary>

```bash
# Install a specific version
curl -fsSL https://get.musher.dev | sh -s -- --version <version>

# Install and also install tmux if missing
curl -fsSL https://get.musher.dev | sh -s -- --install-tmux

# From source
go install github.com/musher-dev/mush/cmd/mush@latest
```

</details>

## Quick Start

```bash
mush bundle load acme/my-kit              # Ephemeral session
mush bundle install acme/my-kit --harness claude  # Install into project
mush bundle list                           # See cached/installed
```

Browse public bundles on [Musher Hub](https://hub.musher.dev). Run `mush doctor` to verify your setup.

## Commands

Mush uses noun-verb command structure. Run `mush <command> --help` for details.

### Bundles

```
mush bundle load <namespace/slug>[:<version>]        Load a bundle into an ephemeral session
mush bundle install <namespace/slug>[:<version>]     Install bundle assets into the current project
mush bundle list               List local bundle cache and installed bundles
mush bundle info <namespace/slug>[:<version>]        Show local details for a bundle reference
mush bundle uninstall <namespace/slug>[:<version>]   Remove installed bundle assets
```

### Account

```
mush auth login                Authenticate with your API key
mush auth status               Show authentication status
mush auth logout               Clear stored credentials

mush config list               List configuration
mush config get <key>          Get configuration value
mush config set <key> <value>  Set configuration value
```

### History

```
mush history list              List stored transcript sessions
mush history view <id>         View transcript events for a session
mush history prune             Delete sessions older than a duration
```

### Setup

```
mush init                      Guided onboarding wizard
mush doctor                    Run diagnostic checks
mush update                    Update to the latest version
mush version                   Show version information
mush completion <shell>        Generate shell completion scripts
```

### Advanced: Remote Runner

> The remote runner connects dev machines to the Musher job queue for platform-driven execution.

```
mush worker start                      Start the worker and process jobs
mush worker start --habitat <slug>     Connect to specific habitat
mush worker start --harness <type>     Use a specific harness (claude or bash)
mush worker start --dry-run            Verify connection without claiming jobs

mush habitat list              List available habitats
```

## Configuration

Mush looks for configuration in this order (highest priority first):

1. **CLI flags** (`--api-url`, global)
2. **Environment variables** (`MUSH_API_KEY`, `MUSH_API_URL`, `MUSH_*`)
3. **OS Keyring** (credentials only)
4. **Config file** (`<user config dir>/mush/config.yaml`)
5. **Built-in defaults**

```yaml
api:
  url: https://api.musher.dev
worker:
  poll_interval: 30
  heartbeat_interval: 30
```

See [Configuration and Data Storage](docs/configuration.md) for all config keys, environment variables, file locations, credential storage details, and global flags.

### Global flags

Use `--api-url` to override the platform API endpoint for a single command invocation:

```bash
mush --api-url https://api.staging.musher.dev worker start --dry-run
mush --api-url http://localhost:8080 doctor
```

`--api-url` takes precedence over `MUSH_API_URL` and `api.url` from config for that process.

`--api-key` is not a global flag. It is available as `mush auth login --api-key ...`, and `MUSH_API_KEY` is preferred for non-interactive usage.

For enterprise TLS interception/proxy environments, set `MUSH_NETWORK_CA_CERT_FILE` to a PEM CA bundle trusted by your organization.

## Contributing

See [CONTRIBUTING.md](./.github/CONTRIBUTING.md) for development setup, code style, and testing.

## License

MIT License — Copyright (c) 2026 musher-dev. See [LICENSE](./LICENSE).
