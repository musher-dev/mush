# Golden Path Step Contracts (CLI Scope)

This contract defines the hardened CLI golden path in this repository.

## Scope

`install -> auth -> init -> worker dry-run -> first claim lifecycle`

## Step 1: Install

- Entry:
  - Supported OS/arch
  - Network access to release artifacts
- Exit:
  - `mush` binary in `$PATH`
  - `mush version` returns semver/build info
- Invariants:
  - Checksum verification is enforced by `install.sh`
  - Install is repeatable and safe to rerun

## Step 2: Authenticate

- Entry:
  - API key available via `MUSHER_API_KEY`, `mush auth login`, or `mush init`
- Exit:
  - `mush auth status` returns credential/organization identity
- Invariants:
  - API key is never printed back in logs/output
  - Auth failures return actionable hint
  - Request correlation metadata (`request_id`, `trace_id`) is surfaced when available

## Step 3: Initialize

- Entry:
  - CLI can prompt, or non-interactive flags/env are provided
- Exit:
  - Credentials validated and stored (or sourced from env)
  - Optional habitat selected and persisted
- Invariants:
  - `mush init --force` is explicit for overwrite
  - Non-interactive setup is deterministic with `--api-key` and `--habitat`

## Step 4: Worker Dry Run

- Entry:
  - Authenticated identity
  - Habitat and queue resolvable
- Exit:
  - `mush worker start --dry-run` verifies API connectivity and queue configuration
- Invariants:
  - No job is claimed in dry-run mode
  - Missing queue instruction yields explicit, actionable error

## Step 5: Claim / Release Lifecycle

- Entry:
  - Runner has queue access
- Exit:
  - Job claim response is parseable
  - Claimed job can be released safely
- Invariants:
  - Required response fields are contract-tested in CI
  - CLI/API boundary changes must preserve contract decoding for existing payloads

## Contract Fixtures

Canonical fixture payloads are stored in `test/contracts/` and verified by
`internal/client/contracts_test.go`.
