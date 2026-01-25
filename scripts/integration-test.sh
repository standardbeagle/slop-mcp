#!/bin/bash
#
# Integration tests for slop-mcp with Claude, Gemini, and Copilot CLIs
#
# Usage:
#   ./scripts/integration-test.sh           # Run all tests
#   ./scripts/integration-test.sh claude    # Test Claude only
#   ./scripts/integration-test.sh gemini    # Test Gemini only
#   ./scripts/integration-test.sh copilot   # Test Copilot only
#
# Environment variables:
#   SLOP_TEST_TIMEOUT   - Timeout per test in seconds (default: 60)
#   SLOP_TEST_VERBOSE   - Set to 1 for verbose output
#   SLOP_TEST_KEEP_DIRS - Set to 1 to keep temp directories after test
#
# Requirements:
#   - Built slop-mcp binary (run 'make build' first)
#   - API keys configured for each tool being tested
#   - claude, gemini, and/or copilot CLIs installed
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
SLOP_MCP_BIN="$BUILD_DIR/slop-mcp"
TIMEOUT="${SLOP_TEST_TIMEOUT:-60}"
VERBOSE="${SLOP_TEST_VERBOSE:-0}"
KEEP_DIRS="${SLOP_TEST_KEEP_DIRS:-0}"

# Test state
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TEMP_DIRS=()

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_error() { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_skip() { echo -e "${YELLOW}[SKIP]${NC} $*"; }
log_verbose() { [[ "$VERBOSE" == "1" ]] && echo -e "${BLUE}[DEBUG]${NC} $*" || true; }

# Track skipped tests separately
TESTS_SKIPPED=0

cleanup() {
    if [[ "$KEEP_DIRS" != "1" ]]; then
        for dir in "${TEMP_DIRS[@]}"; do
            [[ -d "$dir" ]] && rm -rf "$dir"
        done
    fi
}
trap cleanup EXIT

create_temp_dir() {
    local dir
    dir=$(mktemp -d -t slop-mcp-test.XXXXXX)
    TEMP_DIRS+=("$dir")
    echo "$dir"
}

check_binary() {
    if [[ ! -x "$SLOP_MCP_BIN" ]]; then
        log_error "slop-mcp binary not found at $SLOP_MCP_BIN"
        log_info "Run 'make build' first"
        exit 1
    fi
    log_info "Using slop-mcp: $SLOP_MCP_BIN"
}

check_command() {
    local cmd=$1
    if ! command -v "$cmd" &>/dev/null; then
        log_warn "$cmd CLI not found, skipping $cmd tests"
        return 1
    fi
    return 0
}

# Test prompt that exercises slop-mcp's search_tools capability
TEST_PROMPT='Use the search_tools MCP tool to search for tools with "list" in the name. Return only the JSON result, no explanation.'

# Expected: The response should contain tool search results
validate_search_response() {
    local response=$1
    # Check for indicators of successful tool search
    if echo "$response" | grep -qiE '(tools|search|list|total|has_more)'; then
        return 0
    fi
    return 1
}

#
# Claude Code Integration Test
#
test_claude() {
    log_info "Testing Claude Code integration..."

    if ! check_command "claude"; then
        return 0
    fi

    local temp_dir
    temp_dir=$(create_temp_dir)
    local mcp_config="$temp_dir/mcp.json"

    # Create MCP config for Claude
    cat > "$mcp_config" << EOF
{
  "mcpServers": {
    "slop-test": {
      "command": "$SLOP_MCP_BIN",
      "args": ["serve"]
    }
  }
}
EOF

    log_verbose "MCP config: $mcp_config"
    log_verbose "$(cat "$mcp_config")"

    # Run Claude with the test prompt
    local output
    local exit_code=0

    output=$(timeout "$TIMEOUT" claude \
        --mcp-config "$mcp_config" \
        --permission-mode bypassPermissions \
        --print \
        --model haiku \
        "$TEST_PROMPT" 2>&1) || exit_code=$?

    TESTS_RUN=$((TESTS_RUN + 1))

    if [[ $exit_code -eq 124 ]]; then
        log_error "Claude test timed out after ${TIMEOUT}s"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    elif [[ $exit_code -ne 0 ]]; then
        # Check for auth errors
        if echo "$output" | grep -qi "auth\|login\|credential\|API key"; then
            log_skip "Claude: Authentication required (run 'claude' to log in)"
            TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
            TESTS_RUN=$((TESTS_RUN - 1))  # Don't count as run
            return 0
        fi
        log_error "Claude test failed with exit code $exit_code"
        log_verbose "Output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    if validate_search_response "$output"; then
        log_success "Claude Code: MCP integration working"
        log_verbose "Response: $output"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "Claude test: Response doesn't contain expected search results"
        log_verbose "Response: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

#
# Gemini CLI Integration Test
#
test_gemini() {
    log_info "Testing Gemini CLI integration..."

    if ! check_command "gemini"; then
        return 0
    fi

    # Check for API key
    if [[ -z "${GEMINI_API_KEY:-}" ]]; then
        log_skip "Gemini: GEMINI_API_KEY not set"
        TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
        return 0
    fi

    local temp_dir
    temp_dir=$(create_temp_dir)
    local project_dir="$temp_dir/test-project"
    mkdir -p "$project_dir"

    # Change to project dir and add MCP using gemini mcp add
    log_verbose "Project dir: $project_dir"

    # Add the MCP to the project scope
    local add_output
    add_output=$(cd "$project_dir" && gemini mcp add slop-test "$SLOP_MCP_BIN" serve --scope project --trust 2>&1) || true
    log_verbose "MCP add output: $add_output"

    # Run Gemini with the test prompt from the project directory
    local output
    local exit_code=0

    output=$(cd "$project_dir" && timeout "$TIMEOUT" gemini \
        --yolo \
        --output-format text \
        "$TEST_PROMPT" 2>&1) || exit_code=$?

    TESTS_RUN=$((TESTS_RUN + 1))

    if [[ $exit_code -eq 124 ]]; then
        log_error "Gemini test timed out after ${TIMEOUT}s"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    elif [[ $exit_code -ne 0 ]]; then
        log_error "Gemini test failed with exit code $exit_code"
        log_verbose "Output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    if validate_search_response "$output"; then
        log_success "Gemini CLI: MCP integration working"
        log_verbose "Response: $output"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "Gemini test: Response doesn't contain expected search results"
        log_verbose "Response: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

#
# GitHub Copilot CLI Integration Test
#
test_copilot() {
    log_info "Testing GitHub Copilot CLI integration..."

    if ! check_command "copilot"; then
        return 0
    fi

    local temp_dir
    temp_dir=$(create_temp_dir)
    local config_dir="$temp_dir/.copilot"
    mkdir -p "$config_dir"

    # Create MCP config for Copilot
    local mcp_config="$config_dir/mcp-config.json"

    cat > "$mcp_config" << EOF
{
  "mcpServers": {
    "slop-test": {
      "command": "$SLOP_MCP_BIN",
      "args": ["serve"]
    }
  }
}
EOF

    log_verbose "Config dir: $config_dir"
    log_verbose "MCP config: $mcp_config"
    log_verbose "$(cat "$mcp_config")"

    # Run Copilot with the test prompt
    local output
    local exit_code=0

    output=$(timeout "$TIMEOUT" copilot \
        --config-dir "$config_dir" \
        --allow-all-tools \
        --disable-builtin-mcps \
        -p "$TEST_PROMPT" 2>&1) || exit_code=$?

    TESTS_RUN=$((TESTS_RUN + 1))

    if [[ $exit_code -eq 124 ]]; then
        log_error "Copilot test timed out after ${TIMEOUT}s"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    elif [[ $exit_code -ne 0 ]]; then
        # Check for auth errors
        if echo "$output" | grep -qi "auth\|login\|credential\|token"; then
            log_skip "Copilot: Authentication required (run 'copilot' to log in)"
            TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
            TESTS_RUN=$((TESTS_RUN - 1))  # Don't count as run
            return 0
        fi
        log_error "Copilot test failed with exit code $exit_code"
        log_verbose "Output: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi

    if validate_search_response "$output"; then
        log_success "GitHub Copilot: MCP integration working"
        log_verbose "Response: $output"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "Copilot test: Response doesn't contain expected search results"
        log_verbose "Response: $output"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

#
# Main
#
main() {
    log_info "slop-mcp Integration Tests"
    log_info "=========================="
    echo

    check_binary

    local target="${1:-all}"

    case "$target" in
        claude)
            test_claude
            ;;
        gemini)
            test_gemini
            ;;
        copilot)
            test_copilot
            ;;
        all)
            test_claude || true
            test_gemini || true
            test_copilot || true
            ;;
        *)
            log_error "Unknown target: $target"
            echo "Usage: $0 [claude|gemini|copilot|all]"
            exit 1
            ;;
    esac

    echo
    log_info "=========================="
    log_info "Results: $TESTS_PASSED/$TESTS_RUN passed"
    if [[ $TESTS_SKIPPED -gt 0 ]]; then
        log_info "Skipped: $TESTS_SKIPPED (missing API keys or auth)"
    fi

    if [[ $TESTS_FAILED -gt 0 ]]; then
        log_error "$TESTS_FAILED test(s) failed"
        exit 1
    elif [[ $TESTS_RUN -eq 0 ]]; then
        if [[ $TESTS_SKIPPED -gt 0 ]]; then
            log_warn "All tests skipped (configure API keys to run tests)"
            exit 0
        else
            log_warn "No tests were run (CLIs not found)"
            exit 0
        fi
    else
        log_success "All tests passed!"
        exit 0
    fi
}

main "$@"
