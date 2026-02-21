# Architecture Boundaries and Enforcement

This document defines the dependency boundaries for `mush` and how linting enforces them.

## Goals

- Keep command/UI concerns separate from core platform logic.
- Prevent dependency drift as the codebase grows.
- Catch architecture violations automatically in local and CI lint runs.

## Layers

### 1) CLI Entry Layer

- `cmd/mush`
- Responsibility: command wiring, flags, user interaction orchestration, exit semantics.

### 2) Feature/Orchestration Layer

- `internal/harness`
- `internal/wizard`
- `internal/doctor`
- `internal/prompt`
- `internal/output`
- Responsibility: workflow orchestration, user-facing interaction behavior, diagnostics presentation.

### 3) Platform/Core Layer

- `internal/client`
- `internal/auth`
- `internal/config`
- `internal/update`
- `internal/linking`
- `internal/errors`
- `internal/buildinfo`
- `internal/terminal`
- `internal/testutil` *(test helpers only — not imported by production code)*
- Responsibility: API transport, credential/config state, platform operations, shared primitives.

## Enforced Boundaries

1. `internal/**` packages must never import `cmd/mush`.
2. Platform/Core packages must not import presentation packages:
   - `github.com/musher-dev/mush/internal/output`
   - `github.com/musher-dev/mush/internal/prompt`
3. `internal/doctor` must not import `internal/output` (diagnostics model remains output-agnostic).

## Allowed Dependency Direction

- `cmd/mush` -> `internal/*` (allowed)
- Feature/Orchestration -> Platform/Core (allowed)
- Platform/Core -> Feature/Orchestration (forbidden)
- Any `internal/*` -> `cmd/mush` (forbidden)

## Linter Enforcement

Boundaries are enforced with `depguard` in `.golangci.yml` via scoped rules:

- `internal_no_cmd_import`
- `platform_no_presentation`
- `doctor_no_output`

The rules run as part of:

- `task check:lint`
- `task check`
- CI workflow lint job (`.github/workflows/ci.yml`)

## Existing Quality Linters Relevant to Architecture

- `depguard`: import boundary enforcement.
- `staticcheck`, `govet`, `gocritic`: correctness and code quality.
- `revive`: style and consistency.
- `errcheck`, `noctx`, `bodyclose`: operational correctness.

## Refactor Guidance

When a package needs to communicate status/errors upward:

- Prefer returning `error` values or callbacks/interfaces defined close to the caller.
- Avoid direct dependency on `internal/output` from Platform/Core packages.
- Keep rendering and user messaging in `cmd/mush` or Feature/Orchestration packages.
