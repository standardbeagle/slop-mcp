# Demo 3: SLOP script monitor with MCP polling + persistent memory

Output demo/03-slop-monitor.gif

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

Type "# SLOP scripts poll MCPs, detect changes, persist state"
Enter
Sleep 2s

# Show the script
Type "cat demo/monitor-demo.slop"
Enter
Sleep 3s

# Run it
Type "slop-mcp monitor --timeout=14 demo/monitor-demo.slop 2>/dev/null"
Enter
Sleep 16s

# Persistence demo
Type "# Persistent state survives restarts"
Enter
Sleep 1s

Type "cat demo/persist-demo.slop"
Enter
Sleep 3s

# First run
Type "slop-mcp monitor --timeout=10 demo/persist-demo.slop 2>/dev/null"
Enter
Sleep 12s

# Second run shows persistence
Type "# Run again — picks up where it left off"
Enter
Sleep 500ms

Type "slop-mcp monitor --timeout=10 demo/persist-demo.slop 2>/dev/null"
Enter
Sleep 12s

Type "# changed() for deltas, mem_save/mem_load for persistence"
Enter
Sleep 3s
