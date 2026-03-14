---
title: "mush"
description: "Portable agent bundles for local coding agents"
---

## mush

Portable agent bundles for local coding agents

### Synopsis

Load, install, and manage agent bundles from the Musher Hub.
Browse bundles, run them ephemerally, or install assets into
your project's harness directory.

Get started:  mush bundle load

```
mush [flags]
```

### Examples

```
  mush bundle load acme/my-kit
```

### Options

```
      --api-key string   API key override (prefer MUSH_API_KEY env var)
      --api-url string   Override Musher API URL for this command
  -h, --help             help for mush
      --json             Output in JSON format
      --no-color         Disable colored output
      --no-input         Disable interactive prompts
      --no-tui           Disable interactive TUI navigation
      --quiet            Minimal output (for CI)
```

### Hidden Flags

These flags are omitted from `--help` but remain fully functional.
They can also be set via environment variables (`MUSH_LOG_LEVEL`, etc.).

```
      --experimental        Enable experimental features
      --log-file string     Optional structured log file path
      --log-format string   Log format: json, text
      --log-level string    Log level: error, warn, info, debug
      --log-stderr string   Structured logging to stderr: auto, on, off
```

### SEE ALSO

* [mush auth](mush_auth.md)	 - Manage authentication
* [mush bundle](mush_bundle.md)	 - Manage agent bundles
* [mush completion](mush_completion.md)	 - Generate shell completion scripts
* [mush config](mush_config.md)	 - Manage configuration
* [mush doctor](mush_doctor.md)	 - Diagnose common issues
* [mush habitat](mush_habitat.md)	 - Manage habitats
* [mush history](mush_history.md)	 - Inspect transcript history from PTY sessions
* [mush init](mush_init.md)	 - Setup Mush for first use
* [mush paths](mush_paths.md)	 - Show where Mush stores files
* [mush update](mush_update.md)	 - Update mush to the latest version
* [mush version](mush_version.md)	 - Show version information
* [mush worker](mush_worker.md)	 - Manage the local worker runtime

