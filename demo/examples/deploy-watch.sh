#!/bin/bash
# Poll a deployment endpoint → slop-mcp monitor
# Usage: ./deploy-watch.sh https://api.example.com/health

URL="${1:-http://localhost:8080/health}"
INTERVAL="${2:-30}"

slop-mcp message "deploy watch: polling $URL every ${INTERVAL}s"
PREV=""
while true; do
    STATUS=$(curl -sf "$URL" 2>/dev/null || echo "unreachable")
    if [ "$STATUS" != "$PREV" ]; then
        slop-mcp message "deploy status: $STATUS"
        PREV="$STATUS"
    fi
    sleep "$INTERVAL"
done
