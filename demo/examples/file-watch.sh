#!/bin/bash
# Watch for file changes → slop-mcp monitor
# Usage: ./file-watch.sh src/ "*.go"

DIR="${1:-.}"
PATTERN="${2:-*}"

slop-mcp message "watching $DIR for $PATTERN changes"
inotifywait -m -r -e modify,create,delete --format '%e %w%f' "$DIR" 2>/dev/null | \
    grep --line-buffered "$PATTERN" | \
    while read EVENT FILE; do
        slop-mcp message "file $EVENT: $FILE"
    done
