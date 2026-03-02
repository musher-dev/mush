# TUI Navigation Design

> Design document for the interactive TUI navigation mode activated by `mush --interactive`.

## Vision

When a developer types bare `mush` with the interactive toggle enabled, they see a visual navigation screen instead of help text. The TUI provides quick access to common workflows — loading a bundle, exploring the hub, or entering worker mode — without memorizing subcommand names.

The TUI is an **alternative entry point**, not a replacement. All functionality remains accessible via standard CLI subcommands. The TUI is opt-in (default off) and suppressed automatically in non-TTY, CI, JSON, or quiet contexts.

## Information Architecture

### Screen Hierarchy

```
Home
├── Bundle Quick-Load    → text input for slug, version resolution, load
├── Recent Bundles       → list of recently loaded bundles, one-key reload
├── Explore Hub          → search/browse published bundles (API-backed)
└── Worker Mode          → habitat + queue selection, harness options, start
```

### Home Screen

The home screen is the root of the TUI. It shows:

- **Banner** — Mush version, workspace name (if linked).
- **Quick actions** — Keyboard shortcuts for each workflow:
  - `b` — Bundle quick-load (text input for slug)
  - `r` — Recent bundles (list selection)
  - `e` — Explore hub (search/browse)
  - `w` — Worker mode (start flow)
- **Status bar** — Auth status, linked workspace, connectivity indicator.

### Bundle Screen

Activated from home via `b` or "Bundle Quick-Load":

1. **Text input** — Type a bundle slug (with autocomplete from recent/hub).
2. **Version resolution** — Show resolved version, confirm or pick alternative.
3. **Load** — Execute `bundle load` flow, show progress, return to home.

### Recent Bundles Screen

Activated from home via `r`:

- **List** — Recently loaded bundles (from installed.json / cache).
- **Actions** — Enter to reload, `d` for details, `u` to uninstall.

### Worker Entry Screen

Activated from home via `w`:

1. **Habitat selector** — List linked habitats, arrow-key selection.
2. **Queue selector** — Filter queues for selected habitat.
3. **Harness options** — Claude vs Bash, permission flags.
4. **Confirmation** — Summary of selections, Enter to start.
5. **Handoff** — Launches `worker start` with selected options.

## Component Mapping

| UI Element | Bubbles Component | Notes |
|------------|-------------------|-------|
| Home menu | `list.Model` | Vertical list with descriptions |
| Bundle slug input | `textinput.Model` | With placeholder and validation |
| Recent bundles | `list.Model` | Filterable list with metadata |
| Habitat/queue selectors | `list.Model` | Single-select with API-backed items |
| Status bar | Custom `lipgloss` render | Static layout, no interaction |
| Progress indicators | `spinner.Model` | During API calls and downloads |
| Confirmation dialogs | Custom key handler | y/n prompt before destructive actions |

## Architecture

### Package Layout

```
internal/tui/nav/
├── nav.go          # Run() entry point, tea.Program setup
├── model.go        # Root model, screen routing
├── home.go         # Home screen model
├── bundle.go       # Bundle quick-load flow
├── recent.go       # Recent bundles list
├── worker.go       # Worker entry flow
├── styles.go       # Shared lipgloss styles
└── keys.go         # Key bindings
```

### Integration Points

- **`internal/config`** — Read workspace, auth status, recent bundles.
- **`internal/client`** — API calls for hub search, habitat/queue listing.
- **`internal/bundle`** — Bundle resolution, cache lookup, load/install.
- **`internal/worker`** — Worker start with selected options.
- **`internal/output`** — TTY detection for suppression checks (via `cmd/mush` only).

The `nav` package lives at the Feature/Orchestration layer and may import any internal package except `cmd/mush`.

### Data Flow

```
cmd/mush/main.go
  └── shouldShowTUI() → true
        └── nav.Run(ctx)
              └── tea.NewProgram(model).Run()
                    └── Model.Update() dispatches to sub-screens
                          └── Sub-screens call internal packages for data
```

## Phase Roadmap

### Phase 1: Toggle + Stub + Design Doc (this PR)

- `--interactive` flag, `MUSH_INTERACTIVE` env var, `interactive` config key.
- Suppression rules (JSON, quiet, no-input, non-TTY).
- Hello-world Bubbletea stub proving the pipeline works.
- This design document.

### Phase 2: Home Screen with Static Layout

- Home screen with banner, quick-action list, status bar.
- Navigation between home and placeholder sub-screens.
- Lipgloss styling matching existing CLI aesthetic.

### Phase 3: Bundle Loading Flow

- Text input for bundle slug with validation.
- Version resolution via API.
- Bundle load execution with progress spinner.
- Return to home on completion.

### Phase 4: Worker Entry Flow

- Habitat list from API (or cached).
- Queue selection filtered by habitat.
- Harness option selection.
- Confirmation screen and handoff to `worker start`.

### Phase 5: Hub Exploration

- Search input with debounced API queries.
- Paginated bundle list with metadata.
- Bundle detail view with install action.

### Phase 6: Flip Default to `true`

- Flip `interactive` default from `false` to `true`.
- Update documentation and onboarding flow.
- Ensure all suppression rules work correctly at scale.
