---
title: "mush auth login"
description: "Authenticate with your API key"
---

## mush auth login

Authenticate with your API key

### Synopsis

Authenticate with the Musher platform.

Your API key will be stored securely in your system's keyring
(macOS Keychain, Windows Credential Manager, or Linux Secret Service).

You can also set the MUSHER_API_KEY environment variable.

```
mush auth login [flags]
```

### Examples

```
  mush auth login
  mush --api-key sk-... auth login
```

### Options

```
  -h, --help   help for login
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

