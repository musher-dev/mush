---
title: "mush update"
description: "Update mush to the latest version"
---

## mush update

Update mush to the latest version

### Synopsis

Update mush to the latest version from GitHub Releases.

Downloads the new binary, verifies its checksum, and replaces the current
executable. If the binary is not writable, sudo is requested automatically.

Set MUSH_UPDATE_DISABLED=1 to disable update checks.

```
mush update [flags]
```

### Examples

```
  mush update
  mush update --version 1.2.3
  mush update --force
```

### Options

```
  -f, --force            Force update even if already up to date
  -h, --help             help for update
      --version string   Install a specific version (e.g. 1.2.3)
```

### Options inherited from parent commands

```
      --api-key string   API key override (prefer MUSHER_API_KEY env var)
      --api-url string   Override Musher API URL for this command
      --json             Output in JSON format
      --no-color         Disable colored output
      --no-input         Disable interactive prompts
      --no-tui           Disable interactive TUI navigation
      --quiet            Minimal output (for CI)
```

### SEE ALSO

* [mush](mush.md)	 - Portable agent bundles for local coding agents

