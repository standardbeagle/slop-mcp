#!/bin/bash
#
# Release script for slop-mcp
#
# This script ensures all version references are in sync before creating a release.
# It updates server.go, validates CHANGELOG.md, runs tests, and creates the GitHub release.
#
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh 0.9.0
#
# The script will:
# 1. Validate version format (semver without 'v' prefix)
# 2. Update serverVersion in internal/server/server.go
# 3. Validate CHANGELOG.md has an entry for this version
# 4. Run build and tests
# 5. Commit changes (if any)
# 6. Create and push git tag
# 7. Create GitHub release (triggers CI for npm/PyPI publish)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Validate arguments
if [ -z "$1" ]; then
    log_error "Usage: $0 <version>"
    log_error "Example: $0 0.9.0"
    exit 1
fi

VERSION="$1"

# Strip 'v' prefix if provided
VERSION="${VERSION#v}"

# Validate semver format
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    log_error "Invalid version format: $VERSION"
    log_error "Expected semver format: X.Y.Z or X.Y.Z-suffix"
    exit 1
fi

TAG="v$VERSION"

log_info "Preparing release $TAG"

# Check we're on main branch
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ]; then
    log_warn "Not on main branch (currently on $BRANCH)"
    read -p "Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Check for uncommitted changes (excluding the files we'll modify)
if ! git diff --quiet --exit-code -- ':!internal/server/server.go' ':!CHANGELOG.md'; then
    log_error "Uncommitted changes detected. Please commit or stash them first."
    git status --short
    exit 1
fi

# Check tag doesn't already exist
if git rev-parse "$TAG" >/dev/null 2>&1; then
    log_error "Tag $TAG already exists"
    exit 1
fi

# === Version sync checks ===

log_info "Checking version references..."

# 1. Update serverVersion in server.go
SERVER_GO="internal/server/server.go"
CURRENT_SERVER_VERSION=$(grep 'serverVersion = ' "$SERVER_GO" | sed 's/.*"\(.*\)".*/\1/')

if [ "$CURRENT_SERVER_VERSION" != "$VERSION" ]; then
    log_info "Updating $SERVER_GO: $CURRENT_SERVER_VERSION -> $VERSION"
    sed -i "s/serverVersion = \".*\"/serverVersion = \"$VERSION\"/" "$SERVER_GO"
else
    log_info "$SERVER_GO already at version $VERSION"
fi

# 2. Check CHANGELOG.md has entry for this version
CHANGELOG="CHANGELOG.md"
if ! grep -q "## \[$VERSION\]" "$CHANGELOG"; then
    log_error "CHANGELOG.md does not have an entry for version $VERSION"
    log_error "Please add a section: ## [$VERSION] - $(date +%Y-%m-%d)"
    exit 1
fi
log_info "CHANGELOG.md has entry for $VERSION"

# 3. Validate npm/package.json and pyproject.toml exist (versions set by CI)
if [ ! -f "npm/package.json" ]; then
    log_error "npm/package.json not found"
    exit 1
fi
if [ ! -f "pyproject.toml" ]; then
    log_error "pyproject.toml not found"
    exit 1
fi
log_info "Package files present (versions set by CI at release time)"

# === Build and test ===

log_info "Building..."
if ! go build -tags mcp_go_client_oauth ./...; then
    log_error "Build failed"
    exit 1
fi

log_info "Running tests..."
if ! go test -short -tags mcp_go_client_oauth ./...; then
    log_error "Tests failed"
    exit 1
fi

log_info "Build and tests passed"

# === Commit if changes were made ===

if ! git diff --quiet --exit-code; then
    log_info "Committing version updates..."
    git add internal/server/server.go
    git commit -m "chore: update serverVersion to $VERSION for release

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
fi

# === Create and push tag ===

log_info "Creating tag $TAG..."
git tag "$TAG"

log_info "Pushing to origin..."
git push origin main
git push origin "$TAG"

# === Create GitHub release ===

log_info "Creating GitHub release..."

# Extract release notes from CHANGELOG
# Find the section for this version and extract until the next version header
RELEASE_NOTES=$(awk "/^## \[$VERSION\]/,/^## \[/{if(/^## \[/ && !/^## \[$VERSION\]/)exit; print}" "$CHANGELOG" | tail -n +2)

if [ -z "$RELEASE_NOTES" ]; then
    log_warn "Could not extract release notes from CHANGELOG, using default"
    RELEASE_NOTES="See CHANGELOG.md for details."
fi

gh release create "$TAG" \
    --title "$TAG" \
    --notes "$RELEASE_NOTES"

log_info "Release $TAG created successfully!"
log_info "GitHub Actions will now build and publish to npm and PyPI"
log_info "Monitor progress at: https://github.com/standardbeagle/slop-mcp/actions"
