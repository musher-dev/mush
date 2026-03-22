---
title: "mush config list"
description: "List all configuration settings"
---

## mush config list

List all configuration settings

### Synopsis

Display all configuration settings and their current values. Shows available settings with defaults when none are set.

```
mush config list [flags]
```

### Examples

```
  mush config list
  mush config list --json
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

* [mush config](mush_config.md)	 - Manage configuration

