#!/bin/bash
# Sync V2 PROJECT-MAP.md → V3 projects table (idempotent)
#
# Usage:
#   ./projects-sync.sh [v2-dir] [stratavore-migrate-binary]
#
# Defaults:
#   v2-dir: /home/meridian/meridian-home/lex-internal/state
#   binary: /usr/local/bin/stratavore-migrate

set -euo pipefail

V2_DIR="${1:-/home/meridian/meridian-home/lex-internal/state}"
STRATAVORE_MIGRATE="${2:-/usr/local/bin/stratavore-migrate}"

echo "=== Stratavore V2→V3 Projects Sync ==="
echo "V2 Directory: $V2_DIR"
echo "Binary: $STRATAVORE_MIGRATE"
echo ""

"$STRATAVORE_MIGRATE" sync \
    --v2-dir="$V2_DIR" \
    --type=projects

echo ""
echo "Projects sync complete."
