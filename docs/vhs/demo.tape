# Mush CLI — Hero Demo
# Run from repo root: vhs docs/vhs/demo.tape
# Generates: docs/vhs/gif/demo.gif

Output docs/vhs/gif/demo.gif

Source docs/vhs/common.tape

Require mush

# Launch the interactive TUI
Type "mush"
Enter
Sleep 2s

# Navigate the home menu
Sleep 1s
Down
Sleep 800ms
Down
Sleep 800ms
Down
Sleep 800ms
Down
Sleep 800ms

# Back to top
Up
Sleep 800ms
Up
Sleep 800ms
Up
Sleep 800ms
Up
Sleep 1s

# Enter bundle load screen via hotkey
Type "r"
Sleep 1.5s

# Type a bundle slug
Type "acme/code-review"
Sleep 1.5s

# Switch to harness panel
Tab
Sleep 1s

# Navigate harnesses
Down
Sleep 800ms
Up
Sleep 1s

# Back to home
Escape
Sleep 1.5s

# Quit
Type "q"
Sleep 1s
