# Main demo: slop-mcp monitor + message — event streaming for Claude Code

Output demo/monitor-demo.gif

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

Type "# slop-mcp monitor — turn any event source into Claude Code notifications"
Enter
Sleep 2s

Type "slop-mcp monitor --timeout=20 &"
Enter
Sleep 800ms

# Git hook
Type "slop-mcp message 'commit a1b2c3f: fix auth token refresh'"
Enter
Sleep 1.2s

# Lint
Type "slop-mcp message 'lint: handlers.go:142 unused variable err'"
Enter
Sleep 1.2s

# Build
Type "make build 2>&1 && slop-mcp message 'build ok' || slop-mcp message 'build failed'"
Enter
Sleep 2s

# Tests
Type "slop-mcp message 'tests: 42 passed, 1 failed — TestAuthRefresh'"
Enter
Sleep 1.2s

# Fix + retest
Type "slop-mcp message 'tests: 43 passed in 1.2s'"
Enter
Sleep 1.2s

# Deploy
Type "slop-mcp message 'deploy v0.14.0 → staging (3/3 healthy)'"
Enter
Sleep 1.2s

# Alert
Type "slop-mcp message 'ALERT: error rate 4.2% on /api/auth (threshold 1%)'"
Enter
Sleep 3s

# Show hookability
Type ""
Enter
Type "# git hooks, build scripts, CI, cron, file watchers — all one command"
Enter
Sleep 1.5s
Type "# Monitor({ command: 'slop-mcp monitor', persistent: true })"
Enter
Sleep 3s
