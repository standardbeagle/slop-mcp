---
sidebar_position: 5
title: Event Monitoring
description: Stream events from git hooks, builds, CI, and MCP servers into Claude Code's Monitor tool using slop-mcp monitor and message commands.
keywords: [monitor, events, Claude Code, git hooks, CI, build watch, file watcher, notifications]
---

# Event Monitoring

slop-mcp provides two commands that turn any event source into [Claude Code Monitor](https://docs.claude.ai/en/docs/claude-code/cli-usage#monitor) notifications:

- **`slop-mcp monitor`** — watches for events and prints them to stdout
- **`slop-mcp message`** — sends a one-line event to a running monitor

Together they let you pipe git hooks, build output, test results, CI status, file changes, and MCP polling into Claude Code's notification stream.

## How It Works

```
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│  git hook    │────▶│              │     │                  │
│  build script│────▶│  slop-mcp    │────▶│  Claude Code     │
│  CI webhook  │────▶│  monitor     │     │  Monitor tool    │
│  file watcher│────▶│  (stdout)    │     │  (notifications) │
│  SLOP script │────▶│              │     │                  │
└──────────────┘     └──────────────┘     └──────────────────┘
       │                                          │
   slop-mcp message "text"              Each line = one notification
```

The monitor writes each event as one line to stdout. Claude Code's Monitor tool reads stdout and delivers each line as a notification in your conversation.

## Quick Start

Start a monitor and send it events:

```bash
# Terminal 1: start monitor
slop-mcp monitor

# Terminal 2: send events
slop-mcp message "build started"
slop-mcp message "build succeeded in 3.2s"
```

With Claude Code:

```javascript
Monitor({
  command: "slop-mcp monitor",
  description: "build events",
  persistent: true
})
```

## Event Source Patterns

### Git Hooks

```bash
# .git/hooks/post-commit
#!/bin/sh
MSG=$(git log -1 --pretty=format:'%s')
HASH=$(git log -1 --pretty=format:'%h')
slop-mcp message "commit $HASH: $MSG"
```

```bash
# .git/hooks/pre-push
#!/bin/sh
BRANCH=$(git rev-parse --abbrev-ref HEAD)
COUNT=$(git log --oneline @{u}..HEAD 2>/dev/null | wc -l)
slop-mcp message "pushing $COUNT commits on $BRANCH"
```

### Build Commands

Chain `slop-mcp message` with any build tool:

```bash
# Inline
make build && slop-mcp message "build ok" || slop-mcp message "build failed"

# Wrapper script
#!/bin/bash
CMD="$*"
slop-mcp message "build: $CMD"
if eval "$CMD" 2>&1; then
    slop-mcp message "build succeeded"
else
    ERROR=$(eval "$CMD" 2>&1 | tail -3)
    slop-mcp message "build failed: $ERROR"
fi
```

### Test Runners

```bash
#!/bin/bash
CMD="$*"
slop-mcp message "tests: running"
OUTPUT=$(eval "$CMD" 2>&1)
if [ $? -eq 0 ]; then
    SUMMARY=$(echo "$OUTPUT" | grep -E "^(ok|PASS|passed)" | tail -3)
    slop-mcp message "tests passed: $SUMMARY"
else
    FAILURES=$(echo "$OUTPUT" | grep -E "FAIL|ERROR" | head -3)
    slop-mcp message "tests failed: $FAILURES"
fi
```

### File Watchers

```bash
# inotifywait (Linux)
inotifywait -m -r -e modify --format '%w%f' src/ | \
    grep --line-buffered '\.go$' | \
    while read FILE; do
        slop-mcp message "modified: $FILE"
    done

# fswatch (macOS)
fswatch -r src/ | while read FILE; do
    slop-mcp message "modified: $FILE"
done
```

### CI/CD Webhooks

```bash
# Receive webhook, forward to monitor
while read LINE; do
    STATUS=$(echo "$LINE" | jq -r '.status')
    REF=$(echo "$LINE" | jq -r '.ref')
    slop-mcp message "ci: $REF $STATUS"
done
```

## SLOP Script Monitors

For polling MCP servers, use a SLOP script with `slop-mcp monitor`:

```bash
slop-mcp monitor watch-deploy.slop
```

### Delta Detection with `changed()`

The `changed(key, value)` builtin returns `true` only when the value differs from the last call with the same key:

```python
# watch-health.slop
for _ in range(999999999):
    health = myapi.get_health()
    if changed("health", health):
        print("health changed: " + str(health))
    sleep(30000)
```

This prevents flooding the monitor with unchanged state on every poll cycle.

### Persistent State with `mem_save`/`mem_load`

For state that survives monitor restarts, use SLOP's persistent memory:

```python
# watch-deploys.slop — remembers last seen deploy across restarts
last_id = mem_load("monitor", "last_deploy_id", 0)

for _ in range(999999999):
    deploy = ci.get_latest_deploy()
    if deploy["id"] != last_id:
        print("new deploy: " + str(deploy["version"]))
        last_id = deploy["id"]
        mem_save("monitor", "last_deploy_id", last_id)
    sleep(60000)
```

### Multi-Source Polling

```python
# watch-all.slop — poll multiple MCPs in one script
for _ in range(999999999):
    issues = github.list_issues(state: "open")
    if changed("issues", len(issues)):
        print("open issues: " + str(len(issues)))

    health = api.get_health()
    if changed("health", health["status"]):
        print("api health: " + str(health["status"]))

    sleep(30000)
```

## Combining Sources

The monitor watches for both script output and `slop-mcp message` events simultaneously. Run a SLOP script for MCP polling while also receiving messages from git hooks and build scripts:

```bash
# SLOP script polls MCPs, message command receives shell events
slop-mcp monitor watch-health.slop &

# Git hooks, build commands, etc. also send to the same monitor
make build && slop-mcp message "build ok"
```

## Claude Code Integration

### Basic Monitor

```javascript
// Watch for any events
Monitor({
  command: "slop-mcp monitor",
  description: "dev events",
  persistent: true
})
```

### Monitor with Script

```javascript
// Poll MCPs for changes
Monitor({
  command: "slop-mcp monitor watch-health.slop",
  description: "service health",
  persistent: true
})
```

### Timed Monitor

```javascript
// Watch for 10 minutes
Monitor({
  command: "slop-mcp monitor --timeout=600",
  description: "build events",
  timeout_ms: 660000
})
```
