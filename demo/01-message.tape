# Demo 1: slop-mcp message — pipe any event source into Claude Code Monitor

Output demo/01-message.gif

Set Shell "bash"
Set FontSize 16
Set Width 960
Set Height 460
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 35ms
Set WindowBar Colorful
Set WindowBarSize 40
Set CursorBlink false

Type "# slop-mcp message — one command turns anything into a Monitor event"
Enter
Sleep 2s

# Start monitor
Type "slop-mcp monitor --timeout=18 &"
Enter
Sleep 800ms

# Git hook pattern
Type "# git hook → monitor"
Enter
Sleep 500ms
Type "slop-mcp message 'commit a1b2c3f: fix auth token refresh logic'"
Enter
Sleep 1.5s

# Build chain pattern
Type "# build command chain → monitor"
Enter
Sleep 500ms
Type "make build 2>&1 && slop-mcp message 'build ok' || slop-mcp message 'build failed'"
Enter
Sleep 2s

# Test runner pattern
Type "# test output → monitor"
Enter
Sleep 500ms
Type "slop-mcp message 'tests: 42 passed, 1 failed — TestAuthRefresh'"
Enter
Sleep 1.5s

# Deploy pattern
Type "# deploy webhook → monitor"
Enter
Sleep 500ms
Type "slop-mcp message 'deploy v0.14.0 → production (3/3 healthy)'"
Enter
Sleep 1.5s

# Error alert
Type "# error alert → monitor"
Enter
Sleep 500ms
Type "slop-mcp message 'ALERT: error rate 4.2% on /api/auth (threshold 1%)'"
Enter
Sleep 4s

Type ""
Enter
Sleep 500ms
Type "# Any shell command, hook, or cron job can send Monitor events"
Enter
Sleep 3s
