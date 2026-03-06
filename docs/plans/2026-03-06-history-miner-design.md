# History Miner — Design Document

**Date**: 2026-03-06
**Status**: Approved
**Author**: Meridian Lex

---

## Goal

Extend `estimator.py` calibration data by mining historical task durations from two sources:
1. TASK-QUEUE.md and task archive files (metadata: complexity, estimated effort)
2. Git commit histories of repos referenced in those tasks (precise timestamps)

Feed results into `time_sessions.jsonl` as synthetic sessions so the variance correction
algorithm gains real calibration data from pre-time-tracker history.

---

## Architecture

Three sequential phases, each parallelised internally via Haiku scout fleet.

```
Phase 1 — Archive Scouts (parallel, one per file)
  Inputs:  TASK-QUEUE.md, TASK-ARCHIVE-FEB-2026.md, future archives
  Outputs: per-file task JSON → merged tasks.json
  Fleet:   N archive scouts + 1 merge scout (~3 total, scales with archive count)

Phase 2 — Repo Scouts (parallel, one per referenced repo)
  Inputs:  tasks.json + local repo paths
  Outputs: sessions_<repo>.json per repo
  Fleet:   up to 15 scouts

Phase 3 — Coordinator Scouts (parallel batch + single writer)
  Inputs:  tasks.json + all sessions_*.json + existing time_sessions.jsonl
  Outputs: synthetic session entries appended to time_sessions.jsonl
           history-mining-report.md run summary
  Fleet:   N batch scouts (5 tasks/chunk) + 1 final writer scout (~5 total)

Total fleet at full load: ~23 scouts across 3 phases.
```

Deliverable: `history_miner.py` — run once on demand (or scheduled).
Phases are sequential barriers; within each phase all scouts run in parallel.

---

## Data Sources

### Task Archive Format
```
### Task N: <name>
**Assigned**: YYYY-MM-DD
**Completed**: YYYY-MM-DD
**Complexity**: Low|Medium|High
**Estimated Effort**: "2-3 hours" | "30 minutes" | etc.
(body text may mention repo names)
```

### Git Commit Correlation
Commits are matched to tasks via:
- Branch name patterns: `feat/task-N-*`, `fix/task-N-*`, `task-N-*`
- Commit message patterns: `[Task N]`, `task-N`, `closes #N`, `task N:` (case-insensitive)
- Fallback: assigned→completed date range filter on commit timestamps

---

## Key Algorithms

### Commit → Session Clustering
```
git log --format="%H %at %s %D" --all
  → filter commits matching task correlation patterns
  → sort by timestamp
  → cluster: gap > 1800s (30 min) = new session boundary
  → session = {start: first_ts, end: last_ts, duration: end-start}
  → minimum duration floor: 300s (5 min) for single-commit tasks
```

### Estimated Effort String → Minutes
```
"N-M hours"    → ((N+M)/2) * 60   midpoint
"N hour(s)"    → N * 60
"N minutes"    → N
"N-M minutes"  → (N+M)/2
"N-M days"     → skipped (too coarse, no intra-day resolution)
```

### Size Inference (when not explicit in archive)
```
duration < 15 min  → XS
duration < 30 min  → S
duration < 60 min  → M
duration < 120 min → L
else               → XL
```

Complexity defaults to `Medium` when not found in archive.

### Deduplication Guard
- Skip synthetic session if `(job_id, start_timestamp)` tuple already exists in `time_sessions.jsonl`
- Skip if session time window overlaps an existing real session for the same job
- `"synthetic": true` and `"source": "git-mining"|"task-archive"` flags on all entries

---

## Scout Interfaces

Each scout receives a JSON payload and returns structured JSON.

### Archive Scout
```json
Input:  { "file": "/path/to/TASK-ARCHIVE-FEB-2026.md" }
Output: { "tasks": [{ "task_id": 1, "name": "...", "complexity": "High",
           "estimated_minutes": 150, "repos": ["Stratavore"],
           "assigned": "2026-02-07", "completed": "2026-02-07" }] }
```

### Merge Scout
```json
Input:  { "task_lists": [[...], [...]] }
Output: { "tasks": [...] }  (deduplicated by task_id)
```

### Repo Scout
```json
Input:  { "repo_path": "/home/meridian/...", "task_ids": [1, 5, 15],
          "date_range": {"start": "2026-02-07", "end": "2026-03-06"} }
Output: { "sessions": [{ "task_id": 1, "repo": "Stratavore",
           "branch": "feat/task-1-mcp", "start_ts": 1234567890,
           "end_ts": 1234569690, "duration_seconds": 1800,
           "commit_count": 4, "match_method": "branch" }] }
```

### Batch Coordinator Scout
```json
Input:  { "tasks": [...5 tasks...], "sessions": [...matching sessions...] }
Output: { "synthetic_sessions": [{...full time_sessions.jsonl schema...}] }
```

### Final Writer Scout
```json
Input:  { "all_synthetic": [...], "existing_jsonl_path": "...",
          "output_path": "..." }
Output: { "added": 12, "skipped_duplicate": 3, "skipped_no_data": 2,
          "report_path": "history-mining-report.md" }
```

---

## Repo Discovery

Referenced repos are resolved to local paths via lookup:
```python
REPO_MAP = {
    "Stratavore":    "~/meridian-home/projects/Stratavore",
    "lex-internal":  "~/meridian-home/lex-internal",
    "meridian-home": "~/meridian-home",
    # extended from projects/ scan at runtime
}
```

Unresolvable repo references → logged as warnings, skipped.

---

## Output

`time_sessions.jsonl` — synthetic sessions appended, never overwrite real sessions.

`history-mining-report.md`:
```
## History Mining Run — YYYY-MM-DD HH:MM UTC
- Archives scanned: N files, M tasks extracted
- Repos scanned: N repos
- Sessions added: N
- Sessions skipped (duplicate): N
- Sessions skipped (no data): N
- Warnings: [list]
```

---

## File Layout

```
lex-internal/scripts/
  history_miner.py          coordinator + scout orchestration
  test_history_miner.py     unit tests for parsing/clustering algorithms
```

No new dependencies beyond stdlib + existing `time_tracker.py` imports.

---

## Deferred

- Scheduled/cron execution (run once manually for now)
- GitHub API fallback for repos not cloned locally
- Incremental mode (only mine new commits since last run)
