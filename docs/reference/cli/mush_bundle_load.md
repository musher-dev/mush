---
title: "mush bundle load"
description: "Load a bundle into an ephemeral session"
---

## mush bundle load

Load a bundle into an ephemeral session

### Synopsis

Pull a bundle and launch the TUI at the Ready screen where you can choose
to Run or Install. Use --no-tui to skip the TUI and launch the harness
directly (requires --harness).

```
mush bundle load <namespace/slug>[:<version>] [flags]
```

### Examples

```
  mush bundle load acme/my-kit
  mush bundle load acme/my-kit:0.1.0
  mush bundle load acme/my-kit --no-tui --harness claude
```

### Options

```
      --force-sidebar    Skip terminal probe and force sidebar rendering
      --harness string   Harness type to use (required with --no-tui)
  -h, --help             help for load
```

### Options inherited from parent commands

```
      --api-key string   API key override (prefer MUSH_API_KEY env var)
      --api-url string   Override Musher API URL for this command
      --json             Output in JSON format
      --no-color         Disable colored output
      --no-input         Disable interactive prompts
      --no-tui           Disable interactive TUI navigation
      --quiet            Minimal output (for CI)
```

### SEE ALSO

* [mush bundle](mush_bundle.md)	 - Manage agent bundles

