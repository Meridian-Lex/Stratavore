# Phase 4 Resumption Guide
**Target Date**: 2026-02-27 (1 week from 2026-02-20)

## Current State (Phase 3 Complete)

**Branch**: `feature/phase3-parallel-operation`
**PR**: #21 (Phase 3 → Phase 2)
**Database**: Fully migrated, 4 projects, 33 rank events, 21 directives
**Infrastructure**: All 8 Docker containers operational
**Sync**: Automated cron job running every 5 minutes

## Phase 4 Objective

Make Stratavore V3 the default launcher, with V2 preserved as fallback.

## Pre-Phase 4 Checklist

Before starting Phase 4, verify:

- [ ] Phase 3 PR (#21) merged to `feature/phase2-commands`
- [ ] Phase 2 PR merged to `feature/v2-migration` (or appropriate base)
- [ ] Phase 1 PR merged to `main`
- [ ] All PRs in waterfall chain merged successfully
- [ ] 7 days of parallel operation completed (optional stability monitoring)
- [ ] No critical bugs reported during parallel operation
- [ ] Sync logs clean: `tail -100 ~/meridian-home/logs/stratavore-sync.log`
- [ ] Database healthy: `docker ps | grep stratavore`

## Phase 4 Tasks (6 Granular Steps)

### Task 4.1: Verify V3 Readiness
```bash
# Check daemon health
stratavore status

# Verify all projects visible
stratavore projects

# Check for stale runners
stratavore runners

# Database verification
psql -h localhost -U stratavore -d stratavore_state -c "SELECT COUNT(*) FROM projects;"
```

### Task 4.2: Update Launcher Symlink
```bash
# Backup V2 launcher
cp ~/.local/bin/lex ~/.local/bin/lex-v2-backup

# Point to V3
rm ~/.local/bin/lex
ln -s /usr/local/bin/stratavore ~/.local/bin/lex

# Verify
lex --version  # Should show Stratavore version
which lex      # Should point to stratavore
```

### Task 4.3: Immediate Testing
```bash
# Test interactive menu
lex

# Test smart launch
lex myproject

# Test all critical commands
lex mode get
lex state
lex projects
lex tokens
```

### Task 4.4: Monitor for 48 Hours
- Watch daemon logs: `journalctl -u stratavored -f` (if systemd service)
- Check Docker logs: `docker logs -f stratavore-daemon-1`
- Monitor sync logs: `tail -f ~/meridian-home/logs/stratavore-sync.log`
- User workflow testing: Try all daily operations

### Task 4.5: Document Issues (if any)
Create `PHASE-4-ISSUES.md` to track any problems discovered during cutover.

### Task 4.6: Rollback Procedure (if needed)
**Trigger**: Critical bug or workflow broken

```bash
# Revert symlink (< 5 minutes)
rm ~/.local/bin/lex
ln -s ~/.local/bin/lex-v2-backup ~/.local/bin/lex

# Verify rollback
lex --version  # Should show "2.0.0"

# Report issue
echo "V3 cutover paused due to [ISSUE]. V2 restored." | tee ~/phase4-rollback.log
```

## Resumption Command

To resume Phase 4 in a week:

```bash
cd /home/meridian/meridian-home/projects/Stratavore
cat PHASE-4-RESUMPTION.md
```

Or tell Meridian Lex:
```
"Resume Phase 4 migration - primary cutover. Review PHASE-4-RESUMPTION.md and proceed with waterfall PR creation after verifying all Phase 1-3 PRs are merged."
```

## Phase 4 Success Criteria

- [ ] V3 as default for 48 hours with no critical bugs
- [ ] All user workflows functional
- [ ] Rollback tested (symlink revert works)
- [ ] User feedback positive or neutral

## Next Phase Preview

**Phase 5**: V2 Deprecation (after Phase 4 stable)
- Archive V2 launcher
- Mark `Meridian-Lex/lex` repository as archived
- Final state export
- Documentation updates

---

**Resumption Date**: 2026-02-27
**Officer**: Lieutenant (JG) Meridian Lex
**Status**: Phase 3 complete, standing by for Phase 4 authorization
