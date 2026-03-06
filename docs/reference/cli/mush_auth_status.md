---
title: "mush auth status"
description: "Show authentication status"
---

## mush auth status

Show authentication status

### Synopsis

Validate stored credentials against the Musher API and display the authenticated identity.

```
mush auth status [flags]
```

### Examples

```
  mush auth status
  mush auth status --json
```

### Options

```
  -h, --help   help for status
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

* [mush auth](mush_auth.md)	 - Manage authentication

