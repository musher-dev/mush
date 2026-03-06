---
title: "mush init"
description: "Setup Mush for first use"
---

## mush init

Setup Mush for first use

### Synopsis

Initialize Mush with a guided setup wizard.

The wizard will:
  1. Prompt for your API key
  2. Validate the connection
  3. Store credentials securely
  4. Show next steps

If credentials already exist, use --force to overwrite them.

```
mush init [flags]
```

### Examples

```
  mush init
```

### Options

```
      --api-key string   API key to use for non-interactive initialization
  -f, --force            Overwrite existing credentials without prompting
      --habitat string   Habitat slug or ID to select during initialization
  -h, --help             help for init
```

### Options inherited from parent commands

```
      --api-url string   Override Musher API URL for this command
      --json             Output in JSON format
      --no-color         Disable colored output
      --no-input         Disable interactive prompts
      --no-tui           Disable interactive TUI navigation
      --quiet            Minimal output (for CI)
```

### SEE ALSO

* [mush](mush.md)	 - Portable agent bundles for local coding agents

