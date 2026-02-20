#!/bin/bash
# Full V2→V3 sync (all data types)
#
# Usage:
#   ./full-sync.sh [v2-dir] [stratavore-migrate-binary]
#
# Defaults:
#   v2-dir: /home/meridian/meridian-home/lex-internal/state
#   binary: /usr/local/bin/stratavore-migrate

set -euo pipefail

V2_DIR="${1:-/home/meridian/meridian-home/lex-internal/state}"
STRATAVORE_MIGRATE="${2:-/usr/local/bin/stratavore-migrate}"

echo "=== Stratavore V2→V3 Full Sync ==="
echo "V2 Directory: $V2_DIR"
echo "Binary: $STRATAVORE_MIGRATE"
echo ""
echo "Syncing all data types (projects, sessions, config, rank)..."
echo ""

START_TIME=$(date +%s)

"$STRATAVORE_MIGRATE" sync \
    --v2-dir="$V2_DIR" \
    --type=all

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "Full sync complete in ${DURATION}s"
echo "═══════════════════════════════════════════════════════════"
