# tea-dash demo — records the REAL binary running on built-in mock data.
# Regenerate from the repo root:  make build && vhs docs/vhs/demo.tape
# The GIF is the README hero; the MP4 is uploaded as a release asset.

Output docs/demo.gif
Output docs/demo.mp4

Set Shell "bash"
Set FontSize 15
Set Width 1280
Set Height 720
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 70ms

# Launch off-screen so the video opens directly on the dashboard.
Hide
Type "./bin/tea-dash --mock"
Enter
Sleep 3s
Show

# Hero: framed PR view, preview follows the selection.
Sleep 2.5s
Type "j"
Sleep 900ms
Type "j"
Sleep 900ms
Type "k"
Sleep 1.2s

# Drill into the preview (enter focuses it), scroll, back out.
Enter
Sleep 1.2s
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "d"
Sleep 1.4s
Escape
Sleep 1s

# Numbered view jumps: Issues, Inbox, CI, Branches, back to Pulls.
Type "2"
Sleep 2.2s
Type "3"
Sleep 2.2s
Type "4"
Sleep 2.2s
Type "5"
Sleep 1.8s
Type "1"
Sleep 1.6s

# Command palette: fuzzy-find "merge", run it, pick a strategy.
# The success toast appears and the merged PR drops off the list.
Type "j"
Sleep 900ms
Type ":"
Sleep 1.4s
Type "merge"
Sleep 2.4s
Enter
Sleep 2.2s
Enter
Sleep 3s

# Full help overlay, generated from the live keymap.
Type "?"
Sleep 3.2s
Escape
Sleep 2s

# End on the dashboard; quit off-screen.
Hide
Type "q"
