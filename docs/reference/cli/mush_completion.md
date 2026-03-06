---
title: "mush completion"
description: "Generate shell completion scripts"
---

## mush completion

Generate shell completion scripts

### Synopsis

Generate shell completion scripts for Mush CLI.

To load completions:

Bash:
  $ source <(mush completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ mush completion bash > /etc/bash_completion.d/mush
  # macOS:
  $ mush completion bash > $(brew --prefix)/etc/bash_completion.d/mush

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ mush completion zsh > "${fpath[1]}/_mush"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ mush completion fish | source

  # To load completions for each session, execute once:
  $ mush completion fish > ~/.config/fish/completions/mush.fish

PowerShell:
  PS> mush completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> mush completion powershell > mush.ps1
  # and source this file from your PowerShell profile.


```
mush completion [bash|zsh|fish|powershell]
```

### Examples

```
  mush completion bash
  source <(mush completion bash)
```

### Options

```
  -h, --help   help for completion
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

