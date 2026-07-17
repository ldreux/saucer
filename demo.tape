# Run `task build` (or `go build -o bin/saucer .`) before recording this tape.
# Then: vhs demo.tape   -> produces demo.gif in the repo root.

Output demo.gif

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 780
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 80ms

Hide
Type "./bin/saucer"
Enter
Sleep 500ms
Show

Sleep 2s

# Cycle themes
Tab
Sleep 700ms
Tab
Sleep 700ms
Tab
Sleep 700ms
Tab
Sleep 700ms
Tab
Sleep 700ms

# Switch to Countdown
Type "c"
Sleep 1s
Space
Sleep 2s
Space

# Switch to Timer
Type "t"
Sleep 1s
Space
Sleep 2s
Space

# Switch to Clock
Type "k"
Sleep 2s

# Back to Pomodoro
Type "p"
Sleep 1s

# Ambient weather
Type "w"
Sleep 2s
Type "w"
Sleep 2s
Type "w"
Sleep 500ms

# Compact mode
Type "b"
Sleep 2s
Type "b"
Sleep 500ms

Type "q"
