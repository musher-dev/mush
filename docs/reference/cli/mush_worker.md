---
title: "mush worker"
description: "Manage the local worker runtime"
---

## mush worker

Manage the local worker runtime

### Synopsis

Manage the local worker runtime that connects your machine to a habitat
and processes jobs from the Musher platform.

Use subcommands to start the worker.

### Examples

```
  mush worker start
  mush worker start --habitat prod --queue jobs
```

### Options

```
  -h, --help   help for worker
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

* [mush](mush.md)	 - Portable agent bundles for local coding agents
* [mush worker start](mush_worker_start.md)	 - Start the worker and begin processing jobs

