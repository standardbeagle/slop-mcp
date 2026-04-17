# Demo 2: Multiple event sources feeding one monitor

Output demo/02-multi-source.gif

Set Shell "bash"
Set FontSize 16
Set Width 960
Set Height 500
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 35ms
Set WindowBar Colorful
Set WindowBarSize 40
Set CursorBlink false

Type "# Multiple event sources → one Claude Code Monitor"
Enter
Sleep 2s

# Start monitor
Type "slop-mcp monitor --timeout=22 &"
Enter
Sleep 800ms

# Simulate a realistic dev workflow
Type "# Simulating a development workflow..."
Enter
Sleep 1s

# Developer saves a file
Type "slop-mcp message '[fs] modified: internal/server/handlers.go'"
Enter
Sleep 1s

# Linter runs automatically
Type "slop-mcp message '[lint] handlers.go:142 — unused variable err'"
Enter
Sleep 1.2s

# Developer fixes and saves again
Type "slop-mcp message '[fs] modified: internal/server/handlers.go'"
Enter
Sleep 800ms

# Tests kick off
Type "slop-mcp message '[test] running go test ./internal/server/...'"
Enter
Sleep 1.5s

Type "slop-mcp message '[test] FAIL TestHandleExecuteTool — expected 200 got 401'"
Enter
Sleep 1.5s

# Developer fixes the test
Type "slop-mcp message '[fs] modified: internal/server/handlers_test.go'"
Enter
Sleep 800ms

Type "slop-mcp message '[test] running go test ./internal/server/...'"
Enter
Sleep 1.2s

Type "slop-mcp message '[test] ok — 24 tests passed in 1.2s'"
Enter
Sleep 1s

# Developer commits
Type "slop-mcp message '[git] commit e4f5a6b: fix auth middleware token validation'"
Enter
Sleep 1s

# CI picks it up
Type "slop-mcp message '[ci] pipeline started — build + test + deploy'"
Enter
Sleep 1.2s

Type "slop-mcp message '[ci] build succeeded — linux/amd64 linux/arm64 darwin/arm64'"
Enter
Sleep 1s

Type "slop-mcp message '[ci] deploy v0.14.1 → staging (healthy)'"
Enter
Sleep 4s

Type ""
Enter
Type "# fs watch + lint + test + git + CI — all through slop-mcp message"
Enter
Sleep 3s
