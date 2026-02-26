# Worker TUI: Preparedness & Governance

## Current TUI Capabilities

Mush already has interactive terminal features built without a framework dependency:

- **Status bar + scroll region** — `internal/harness/` uses ANSI escape sequences to render a live status bar and scrolling PTY output during job execution.
- **Arrow-key prompt selection** — `internal/prompt/` provides arrow-key navigation with Up/Down cursor movement, Enter to confirm, Esc to cancel. Falls back to numbered input when stdin is not a TTY.
- **Raw mode management** — `golang.org/x/term` handles raw mode entry/exit for interactive prompts and password input.
- **Copy mode** — Ctrl+S toggles a copy-friendly mode in the watch UI; Esc returns to live input.

These features work well for the current use cases without adding a TUI framework dependency.

## Framework Gap Analysis

### What a TUI framework would unlock

A framework like [Bubble Tea](https://github.com/charmbracelet/bubbletea) would provide:

- **Multi-pane layouts** — Split views for simultaneous job list + execution output.
- **Scrollable viewports** — Buffered scroll-back with mouse support.
- **Reusable widgets** — Tables, text inputs, spinners, progress bars from the `bubbles` library.
- **Layout engine** — Flexbox-like layout via `lipgloss` for responsive terminal UIs.

### What it would cost

- **New dependency** — `bubbletea` + `bubbles` + `lipgloss` added to `go.mod` (Charmbracelet packages are already transitive via linters in `go.sum`, but not production dependencies).
- **Elm architecture** — Bubble Tea uses a Model-Update-View loop that differs from the current imperative rendering approach. Existing harness code would need adaptation.
- **Second rendering layer** — The watch UI's ANSI scroll region and the TUI framework's renderer could conflict. Careful integration boundaries would be needed.

## Bare-Noun TUI Prohibition

**Governance decision:** Bare noun commands MUST show help text.

All six noun commands follow this pattern consistently:

```
mush worker    → help
mush bundle    → help
mush auth      → help
mush config    → help
mush history   → help
mush habitat   → help
```

This contract ensures:
- **Discoverability** — Users who type a noun always see available subcommands.
- **Non-TTY safety** — Help text renders correctly in pipes, scripts, and CI environments.
- **Consistency** — No noun has special behavior; all follow the same pattern.

If interactive selection is added in the future, it goes on **`mush worker start`** (which already has interactive habitat/queue prompts), not on bare `mush worker`.

## Future TUI Adoption Path

If/when a TUI framework is adopted:

1. **Add dependencies** — `github.com/charmbracelet/bubbletea` + `bubbles` to `go.mod`.
2. **Create `internal/tui/` package** — Lives at the Feature/Orchestration layer. Must not be imported by Platform/Core packages.
3. **Migrate prompt selection** — Replace `internal/prompt/` arrow-key selection with a Bubble Tea `list` model for richer interaction (filtering, search).
4. **Extend `mush worker start`** — Add a multi-step wizard combining habitat, queue, harness, and bundle selection into a single TUI flow.
5. **Preserve `internal/harness/` status bar** — The current ANSI scroll region approach works well for the watch-mode use case and does not need framework migration.
