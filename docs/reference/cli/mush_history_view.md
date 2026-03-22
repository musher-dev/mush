---
title: "mush history view"
description: "View transcript events for a session"
---

## mush history view

View transcript events for a session

### Synopsis

Display the captured transcript events for a specific session.

Use --follow to tail the transcript in real time while a session is active.
Use --search to filter output to lines matching a substring.

```
mush history view <session-id> [flags]
```

### Examples

```
  mush history view SESSION_ID
  mush history view SESSION_ID --follow
```

### Options

```
      --follow          Follow updates as new transcript events are written
  -h, --help            help for view
      --raw             Show raw output including ANSI escape sequences
      --search string   Filter output to lines containing this substring
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

