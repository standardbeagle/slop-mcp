#!/bin/bash
# Hook any build command into slop-mcp monitor
# Usage: ./build-watch.sh make build
#        ./build-watch.sh npm run build
#        ./build-watch.sh cargo build

CMD="$*"
slop-mcp message "build: $CMD"

if eval "$CMD" 2>&1; then
    slop-mcp message "build succeeded: $CMD"
else
    EXIT=$?
    # Capture last 3 lines of error output for context
    ERROR=$(eval "$CMD" 2>&1 | tail -3)
    slop-mcp message "build failed ($EXIT): $ERROR"
fi
