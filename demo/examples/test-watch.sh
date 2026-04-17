#!/bin/bash
# Hook test runner output into slop-mcp monitor
# Usage: ./test-watch.sh go test ./...
#        ./test-watch.sh npm test
#        ./test-watch.sh pytest

CMD="$*"
slop-mcp message "tests: running $CMD"

OUTPUT=$(eval "$CMD" 2>&1)
EXIT=$?

if [ $EXIT -eq 0 ]; then
    # Extract summary line (works for go test, pytest, jest)
    SUMMARY=$(echo "$OUTPUT" | grep -E "^(ok|PASS|FAIL|passed|failed)" | tail -3)
    slop-mcp message "tests passed: $SUMMARY"
else
    FAILURES=$(echo "$OUTPUT" | grep -E "^(---|FAIL|ERROR|AssertionError)" | head -3)
    slop-mcp message "tests failed: $FAILURES"
fi
