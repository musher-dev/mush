---
title: "mush config set"
description: "Set a configuration value"
---

## mush config set

Set a configuration value

### Synopsis

Set a configuration key to the given value. The value is persisted to the config file.

```
mush config set <key> <value> [flags]
```

### Examples

```
  mush config set api.url https://api.example.com
```

### Options

```
  -h, --help   help for set
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

