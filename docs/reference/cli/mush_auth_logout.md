---
title: "mush auth logout"
description: "Clear stored credentials"
---

## mush auth logout

Clear stored credentials

### Synopsis

Remove stored API credentials from the system keyring. Does not affect the MUSHER_API_KEY environment variable.

```
mush auth logout [flags]
```

### Examples

```
  mush auth logout
```

### Options

```
  -h, --help   help for logout
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

* [mush auth](mush_auth.md)	 - Manage authentication

