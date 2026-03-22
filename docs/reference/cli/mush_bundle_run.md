---
title: "mush bundle run"
description: "Run a bundle directly with a harness"
---

## mush bundle run

Run a bundle directly with a harness

### Synopsis

Pull a bundle and launch the harness binary directly as a subprocess with
inherited stdio. No TUI, no PTY wrapping — you get the raw harness
experience as if you ran it yourself.

Use --dir to load a bundle from a local directory or --sample to use the
built-in sample bundle for testing.

```
mush bundle run [<namespace/slug>[:<version>]] --harness <type> [flags]
```

### Examples

```
  mush bundle run acme/my-kit --harness claude
  mush bundle run acme/my-kit:0.1.0 --harness claude
  mush bundle run --dir ./my-bundle --harness claude
  mush bundle run --sample --harness claude
```

### Options

```
      --dir string       Load bundle from a local directory
      --harness string   Harness type to use (required)
  -h, --help             help for run
      --sample           Load the built-in sample bundle
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

