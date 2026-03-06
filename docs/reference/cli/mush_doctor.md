---
title: "mush doctor"
description: "Diagnose common issues"
---

## mush doctor

Diagnose common issues

### Synopsis

Run diagnostic checks to identify configuration and connectivity issues.

Checks performed:
  - Directory structure and permissions
  - Configuration file validity
  - Credential file security
  - API connectivity and response time
  - Authentication status
  - CLI version

```
mush doctor [flags]
```

### Examples

```
  mush doctor
```

### Options

```
  -h, --help   help for doctor
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

