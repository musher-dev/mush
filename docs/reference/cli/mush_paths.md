---
title: "mush paths"
description: "Show where Mush stores files"
---

## mush paths

Show where Mush stores files

### Synopsis

Display all file and directory paths used by Mush.

Useful for debugging, scripting, and understanding where configuration,
state, cache, and credential files are stored on this system.

```
mush paths [flags]
```

### Examples

```
  mush paths
  mush paths --json
```

### Options

```
  -h, --help   help for paths
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

* [mush](mush.md)	 - Portable agent bundles for local coding agents

