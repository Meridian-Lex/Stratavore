#!/bin/bash
# Database migration script for Stratavore

set -euo pipefail

DIRECTION="${1:-up}"
MIGRATIONS_DIR="migrations/postgres"

# Database connection
DB_HOST="${STRATAVORE_DB_HOST:-localhost}"
DB_PORT="${STRATAVORE_DB_PORT:-5432}"
DB_NAME="${STRATAVORE_DB_NAME:-stratavore_state}"
DB_USER="${STRATAVORE_DB_USER:-stratavore}"
DB_PASSWORD="${STRATAVORE_DB_PASSWORD:-stratavore_password}"

PGPASSWORD="$DB_PASSWORD"
export PGPASSWORD

psql_cmd() {
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" "$@"
}

echo "==> Running migrations ($DIRECTION)"
echo " Database: $DB_NAME"
echo " Host: $DB_HOST:$DB_PORT"
echo ""

if [ "$DIRECTION" = "up" ]; then
    # Apply migrations in order
    for migration in "$MIGRATIONS_DIR"/*.up.sql; do
        if [ -f "$migration" ]; then
            echo "Applying: $(basename "$migration")"
            if psql_cmd -f "$migration"; then
                echo " [OK] Success"
            else
                echo " [FAIL] Failed"
                exit 1
            fi
        fi
    done
elif [ "$DIRECTION" = "down" ]; then
    # Rollback migrations in reverse order
    for migration in $(ls -r "$MIGRATIONS_DIR"/*.down.sql); do
        if [ -f "$migration" ]; then
            echo "Rolling back: $(basename "$migration")"
            if psql_cmd -f "$migration"; then
                echo " [OK] Success"
            else
                echo " [FAIL] Failed"
                exit 1
            fi
        fi
    done
else
    echo "ERROR: Unknown direction '$DIRECTION'"
    echo "Usage: $0 [up|down]"
    exit 1
fi

echo ""
echo "[OK] Migrations complete"
