---
title: "mush history list"
description: "List stored transcript sessions"
---

## mush history list

List stored transcript sessions

### Synopsis

List all locally stored transcript sessions with their start and close times.

```
mush history list [flags]
```

### Examples

```
  mush history list
  mush history list --json
```

### Options

```
  -h, --help   help for list
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

* [mush history](mush_history.md)	 - Inspect transcript history from PTY sessions

