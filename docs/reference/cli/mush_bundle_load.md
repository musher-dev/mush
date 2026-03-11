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

Alternatively, load a bundle from a local directory with --dir or use the
built-in sample bundle with --sample for testing.

```
mush bundle load [<namespace/slug>[:<version>]] [flags]
```

### Examples

```
  mush bundle load acme/my-kit
  mush bundle load acme/my-kit:0.1.0
  mush bundle load acme/my-kit --no-tui --harness claude
  mush bundle load --dir ./my-bundle --no-tui --harness claude
  mush bundle load --sample --no-tui --harness claude
```

### Options

```
      --dir string       Load bundle from a local directory
      --force-sidebar    Skip terminal probe and force sidebar rendering
      --harness string   Harness type to use (required with --no-tui)
  -h, --help             help for load
      --sample           Load the built-in sample bundle
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

