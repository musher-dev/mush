# Mush Harness Rendering Options

## Current State

**The harness uses Option 3 (Scroll Region Approach)** with raw PTY passthrough and ANSI scroll regions. This provides:
- Full-fidelity Claude Code rendering (no terminal emulation layer)
- Real-time status bar updates in reserved top lines
- Direct keyboard passthrough to Claude's TUI

See `internal/harness/model.go` for the implementation.

---

## Historical Context

The original implementation attempted to use bubbleterm to embed Claude Code in a Bubble Tea TUI. This had rendering issues due to terminal emulator limitations:
- Unicode braille/box-drawing characters displayed incorrectly
- Some colors didn't render properly
- Advanced ANSI escape sequences not fully interpreted

After evaluation, the scroll region approach was chosen for its simplicity and rendering fidelity.

---

## Option 1: bubbleterm Approach (Deprecated)

Uses bubbleterm's terminal emulator to render Claude Code output.

**Pros:**
- Status bar visible during execution
- Real-time job status updates
- Window-in-window architecture

**Cons:**
- Terminal emulator limitations cause rendering issues
- No wide-character support

---

## Option 2: Direct Execution with Status Bookends

Use `tea.ExecProcess` to give Claude Code full terminal control, with status shown before/after.

```
┌─ MUSH HARNESS ─────────────────────────────────┐
│ Starting Claude Code... Press Ctrl+C to abort  │
│ Job: test-job-123 | Habitat: staging            │
└─────────────────────────────────────────────────┘

[Claude Code runs with full terminal control - perfect rendering]

┌─ MUSH HARNESS ─────────────────────────────────┐
│ Job completed: test-job-123 | Duration: 45s     │
│ Waiting for next job...                         │
└─────────────────────────────────────────────────┘
```

**Implementation:**
```go
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    case JobReceivedMsg:
        // Show pre-execution status, then exec Claude (interactive)
        cmd := tea.ExecProcess(exec.Command("claude"), nil)
        return m, cmd

    case tea.ExecProcessFinishedMsg:
        // Claude finished, show post-execution status
        m.statusBar.SetCompleted(msg.ExitCode)
        return m, nil
}
```

**Pros:**
- Perfect rendering (Claude has full terminal control)
- Simple implementation
- No terminal emulator complexity

**Cons:**
- No real-time status overlay during execution
- Status only visible between jobs

Note: this option is included for historical comparison. Mush's harness uses a PTY + scroll region so the status bar stays visible while Claude runs.

---

## Option 3: Scroll Region Approach (DECSTBM) ⭐ PROMISING

Use ANSI [scroll regions](https://ghostty.org/docs/vt/csi/decstbm) to reserve top lines for status while Claude renders below.

**How it works:**
1. Reserve top 2-3 lines for status bar
2. Set scroll region with `CSI t;b r` (e.g., `\x1b[4;999r` = lines 4 to bottom)
3. Position cursor in scroll region and run Claude via PTY
4. Periodically update status by: save cursor → move to line 1 → write status → restore cursor

**Key escape sequences:**
```
\x1b[4;999r     Set scroll region (lines 4 to bottom)
\x1b[s          Save cursor position
\x1b[u          Restore cursor position
\x1b[1;1H       Move cursor to row 1, col 1
\x1b[0m         Reset colors
```

**Implementation sketch:**
```go
func (m *RootModel) startWithScrollRegion() {
    // Initial setup
    fmt.Print("\x1b[1;1H")           // Move to top
    fmt.Print(m.renderStatusBar())   // Draw status
    fmt.Print("\x1b[4;999r")         // Set scroll region (line 4+)
    fmt.Print("\x1b[4;1H")           // Move cursor to line 4

    // Start Claude PTY - output goes to scroll region
    m.pty.Start()

    // Periodic status updates
    go func() {
        for range time.Tick(time.Second) {
            fmt.Print("\x1b[s")              // Save cursor
            fmt.Print("\x1b[1;1H")           // Move to status area
            fmt.Print(m.renderStatusBar())  // Update status
            fmt.Print("\x1b[u")              // Restore cursor
        }
    }()
}
```

**Pros:**
- Full-fidelity Claude rendering (no terminal emulation)
- Real-time status bar updates
- Uses standard terminal features (widely supported)
- No external library dependencies

**Cons:**
- More complex cursor/state management
- Status updates might cause visual flicker
- Need to handle resize events carefully

---

## Option 4: Alternative Terminal Emulators

Replace bubbleterm with a more mature VT emulator:

| Library | Description | Wide-char? |
|---------|-------------|------------|
| [hinshun/vt10x](https://pkg.go.dev/github.com/hinshun/vt10x) | VT10x emulator, influenced by st/rxvt/xterm | Unknown |
| [ActiveState/vt10x](https://pkg.go.dev/github.com/ActiveState/vt10x) | Fork with expect.Console support | 256-color ✓ |
| [jaguilar/vt100](https://pkg.go.dev/github.com/jaguilar/vt100) | Quick-and-dirty ANSI emulator | Limited |
| [micro-editor/terminal](https://pkg.go.dev/github.com/micro-editor/terminal) | Used by micro editor | Likely ✓ |

**Evaluation needed:** Test each library with Claude Code to see which handles Unicode/braille correctly.

---

## Option 5: External Status Channel

Show status outside the terminal:
- **System notifications** (libnotify/terminal-notifier)
- **Separate tmux pane** - Run status in adjacent pane
- **Web dashboard** - WebSocket updates to browser
- **Log file** with `tail -f`

---

## Option 6: Hybrid - tmux Integration

Use tmux for proper PTY multiplexing:
```bash
tmux new-session -d -s mush
tmux split-window -v -l 3  # Status pane (3 lines)
tmux select-pane -t 0      # Claude pane
```

Control tmux from Go using `tmux send-keys` and `tmux display-message`.

**Pros:**
- Battle-tested terminal multiplexing
- Full Unicode support
- Native scrollback

**Cons:**
- Requires tmux installed
- More complex setup
- Less portable

---

## Recommendation

**Try Option 3 (Scroll Region) first** - it offers the best balance of:
- Full-fidelity rendering
- Real-time status updates
- No external dependencies
- Uses standard ANSI sequences

If scroll regions cause issues (flicker, compatibility), fall back to **Option 2** (bookend status).

---

## References

- [ANSI Escape Codes](https://gist.github.com/fnky/458719343aabd01cfb17a3a4f7296797)
- [DECSTBM - Set Scroll Margins](https://ghostty.org/docs/vt/csi/decstbm)
- [bubbleterm source](https://pkg.go.dev/github.com/taigrr/bubbleterm)
- [Build your own Command Line with ANSI](https://www.lihaoyi.com/post/BuildyourownCommandLinewithANSIescapecodes.html)
