---
title: "mush bundle info"
description: "Show details for a bundle reference"
---

## mush bundle info

Show details for a bundle reference

### Synopsis

Show hub metadata, cached versions, and installation status for a bundle.

Queries the Musher Hub for bundle details (public, no auth required) and
also checks the local cache and current project directory.

```
mush bundle info <namespace/slug>[:<version>] [flags]
```

### Examples

```
  mush bundle info acme/my-agent-kit
  mush bundle info acme/my-agent-kit:1.0.0
```

### Options

```
  -h, --help   help for info
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

* [mush bundle](mush_bundle.md)	 - Manage agent bundles

