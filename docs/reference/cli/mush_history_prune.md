---
title: "mush history prune"
description: "Delete transcript sessions older than a duration"
---

## mush history prune

Delete transcript sessions older than a duration

### Synopsis

Delete transcript sessions older than the configured retention window.

The default retention comes from the history.retention config key (default 720h).
Use --older-than to override. Requires confirmation unless --force is passed.

```
mush history prune [flags]
```

### Examples

```
  mush history prune
  mush history prune --older-than 168h
  mush history prune --force
```

### Options

```
  -f, --force               Skip confirmation prompt
  -h, --help                help for prune
      --older-than string   Override retention window (example: 168h)
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

* [mush history](mush_history.md)	 - Inspect transcript history from PTY sessions

