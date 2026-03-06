---
title: "mush history"
description: "Inspect transcript history from PTY sessions"
---

## mush history

Inspect transcript history from PTY sessions

### Synopsis

Inspect and manage transcript history captured during PTY harness sessions.

Transcripts are stored locally and can be listed, viewed, or pruned to free
disk space.

### Options

```
  -h, --help   help for history
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
* [mush history list](mush_history_list.md)	 - List stored transcript sessions
* [mush history prune](mush_history_prune.md)	 - Delete transcript sessions older than a duration
* [mush history view](mush_history_view.md)	 - View transcript events for a session

