---
title: "mush bundle"
description: "Manage agent bundles"
---

## mush bundle

Manage agent bundles

### Synopsis

Pull versioned collections of agent assets from the Musher platform
and either load them ephemerally or install them into a harness's native
directory structure.

### Options

```
  -h, --help   help for bundle
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
* [mush bundle info](mush_bundle_info.md)	 - Show local details for a bundle reference
* [mush bundle install](mush_bundle_install.md)	 - Install bundle assets into the current project
* [mush bundle list](mush_bundle_list.md)	 - List local bundle cache and installed bundles
* [mush bundle load](mush_bundle_load.md)	 - Load a bundle into an ephemeral session
* [mush bundle uninstall](mush_bundle_uninstall.md)	 - Remove installed bundle assets from the current project

