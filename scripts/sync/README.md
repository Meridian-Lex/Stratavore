# Lex V2 → Stratavore V3 Sync Scripts

Idempotent synchronization scripts for ongoing V2→V3 parallel operation.

## Purpose

During the parallel operation phase (Phase 3 of migration), these scripts keep V3 synchronized with V2 state changes. All operations use UPSERT semantics — safe to run repeatedly.

## Scripts

| Script | Syncs | Idempotency |
|--------|-------|-------------|
| `projects-sync.sh` | PROJECT-MAP.md → projects table | INSERT ... ON CONFLICT UPDATE |
| `sessions-sync.sh` | time_sessions.jsonl → sessions table | INSERT ... ON CONFLICT UPDATE |
| `config-sync.sh` | LEX-CONFIG.yaml → token_budgets/quotas | UPDATE existing rows |
| `rank-sync.sh` | rank-status.jsonl → rank_tracking | Append-only (new events) |
| `full-sync.sh` | All of the above | Combines all syncs |

## Usage

### Manual Execution

```bash
# Sync specific data type
./scripts/sync/projects-sync.sh

# Sync with custom V2 directory
./scripts/sync/sessions-sync.sh /path/to/lex-internal/state

# Full sync with custom binary path
./scripts/sync/full-sync.sh /path/to/state /path/to/stratavore-migrate
```

### Scheduled Execution (Cron)

For ongoing V2→V3 synchronization during parallel operation:

```bash
# Edit crontab
crontab -e

# Add entry (sync every 10 minutes)
*/10 * * * * /home/meridian/meridian-home/projects/Stratavore/scripts/sync/full-sync.sh >> /var/log/stratavore-sync.log 2>&1
```

**Recommended sync interval**: 5-10 minutes during active development, hourly otherwise.

## Environment

Scripts require:
- `stratavore-migrate` binary (default: `/usr/local/bin/stratavore-migrate`)
- `STRATAVORE_DB_URL` environment variable set
- V2 state directory accessible (default: `~/meridian-home/lex-internal/state`)

## Output

Each script reports:
- Sync type and configuration
- Row counts processed
- Success/failure status

Example output:
```
=== Stratavore V2→V3 Full Sync ===
V2 Directory: /home/meridian/meridian-home/lex-internal/state
Binary: /usr/local/bin/stratavore-migrate

Syncing all data types (projects, sessions, config, rank)...

Lex V2 → Stratavore V3 Synchronization
═══════════════════════════════════════════════════════════
Sync type: all

[Projects] Syncing PROJECT-MAP.md...
  ✓ Synced 4 projects
[Sessions] Syncing time_sessions.jsonl...
  ✓ Synced 3 sessions
[Config] Syncing LEX-CONFIG.yaml...
  ✓ Synced 1 budgets, 0 quotas
[Rank] Syncing rank-status.jsonl...
  ✓ Synced 234 rank events

✓ Synchronization complete

═══════════════════════════════════════════════════════════
Full sync complete in 2s
═══════════════════════════════════════════════════════════
```

## Safety

All syncs are:
- **Transactional**: Commit only on success
- **Idempotent**: Safe to run multiple times
- **Read-only on V2**: Never modifies V2 source files
- **Non-destructive**: UPSERT preserves existing V3 data

## Troubleshooting

**Sync fails with "database URL required":**
```bash
export STRATAVORE_DB_URL="postgres://user:pass@localhost:5432/stratavore_db"
```

**Binary not found:**
```bash
# Install stratavore-migrate to /usr/local/bin
sudo cp bin/stratavore-migrate /usr/local/bin/
sudo chmod +x /usr/local/bin/stratavore-migrate

# Or specify path explicitly
./full-sync.sh /path/to/state /path/to/bin/stratavore-migrate
```

**Permission denied:**
```bash
chmod +x scripts/sync/*.sh
```

## Integration with Phase 3

These scripts are designed for **Phase 3: Parallel Operation & Continuous Sync** of the migration plan. They enable:
1. V2 continues as source of truth for state files
2. V3 periodically imports V2 updates via these scripts
3. User can test V3 with live V2 data
4. Gradual cutover preparation with minimal risk
