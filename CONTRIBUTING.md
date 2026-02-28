# Contributing to Mush

Thanks for your interest in contributing to Mush! This guide will help you get started.

## Prerequisites

- **Go 1.26+** — [install](https://go.dev/dl/)
- **Task** — [install](https://taskfile.dev/installation/) (build automation)

## Setup

```bash
git clone https://github.com/musher-dev/mush.git
cd mush
task setup
task hooks:install
```

If you use the devcontainer, hooks are installed automatically during post-create.

## Development Workflow

```bash
# Format code
task fmt

# Run all checks (format, mod tidy, lint, vuln scan, shell/workflow lint, tests, install tests)
task check

# Run tests only
task check:test

# Build binary
task build

# Run without building
task run -- --help
task run -- link --dry-run
```

## Making Changes

1. **Fork** the repo and create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```

2. **Make your changes** — see [Code Style](#code-style) below.

3. **Run checks** before committing:
   ```bash
   task check
   ```

4. **Commit** using [Conventional Commits](https://www.conventionalcommits.org/):
   ```
   feat(link): add retry logic for transient failures
   fix(auth): handle expired token refresh
   docs: update configuration examples
   chore: bump golangci-lint to v2
   ```
   This project uses [Release Please](https://github.com/googleapis/release-please) to generate changelogs, so commit prefixes matter.

5. **Open a Pull Request** against `main`.

## Code Style

- **Formatting**: `golangci-lint fmt` with `gofumpt` + `goimports` (enforced by `task fmt` / `task check:fmt`)
- **Linting**: `golangci-lint` with strict rules (including `varnamelen`, complexity, test rigor, and `nolint` hygiene) via `task check:lint`
- **Shell/Workflows**: `shellcheck` + `shfmt` + `actionlint` via `task check:shell` and `task check:workflow`
- **Output**: All user-facing output goes through `internal/output.Writer` — never use `fmt.Print*` directly in commands
- **Commands**: Follow the noun-verb pattern (`mush <resource> <verb>`)
- **Errors**: Return errors with context (`fmt.Errorf("context: %w", err)`), never panic

See [CLAUDE.md](./.claude/CLAUDE.md) for detailed architecture and patterns.

## Tooling and Task Policy

- `Taskfile.yml` is the source of truth for local and CI quality checks.
- CI invokes `task` targets instead of duplicating check commands inline.
- Go-based dev tools are pinned in `go.mod` via the Go `tool` directive.
- Git hooks are repo-managed via `.githooks` and executed through Task targets.
- Run `task hooks:doctor` to validate hook setup.
- Hooks are local pre-flight checks; CI remains the final authoritative gate.

## Testing

```bash
# Run all tests
task check:test

# Run all static quality checks
task check

# Run a specific test
go test ./internal/auth/... -run TestKeyring

# Run with verbose output
go test ./... -v
```

- Unit tests live in `*_test.go` alongside the code they test
- Mock external dependencies (API, Claude Code, keyring)
- Use `output.NewWriter()` with buffers for testing CLI output

## Commit Messages

This project uses **Conventional Commits** with these prefixes:

| Prefix | Purpose | Appears in changelog? |
|--------|---------|----------------------|
| `feat` | New feature | Yes |
| `fix` | Bug fix | Yes |
| `docs` | Documentation | No |
| `chore` | Maintenance | No |
| `ci` | CI/CD changes | No |
| `refactor` | Code restructuring | No |
| `test` | Test changes | No |

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](./LICENSE).
