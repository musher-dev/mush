---
title: "mush bundle uninstall"
description: "Remove installed bundle assets from the current project"
---

## mush bundle uninstall

Remove installed bundle assets from the current project

### Synopsis

Remove previously installed bundle assets from the current project directory.

Lists the files that will be removed and prompts for confirmation unless
--force is passed.

```
mush bundle uninstall <namespace/slug>[:<version>] --harness <type> [flags]
```

### Examples

```
  mush bundle uninstall acme/my-kit --harness claude
  mush bundle uninstall acme/my-kit:1.0.0 --harness claude --force
```

### Options

```
  -f, --force            Skip confirmation prompt
      --harness string   Harness type to uninstall from (required)
  -h, --help             help for uninstall
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

* [mush bundle](mush_bundle.md)	 - Manage agent bundles

