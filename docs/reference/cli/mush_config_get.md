---
title: "mush config get"
description: "Get a configuration value"
---

## mush config get

Get a configuration value

### Synopsis

Retrieve and display the current value of a single configuration key.

```
mush config get <key> [flags]
```

### Examples

```
  mush config get api.url
```

### Options

```
  -h, --help   help for get
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

* [mush config](mush_config.md)	 - Manage configuration

