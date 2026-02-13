#!/usr/bin/env bash
# bump-version.sh – Update the project version in every location atomically.
#
# Usage:
#./scripts/bump-version.sh 1.5.0
# VERSION=1.5.0./scripts/bump-version.sh # alternative
#
# What it touches:
# VERSION (single source of truth)
# Makefile (VERSION?=...)
# build.ps1 ($VERSION = "...")
# build.bat (set VERSION=...)
# cmd/stratavored/main.go (Version = "...")
# cmd/stratavore/main.go (Version = "...")
# cmd/stratavore-agent/main.go (agent_version: "...")
# Dockerfile.builder header comment (informational)
# docker-compose.builder.yml LABEL (informational)
#
# The script is idempotent – running it twice with the same version is safe.
set -euo pipefail

# ── Resolve target version ────────────────────────────────────────────────────
NEW_VERSION="${1:-${VERSION:-}}"
if [[ -z "$NEW_VERSION" ]]; then
    echo "Usage: $0 <new-version> e.g. $0 1.5.0" >&2
    exit 1
fi

# Basic semver validation (major.minor.patch)
if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: version must be in semver format (e.g. 1.5.0), got: $NEW_VERSION" >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OLD_VERSION="$(cat "$ROOT/VERSION" | tr -d '[:space:]')"

if [[ "$NEW_VERSION" == "$OLD_VERSION" ]]; then
    echo "Already at version $NEW_VERSION – nothing to do."
    exit 0
fi

echo "Bumping $OLD_VERSION → $NEW_VERSION"

# Helper: sed in-place (portable macOS/Linux)
sedi() {
    if sed --version 2>/dev/null | grep -q GNU; then
        sed -i "$@"
    else
        sed -i '' "$@"
    fi
}

# ── 1. VERSION file ───────────────────────────────────────────────────────────
echo "$NEW_VERSION" > "$ROOT/VERSION"
echo " [OK] VERSION"

# ── 2. Makefile ───────────────────────────────────────────────────────────────
sedi "s|^VERSION?=.*|VERSION?=$NEW_VERSION|" "$ROOT/Makefile"
echo " [OK] Makefile"

# ── 3. build.ps1 ─────────────────────────────────────────────────────────────
sedi "s|\\\$VERSION = \".*\"|\$VERSION = \"$NEW_VERSION\"|" "$ROOT/build.ps1"
# Also update the banner line
sedi "s|Stratavore v[0-9.]* Windows Build\"|Stratavore v$NEW_VERSION Windows Build\"|" "$ROOT/build.ps1"
echo " [OK] build.ps1"

# ── 4. build.bat ─────────────────────────────────────────────────────────────
sedi "s|^set VERSION=.*|set VERSION=$NEW_VERSION|" "$ROOT/build.bat"
sedi "s|Stratavore v[0-9.]* Windows Build|Stratavore v$NEW_VERSION Windows Build|" "$ROOT/build.bat"
echo " [OK] build.bat"

# ── 5. cmd/stratavored/main.go ───────────────────────────────────────────────
sedi "s|Version = \"[0-9.]*\"|Version = \"$NEW_VERSION\"|" "$ROOT/cmd/stratavored/main.go"
echo " [OK] cmd/stratavored/main.go"

# ── 6. cmd/stratavore/main.go ────────────────────────────────────────────────
sedi "s|Version = \"[0-9.]*\"|Version = \"$NEW_VERSION\"|" "$ROOT/cmd/stratavore/main.go"
echo " [OK] cmd/stratavore/main.go"

# ── 7. cmd/stratavore-agent/main.go (agent_version in heartbeats) ────────────
# The agent embeds its own version in heartbeat JSON strings
sedi "s|\"agent_version\": \"[0-9.]*\"|\"agent_version\": \"$NEW_VERSION\"|g" "$ROOT/cmd/stratavore-agent/main.go"
echo " [OK] cmd/stratavore-agent/main.go"

# ── 8. Dockerfile.builder banner ─────────────────────────────────────────────
sedi "s|# Stratavore v[0-9.]* Docker|# Stratavore v$NEW_VERSION Docker|g" "$ROOT/Dockerfile.builder" 2>/dev/null || true
echo " [OK] Dockerfile.builder (banner, if present)"

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "Version bumped: $OLD_VERSION → $NEW_VERSION"
echo ""
echo "Files updated:"
echo " VERSION, Makefile, build.ps1, build.bat"
echo " cmd/stratavored/main.go, cmd/stratavore/main.go, cmd/stratavore-agent/main.go"
echo ""
echo "Next steps:"
echo " git add -A && git commit -m \"chore: bump version to v$NEW_VERSION\""
echo " git tag v$NEW_VERSION"
