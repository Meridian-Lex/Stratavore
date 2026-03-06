# History Miner — Implementation Plan

> **For Lex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build `history_miner.py` — a tool library with CLI commands that Haiku subagents invoke during a three-phase mining operation. Produces synthetic calibration sessions for `estimator.py`.

**Architecture:** `history_miner.py` is a **pure Python tool** — it provides the parsing, git mining, and synthesis functions via a CLI. Parallelism is NOT Python code: Lex dispatches 10–20 actual **Haiku subagents** (Meridian Lex Task tool) per phase. Each Haiku scout is given one unit of work (one archive file, one repo, one task batch), runs `history_miner.py <command>`, and returns structured JSON. Lex aggregates results between phases. The Python module has no orchestration logic.

```
jobs/history_miner.py      — pure tool library + CLI (invoked by scouts)
jobs/test_history_miner.py — unit tests for all pure functions

Scout orchestration: Lex session dispatches Task tool (subagent_type=general-purpose, model=haiku)
  Phase 1 scouts: python3 history_miner.py archive-parse <file>
  Phase 2 scouts: python3 history_miner.py repo-scan <repo_path> <task_ids_json>
  Phase 3 scouts: python3 history_miner.py synthesize <tasks_json> <sessions_json>
  Writer:         python3 history_miner.py write <synthetic_json>
```

**Tech Stack:** Python 3.11, stdlib only (json, re, subprocess, datetime, pathlib). No new dependencies.

---

## Pre-work: verify branch

```bash
cd ~/meridian-home/projects/Stratavore
git branch  # confirm feat/time-tracking-estimation
```

---

## Task 1: Archive parser — parse_archive_file()

**Files:**
- Create: `jobs/history_miner.py`
- Create: `jobs/test_history_miner.py`

**What it does:** Parse a TASK-QUEUE.md or archive file and extract structured task records.

Each task block looks like:
```
### Task 15: Identity & Branding Cleanup
**Priority**: HIGH
**Status**: COMPLETE
**Assigned**: 2026-02-07
**Completed**: 2026-02-07
**Complexity**: High - Multiple server configurations
**Estimated Effort**: 2-3 hours
(body text may mention "Stratavore", "lex-internal", etc.)
```

**Step 1: Write the failing tests**

Create `jobs/test_history_miner.py`:

```python
import pytest
from history_miner import parse_archive_file, parse_effort_string, infer_size

SAMPLE_ARCHIVE = """\
### Task 1: MCP Server Infrastructure Setup
**Priority**: HIGH
**Status**: COMPLETE
**Assigned**: 2026-02-07
**Completed**: 2026-02-07
**Complexity**: High - Multiple server configurations
**Estimated Effort**: 2-3 hours

Some body text mentioning Stratavore and lex-internal repos.

---

### Task 5: Install Britfix
**Priority**: MEDIUM
**Status**: COMPLETE
**Assigned**: 2026-02-07
**Completed**: 2026-02-07
**Complexity**: Low
**Estimated Effort**: 30 minutes

Installed via uv. No repos involved.

---

### Task 42: No Effort Field
**Priority**: LOW
**Status**: COMPLETE
**Assigned**: 2026-03-01
**Completed**: 2026-03-02
**Complexity**: Medium
"""

def test_parse_returns_list_of_tasks(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert len(tasks) == 3

def test_parse_extracts_task_id_and_name(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert tasks[0]["task_id"] == 1
    assert tasks[0]["name"] == "MCP Server Infrastructure Setup"

def test_parse_extracts_dates(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert tasks[0]["assigned"] == "2026-02-07"
    assert tasks[0]["completed"] == "2026-02-07"

def test_parse_extracts_complexity(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert tasks[0]["complexity"] == "High"
    assert tasks[1]["complexity"] == "Low"

def test_parse_extracts_estimated_minutes(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert tasks[0]["estimated_minutes"] == 150  # midpoint of 2-3 hours
    assert tasks[1]["estimated_minutes"] == 30

def test_parse_none_effort_when_missing(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert tasks[2]["estimated_minutes"] is None

def test_parse_detects_repo_mentions(tmp_path):
    f = tmp_path / "archive.md"
    f.write_text(SAMPLE_ARCHIVE)
    tasks = parse_archive_file(str(f))
    assert "Stratavore" in tasks[0]["repos"]
    assert "lex-internal" in tasks[0]["repos"]

def test_parse_skips_incomplete_tasks(tmp_path):
    content = """\
### Task 99: Not Done Yet
**Priority**: HIGH
**Status**: IN PROGRESS
**Assigned**: 2026-03-06
**Complexity**: Medium
"""
    f = tmp_path / "active.md"
    f.write_text(content)
    tasks = parse_archive_file(str(f))
    assert len(tasks) == 0  # IN PROGRESS tasks excluded
```

**Step 2: Run — confirm all fail**
```bash
cd ~/meridian-home/projects/Stratavore/jobs
python3 -m pytest test_history_miner.py -v 2>&1 | head -20
```
Expected: `ModuleNotFoundError: No module named 'history_miner'`

**Step 3: Implement `history_miner.py` — parse_archive_file() and helpers**

Create `jobs/history_miner.py`:

```python
#!/usr/bin/env python3
"""
Meridian Lex — History Miner
Mine task archives and git commit histories for estimator calibration data.

Phases (each invocable as a CLI command for scout deployment):
  archive-parse  <file>              Parse one archive/queue file → JSON tasks
  repo-scan      <repo> <tasks_json> Mine git commits for task sessions → JSON sessions
  synthesize     <tasks> <sessions>  Build synthetic time_sessions.jsonl entries → JSON
  mine           [--dry-run]         Full pipeline (orchestrates all phases locally)
"""

import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional

DEFAULT_SESSIONS_FILE = os.path.expanduser(
    "~/meridian-home/lex-internal/state/time_sessions.jsonl"
)

KNOWN_REPOS = [
    "Stratavore", "lex-internal", "meridian-home", "lex", "Lex-webui",
    "Gantry", "Synapse", "synapse", "britfix", "Starfix",
    "gilded-sentinel-renovate", "lex-usage-monitor", "lex-voice",
]

REPO_MAP = {
    "Stratavore":    "~/meridian-home/projects/Stratavore",
    "lex-internal":  "~/meridian-home/lex-internal",
    "meridian-home": "~/meridian-home",
    "lex":           "~/meridian-home/projects/lex",
    "Lex-webui":     "~/meridian-home/projects/Lex-webui",
    "Gantry":        "~/meridian-home/projects/Gantry",
    "Synapse":       "~/meridian-home/projects/Synapse",
    "synapse":       "~/meridian-home/projects/Synapse",
    "britfix":       "~/meridian-home/projects/britfix",
    "gilded-sentinel-renovate": "~/meridian-home/projects/gilded-sentinel-renovate",
}

VALID_STATUSES = {"COMPLETE", "COMPLETED", "CANCELLED"}


# ── Parsing ───────────────────────────────────────────────────────────────────

def parse_effort_string(effort: str) -> Optional[int]:
    """Convert effort string to minutes. Returns None if unparseable or too coarse."""
    if not effort:
        return None
    effort = effort.strip().strip('"').lower()

    # "N-M days" — too coarse, skip
    if "day" in effort:
        return None

    # "N-M hours" or "N-M hour"
    m = re.match(r"(\d+(?:\.\d+)?)\s*[-–]\s*(\d+(?:\.\d+)?)\s*hours?", effort)
    if m:
        return int(((float(m.group(1)) + float(m.group(2))) / 2) * 60)

    # "N hours" or "N hour"
    m = re.match(r"(\d+(?:\.\d+)?)\s*hours?", effort)
    if m:
        return int(float(m.group(1)) * 60)

    # "N-M minutes" or "N-M min"
    m = re.match(r"(\d+)\s*[-–]\s*(\d+)\s*min", effort)
    if m:
        return (int(m.group(1)) + int(m.group(2))) // 2

    # "N minutes" or "N min"
    m = re.match(r"(\d+)\s*min", effort)
    if m:
        return int(m.group(1))

    return None


def infer_size(duration_minutes: int) -> str:
    """Infer task size from actual duration."""
    if duration_minutes < 15:
        return "XS"
    if duration_minutes < 30:
        return "S"
    if duration_minutes < 60:
        return "M"
    if duration_minutes < 120:
        return "L"
    return "XL"


def _extract_complexity(text: str) -> Optional[str]:
    """Extract Low/Medium/High from a Complexity field value."""
    for level in ("High", "Medium", "Low"):
        if level.lower() in text.lower():
            return level
    return None


def _find_repo_mentions(text: str) -> List[str]:
    """Scan text for known repo name mentions."""
    found = []
    for repo in KNOWN_REPOS:
        if repo.lower() in text.lower():
            found.append(repo)
    return found


def parse_archive_file(filepath: str) -> List[Dict]:
    """
    Parse a TASK-QUEUE.md or TASK-ARCHIVE file.
    Returns list of completed task dicts.
    Only includes tasks with status COMPLETE or CANCELLED.
    """
    with open(filepath, "r") as f:
        content = f.read()

    tasks = []
    # Split on task headers: ### Task N: Name
    blocks = re.split(r"(?=^### Task \d+:)", content, flags=re.MULTILINE)

    for block in blocks:
        header_m = re.match(r"^### Task (\d+):\s*(.+)", block)
        if not header_m:
            continue

        task_id = int(header_m.group(1))
        name = header_m.group(2).strip()

        # Status — only include completed/cancelled
        status_m = re.search(r"\*\*Status\*\*:\s*(.+)", block)
        if not status_m:
            continue
        status = status_m.group(1).strip().upper().split()[0]
        if status not in VALID_STATUSES:
            continue

        assigned_m = re.search(r"\*\*Assigned\*\*:\s*(\d{4}-\d{2}-\d{2})", block)
        completed_m = re.search(r"\*\*Completed\*\*:\s*(\d{4}-\d{2}-\d{2})", block)
        complexity_m = re.search(r"\*\*Complexity\*\*:\s*(.+)", block)
        effort_m = re.search(r"\*\*Estimated Effort\*\*:\s*(.+)", block)

        tasks.append({
            "task_id": task_id,
            "name": name,
            "assigned": assigned_m.group(1) if assigned_m else None,
            "completed": completed_m.group(1) if completed_m else None,
            "complexity": _extract_complexity(complexity_m.group(1)) if complexity_m else None,
            "estimated_minutes": parse_effort_string(effort_m.group(1)) if effort_m else None,
            "repos": _find_repo_mentions(block),
        })

    return tasks
```

**Step 4: Run tests — confirm all pass**
```bash
cd ~/meridian-home/projects/Stratavore/jobs
python3 -m pytest test_history_miner.py -v
```
Expected: 8 passed.

**Step 5: Commit**
```bash
cd ~/meridian-home/projects/Stratavore
git add jobs/history_miner.py jobs/test_history_miner.py
git commit -m "feat(history-miner): archive parser — parse_archive_file, parse_effort_string, infer_size"
```

---

## Task 2: Git commit miner — mine_repo_sessions()

**Files:**
- Modify: `jobs/history_miner.py`
- Modify: `jobs/test_history_miner.py`

**What it does:** Given a repo path and list of task IDs, run `git log` and cluster matching commits into work sessions.

**Step 1: Add failing tests** (append to `test_history_miner.py`):

```python
import subprocess
from history_miner import cluster_commits, match_commit_to_task, mine_repo_sessions

def test_match_commit_branch_pattern():
    assert match_commit_to_task("feat/task-5-britfix", "", [5]) == 5
    assert match_commit_to_task("fix/task-15-identity", "", [15]) == 15
    assert match_commit_to_task("main", "unrelated commit", [5]) is None

def test_match_commit_message_pattern():
    assert match_commit_to_task("main", "[Task 3] fix parser", [3]) == 3
    assert match_commit_to_task("main", "task-3: fix parser", [3]) == 3
    assert match_commit_to_task("main", "closes #3", [3]) == 3

def test_match_commit_no_match():
    assert match_commit_to_task("feature/unrelated", "some commit", [5, 15]) is None

def test_cluster_commits_single_session():
    commits = [
        {"ts": 1000, "hash": "aaa"},
        {"ts": 1100, "hash": "bbb"},
        {"ts": 1200, "hash": "ccc"},
    ]
    sessions = cluster_commits(commits)
    assert len(sessions) == 1
    assert sessions[0]["start_ts"] == 1000
    assert sessions[0]["end_ts"] == 1200
    assert sessions[0]["commit_count"] == 3

def test_cluster_commits_splits_on_gap():
    commits = [
        {"ts": 1000, "hash": "aaa"},
        {"ts": 1100, "hash": "bbb"},
        {"ts": 5000, "hash": "ccc"},  # gap > 1800s
        {"ts": 5100, "hash": "ddd"},
    ]
    sessions = cluster_commits(commits)
    assert len(sessions) == 2
    assert sessions[0]["end_ts"] == 1100
    assert sessions[1]["start_ts"] == 5000

def test_cluster_commits_minimum_floor():
    """Single commit gets 5-minute floor."""
    commits = [{"ts": 1000, "hash": "aaa"}]
    sessions = cluster_commits(commits)
    assert sessions[0]["duration_seconds"] == 300

def test_cluster_commits_empty():
    assert cluster_commits([]) == []

def test_mine_repo_sessions_returns_list(tmp_path):
    """mine_repo_sessions on non-git dir returns empty list gracefully."""
    result = mine_repo_sessions(str(tmp_path), [1, 2, 3])
    assert result == []
```

**Step 2: Run — confirm new tests fail**
```bash
python3 -m pytest test_history_miner.py -v 2>&1 | grep -E "PASS|FAIL|ERROR"
```
Expected: 8 pass (Task 1 tests), 7 fail (new tests).

**Step 3: Implement** — append to `history_miner.py` after `parse_archive_file`:

```python
# ── Git Mining ────────────────────────────────────────────────────────────────

TASK_PATTERNS = [
    re.compile(r"task[-/](\d+)", re.IGNORECASE),
    re.compile(r"\[task\s+(\d+)\]", re.IGNORECASE),
    re.compile(r"closes\s+#(\d+)", re.IGNORECASE),
    re.compile(r"task\s+(\d+):", re.IGNORECASE),
]


def match_commit_to_task(refname: str, subject: str, task_ids: List[int]) -> Optional[int]:
    """Return task_id if commit refname or subject matches any task in task_ids."""
    combined = f"{refname} {subject}"
    for pattern in TASK_PATTERNS:
        m = pattern.search(combined)
        if m:
            tid = int(m.group(1))
            if tid in task_ids:
                return tid
    return None


def cluster_commits(commits: List[Dict], gap_seconds: int = 1800) -> List[Dict]:
    """
    Group commits into sessions where intra-commit gap <= gap_seconds.
    Each session: {start_ts, end_ts, duration_seconds, commit_count}
    Minimum duration floor: 300s (5 min).
    """
    if not commits:
        return []

    commits = sorted(commits, key=lambda c: c["ts"])
    sessions = []
    cluster = [commits[0]]

    for commit in commits[1:]:
        if commit["ts"] - cluster[-1]["ts"] <= gap_seconds:
            cluster.append(commit)
        else:
            sessions.append(_cluster_to_session(cluster))
            cluster = [commit]
    sessions.append(_cluster_to_session(cluster))
    return sessions


def _cluster_to_session(cluster: List[Dict]) -> Dict:
    start = cluster[0]["ts"]
    end = cluster[-1]["ts"]
    duration = max(end - start, 300)  # 5-minute floor
    return {
        "start_ts": start,
        "end_ts": end,
        "duration_seconds": duration,
        "commit_count": len(cluster),
    }


def mine_repo_sessions(repo_path: str, task_ids: List[int]) -> List[Dict]:
    """
    Run git log on repo_path, cluster matching commits into sessions per task.
    Returns list of {task_id, repo, start_ts, end_ts, duration_seconds,
                     commit_count, match_method}.
    Returns [] if repo_path is not a git repo or git fails.
    """
    repo_path = os.path.expanduser(repo_path)
    if not os.path.isdir(repo_path):
        return []

    try:
        result = subprocess.run(
            ["git", "log", "--format=%H %at %s %D", "--all"],
            cwd=repo_path,
            capture_output=True, text=True, timeout=30
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return []

    if result.returncode != 0:
        return []

    # Parse log lines
    task_commits: Dict[int, List[Dict]] = {tid: [] for tid in task_ids}
    for line in result.stdout.splitlines():
        parts = line.split(" ", 3)
        if len(parts) < 2:
            continue
        commit_hash, ts_str = parts[0], parts[1]
        subject = parts[2] if len(parts) > 2 else ""
        refname = parts[3] if len(parts) > 3 else ""
        try:
            ts = int(ts_str)
        except ValueError:
            continue

        tid = match_commit_to_task(refname, subject, task_ids)
        if tid is not None:
            task_commits[tid].append({"ts": ts, "hash": commit_hash})

    repo_name = os.path.basename(repo_path.rstrip("/"))
    sessions = []
    for tid, commits in task_commits.items():
        if not commits:
            continue
        for s in cluster_commits(commits):
            sessions.append({
                "task_id": tid,
                "repo": repo_name,
                "start_ts": s["start_ts"],
                "end_ts": s["end_ts"],
                "duration_seconds": s["duration_seconds"],
                "commit_count": s["commit_count"],
                "match_method": "commit-pattern",
            })
    return sessions
```

**Step 4: Run tests — confirm all 15 pass**
```bash
python3 -m pytest test_history_miner.py -v
```
Expected: 15 passed.

**Step 5: Commit**
```bash
cd ~/meridian-home/projects/Stratavore
git add jobs/history_miner.py jobs/test_history_miner.py
git commit -m "feat(history-miner): git commit miner — match_commit_to_task, cluster_commits, mine_repo_sessions"
```

---

## Task 3: Session synthesizer — build_synthetic_sessions() + deduplication

**Files:**
- Modify: `jobs/history_miner.py`
- Modify: `jobs/test_history_miner.py`

**What it does:** Combines task metadata with mined git sessions to produce `time_sessions.jsonl`-compatible entries. Also handles deduplication against existing sessions.

**Step 1: Add failing tests** (append to `test_history_miner.py`):

```python
from history_miner import build_synthetic_sessions, load_existing_ids

SAMPLE_TASK = {
    "task_id": 5, "name": "Install Britfix",
    "assigned": "2026-02-07", "completed": "2026-02-07",
    "complexity": "Low", "estimated_minutes": 30,
    "repos": ["britfix"],
}
SAMPLE_GIT_SESSION = {
    "task_id": 5, "repo": "britfix",
    "start_ts": 1739500000, "end_ts": 1739500300,
    "duration_seconds": 300, "commit_count": 2,
    "match_method": "commit-pattern",
}

def test_build_synthetic_basic(tmp_path):
    sf = str(tmp_path / "sessions.jsonl")
    open(sf, "w").close()
    results = build_synthetic_sessions([SAMPLE_TASK], [SAMPLE_GIT_SESSION], sf)
    assert len(results) == 1
    s = results[0]
    assert s["job_id"] == "task-5"
    assert s["status"] == "completed"
    assert s["synthetic"] is True
    assert s["source"] == "git-mining"
    assert s["size"] == "XS"   # 5 min < 15 min threshold
    assert s["complexity"] == "Low"
    assert s["estimated_minutes"] == 30
    assert s["duration_seconds"] == 300

def test_build_synthetic_skips_duplicate(tmp_path):
    sf = str(tmp_path / "sessions.jsonl")
    existing = {
        "session_id": "task-5_existing", "job_id": "task-5",
        "status": "completed", "synthetic": False,
    }
    with open(sf, "w") as f:
        f.write(json.dumps(existing) + "\n")
    results = build_synthetic_sessions([SAMPLE_TASK], [SAMPLE_GIT_SESSION], sf)
    assert len(results) == 0

def test_build_synthetic_archive_fallback(tmp_path):
    """Task with no git sessions falls back to archive dates for duration."""
    sf = str(tmp_path / "sessions.jsonl")
    open(sf, "w").close()
    task = {**SAMPLE_TASK, "assigned": "2026-02-07", "completed": "2026-02-07",
            "estimated_minutes": 30}
    results = build_synthetic_sessions([task], [], sf)
    # Archive fallback: estimated_minutes used as duration proxy
    assert len(results) == 1
    assert results[0]["source"] == "task-archive"

def test_build_synthetic_skips_multi_day_no_git(tmp_path):
    """Task spanning multiple days with no git data is skipped (too coarse)."""
    sf = str(tmp_path / "sessions.jsonl")
    open(sf, "w").close()
    task = {**SAMPLE_TASK, "assigned": "2026-02-07", "completed": "2026-02-10",
            "estimated_minutes": None}
    results = build_synthetic_sessions([task], [], sf)
    assert len(results) == 0

def test_load_existing_ids(tmp_path):
    sf = str(tmp_path / "sessions.jsonl")
    with open(sf, "w") as f:
        f.write(json.dumps({"job_id": "task-1"}) + "\n")
        f.write(json.dumps({"job_id": "task-5"}) + "\n")
    ids = load_existing_ids(sf)
    assert "task-1" in ids
    assert "task-5" in ids
```

**Step 2: Run — confirm new tests fail**
```bash
python3 -m pytest test_history_miner.py -v 2>&1 | grep -E "PASS|FAIL|ERROR"
```
Expected: 15 pass, 5 fail.

**Step 3: Implement** — append to `history_miner.py`:

```python
# ── Session Synthesis ─────────────────────────────────────────────────────────

def load_existing_ids(sessions_file: str) -> set:
    """Return set of job_ids already in sessions_file."""
    ids = set()
    if not os.path.exists(sessions_file):
        return ids
    with open(sessions_file, "r") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                s = json.loads(line)
                if s.get("job_id"):
                    ids.add(s["job_id"])
            except json.JSONDecodeError:
                pass
    return ids


def build_synthetic_sessions(
    tasks: List[Dict],
    git_sessions: List[Dict],
    existing_sessions_file: str,
) -> List[Dict]:
    """
    Combine task metadata + git sessions into synthetic time_sessions entries.
    Skips tasks already present in existing_sessions_file.
    Falls back to archive-derived estimate when no git sessions found.
    """
    existing_ids = load_existing_ids(existing_sessions_file)

    # Index git sessions by task_id
    git_by_task: Dict[int, List[Dict]] = {}
    for gs in git_sessions:
        git_by_task.setdefault(gs["task_id"], []).append(gs)

    synthetic = []
    for task in tasks:
        tid = task["task_id"]
        job_id = f"task-{tid}"

        if job_id in existing_ids:
            continue

        git_for_task = git_by_task.get(tid, [])

        if git_for_task:
            # Use git session data
            for i, gs in enumerate(git_for_task):
                dur = gs["duration_seconds"]
                size = infer_size(dur // 60)
                start_dt = datetime.fromtimestamp(gs["start_ts"], tz=timezone.utc)
                end_dt = datetime.fromtimestamp(gs["end_ts"], tz=timezone.utc)
                synthetic.append(_make_session(
                    session_id=f"{job_id}_git_{i}",
                    job_id=job_id,
                    task=task,
                    size=size,
                    duration_seconds=dur,
                    start_dt=start_dt,
                    end_dt=end_dt,
                    source="git-mining",
                ))
        else:
            # Archive fallback: only use if same-day (duration derivable from estimate)
            assigned = task.get("assigned")
            completed = task.get("completed")
            est = task.get("estimated_minutes")

            if assigned != completed:
                continue  # multi-day, too coarse

            if not est:
                continue  # no estimate and no git data — skip

            # Use estimated_minutes as proxy duration
            dur = est * 60
            size = infer_size(est)
            try:
                base_ts = datetime.strptime(assigned, "%Y-%m-%d").replace(tzinfo=timezone.utc).timestamp()
            except (TypeError, ValueError):
                continue
            start_dt = datetime.fromtimestamp(base_ts, tz=timezone.utc)
            end_dt = datetime.fromtimestamp(base_ts + dur, tz=timezone.utc)
            synthetic.append(_make_session(
                session_id=f"{job_id}_archive_0",
                job_id=job_id,
                task=task,
                size=size,
                duration_seconds=dur,
                start_dt=start_dt,
                end_dt=end_dt,
                source="task-archive",
            ))

    return synthetic


def _make_session(session_id, job_id, task, size, duration_seconds,
                  start_dt, end_dt, source) -> Dict:
    return {
        "session_id": session_id,
        "job_id": job_id,
        "agent": "lex",
        "description": task["name"],
        "status": "completed",
        "size": size,
        "complexity": task.get("complexity") or "Medium",
        "estimated_minutes": task.get("estimated_minutes"),
        "start_time": start_dt.isoformat(),
        "start_timestamp": start_dt.timestamp(),
        "end_time": end_dt.isoformat(),
        "end_timestamp": end_dt.timestamp(),
        "duration_seconds": duration_seconds,
        "paused_time": 0,
        "pauses": [],
        "notes": f"backfilled by history_miner — source: {source}",
        "created_at": datetime.now(tz=timezone.utc).isoformat(),
        "synthetic": True,
        "source": source,
    }
```

**Step 4: Run tests — confirm all 20 pass**
```bash
python3 -m pytest test_history_miner.py -v
```
Expected: 20 passed.

**Step 5: Commit**
```bash
cd ~/meridian-home/projects/Stratavore
git add jobs/history_miner.py jobs/test_history_miner.py
git commit -m "feat(history-miner): session synthesizer — build_synthetic_sessions, deduplication, archive fallback"
```

---

## Task 4: Writer CLI + scout dispatch procedure

**Files:**
- Modify: `jobs/history_miner.py`

**What it does:** Adds `write` CLI command (final writer scout target) and `resolve_repo_path()`. Then documents the Haiku scout dispatch procedure — the Python module is complete; orchestration is done by Lex via Task tool subagents.

**Step 1: Add failing tests** (append to `test_history_miner.py`):

```python
from history_miner import write_synthetic_sessions, resolve_repo_path

def test_write_synthetic_appends(tmp_path):
    sf = str(tmp_path / "sessions.jsonl")
    open(sf, "w").close()
    sessions = [{"session_id": "task-1_git_0", "job_id": "task-1", "status": "completed"}]
    stats = write_synthetic_sessions(sessions, sf)
    assert stats["added"] == 1
    lines = open(sf).readlines()
    assert len(lines) == 1

def test_write_synthetic_skips_existing(tmp_path):
    sf = str(tmp_path / "sessions.jsonl")
    existing = {"session_id": "task-1_git_0", "job_id": "task-1", "status": "completed"}
    with open(sf, "w") as f:
        f.write(json.dumps(existing) + "\n")
    sessions = [{"session_id": "task-1_git_0", "job_id": "task-1", "status": "completed"}]
    stats = write_synthetic_sessions(sessions, sf)
    assert stats["added"] == 0
    assert stats["skipped_duplicate"] == 1

def test_resolve_repo_path_known():
    path = resolve_repo_path("Stratavore")
    assert path is not None
    assert "Stratavore" in path

def test_resolve_repo_path_unknown():
    assert resolve_repo_path("nonexistent-repo-xyz") is None
```

**Step 2: Run — confirm new tests fail**
```bash
python3 -m pytest test_history_miner.py -v 2>&1 | grep -E "PASS|FAIL|ERROR"
```
Expected: 20 pass, 4 fail.

**Step 3: Implement** — append to `history_miner.py`:

```python
# ── Writer ────────────────────────────────────────────────────────────────────

def resolve_repo_path(repo_name: str) -> Optional[str]:
    """Resolve repo name to local path. Returns None if not found."""
    path = REPO_MAP.get(repo_name)
    if path:
        expanded = os.path.expanduser(path)
        if os.path.isdir(expanded):
            return expanded
    projects_dir = os.path.expanduser("~/meridian-home/projects")
    candidate = os.path.join(projects_dir, repo_name)
    if os.path.isdir(candidate):
        return candidate
    return None


def write_synthetic_sessions(
    sessions: List[Dict],
    sessions_file: str = DEFAULT_SESSIONS_FILE,
) -> Dict:
    """Append synthetic sessions to sessions_file. Skip existing session_ids."""
    existing_sids = set()
    if os.path.exists(sessions_file):
        with open(sessions_file, "r") as f:
            for line in f:
                line = line.strip()
                if line:
                    try:
                        existing_sids.add(json.loads(line)["session_id"])
                    except (json.JSONDecodeError, KeyError):
                        pass

    added = 0
    skipped = 0
    with open(sessions_file, "a") as f:
        for s in sessions:
            if s.get("session_id") in existing_sids:
                skipped += 1
                continue
            f.write(json.dumps(s) + "\n")
            added += 1

    return {"added": added, "skipped_duplicate": skipped}
```

**Then add the CLI block** — append at bottom of `history_miner.py`:

```python
# ── CLI ───────────────────────────────────────────────────────────────────────
# Each command is the target for one class of Haiku scout.
# Orchestration (dispatching scouts, collecting results) is done by the Lex
# session via the Task tool — NOT by this script.

USAGE = """\
Meridian Lex — History Miner (scout tool)

Usage: history_miner.py <command> [args]

Commands (invoked by Haiku scouts):
  archive-parse <file>                    Parse one archive file → JSON tasks
  repo-scan <repo_path> <task_ids_json>   Mine one repo → JSON sessions
  synthesize <tasks_json> <sessions_json> Build synthetic sessions → JSON
  write <synthetic_json>                  Append sessions to time_sessions.jsonl

Sessions file: {sessions_file}
"""


def main():
    if len(sys.argv) < 2:
        print(USAGE.format(sessions_file=DEFAULT_SESSIONS_FILE))
        return

    cmd = sys.argv[1]

    if cmd == "archive-parse":
        if len(sys.argv) < 3:
            print("Usage: history_miner.py archive-parse <file>")
            return
        tasks = parse_archive_file(sys.argv[2])
        print(json.dumps({"tasks": tasks}, indent=2))

    elif cmd == "repo-scan":
        if len(sys.argv) < 4:
            print("Usage: history_miner.py repo-scan <repo_path> <task_ids_json>")
            return
        task_ids = json.loads(sys.argv[3])
        sessions = mine_repo_sessions(sys.argv[2], task_ids)
        print(json.dumps({"sessions": sessions}, indent=2))

    elif cmd == "synthesize":
        if len(sys.argv) < 4:
            print("Usage: history_miner.py synthesize <tasks_json> <sessions_json>")
            return
        tasks = json.loads(sys.argv[2])
        sessions = json.loads(sys.argv[3])
        synthetic = build_synthetic_sessions(tasks, sessions, DEFAULT_SESSIONS_FILE)
        print(json.dumps({"synthetic_sessions": synthetic}, indent=2))

    elif cmd == "write":
        if len(sys.argv) < 3:
            print("Usage: history_miner.py write <synthetic_json>")
            return
        sessions = json.loads(sys.argv[2])
        stats = write_synthetic_sessions(sessions)
        print(json.dumps(stats, indent=2))

    else:
        print(f"[ERR] Unknown command: '{cmd}'")
        print(USAGE.format(sessions_file=DEFAULT_SESSIONS_FILE))


if __name__ == "__main__":
    main()
```

**Scout dispatch procedure (run by Lex via Task tool — NOT Python):**

When performing a mining operation, Lex dispatches the following subagent fleet in three sequential phases:

```
PHASE 1 — Archive scouts (all parallel, subagent_type=general-purpose, model=haiku)
  For each archive file:
    Task: "Run: python3 ~/meridian-home/projects/Stratavore/jobs/history_miner.py
           archive-parse <filepath>
           Return the full JSON output."
  Collect all {"tasks": [...]} results → merge into combined task list.

PHASE 2 — Repo scouts (all parallel, subagent_type=general-purpose, model=haiku)
  Collect unique repo names from all tasks.
  For each repo:
    Task: "Run: python3 history_miner.py repo-scan <repo_path> '<task_ids_json>'
           Return the full JSON output."
  Collect all {"sessions": [...]} results → merge into combined session list.

PHASE 3 — Batch synthesis scouts (all parallel, subagent_type=general-purpose, model=haiku)
  Chunk tasks into batches of 5.
  For each batch:
    Task: "Run: python3 history_miner.py synthesize '<tasks_json>' '<sessions_json>'
           Return the full JSON output."
  Collect all {"synthetic_sessions": [...]} results → flatten into one list.

WRITER — single scout (subagent_type=general-purpose, model=haiku)
  Task: "Run: python3 history_miner.py write '<all_synthetic_json>'
         Return the stats JSON."
  Report added/skipped counts to Admiral.
```

**Step 4: Run all tests — confirm 24 pass**
```bash
python3 -m pytest test_history_miner.py -v
```
Expected: 24 passed.

**Step 5: Commit**
```bash
cd ~/meridian-home/projects/Stratavore
git add jobs/history_miner.py jobs/test_history_miner.py
git commit -m "feat(history-miner): full pipeline — mine(), write_synthetic_sessions, CLI"
```

---

## Task 5: Smoke test + verification

**Step 1: Test archive-parse CLI directly**
```bash
cd ~/meridian-home/projects/Stratavore/jobs
python3 history_miner.py archive-parse \
  ~/meridian-home/lex-internal/state/TASK-ARCHIVE-FEB-2026.md \
  | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'Tasks extracted: {len(d[\"tasks\"])}')"
```
Expected: `Tasks extracted: N` where N > 10.

**Step 2: Test repo-scan CLI directly**
```bash
TASK_IDS=$(python3 history_miner.py archive-parse \
  ~/meridian-home/lex-internal/state/TASK-ARCHIVE-FEB-2026.md \
  | python3 -c "import json,sys; d=json.load(sys.stdin); print(json.dumps([t['task_id'] for t in d['tasks']]))")

python3 history_miner.py repo-scan \
  ~/meridian-home/projects/Stratavore "$TASK_IDS" \
  | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'Git sessions found: {len(d[\"sessions\"])}')"
```
Expected: `Git sessions found: N` (may be 0 if no task-pattern commits; that is acceptable).

**Step 3: Run full test suite**
```bash
python3 -m pytest test_time_tracker.py test_estimator.py test_history_miner.py -v
```
Expected: all tests pass.

**Step 4: Manual Haiku scout run (Lex dispatches fleet)**

At this point, trigger a manual mining run by asking Lex to:
- Dispatch Phase 1 archive scouts (one per archive file, in parallel)
- Dispatch Phase 2 repo scouts for all referenced repos (in parallel)
- Dispatch Phase 3 synthesis batch scouts (in parallel)
- Dispatch writer scout

Then verify:
```bash
python3 -c "
import json
with open('/home/meridian/meridian-home/lex-internal/state/time_sessions.jsonl') as f:
    lines = [json.loads(l) for l in f if l.strip()]
synth = [l for l in lines if l.get('synthetic')]
real = [l for l in lines if not l.get('synthetic')]
print(f'Real sessions: {len(real)}, Synthetic: {len(synth)}')
"
```
Expected: synthetic count > 0, real sessions unchanged.

**Step 3: Verify estimator calibration improved**
```bash
python3 estimator.py calibration
```
Expected: some size buckets now show `confidence: medium` or `high` vs previous `low`.

**Step 4: Run full test suite**
```bash
python3 -m pytest test_time_tracker.py test_estimator.py test_history_miner.py -v
```
Expected: all tests pass.

**Step 5: Commit the mined sessions to lex-internal**
```bash
cd ~/meridian-home/lex-internal
git add state/time_sessions.jsonl
git commit -m "data(time-sessions): synthetic sessions from history_miner run"
git push
cd ~/meridian-home
git add lex-internal
git commit -m "chore: update lex-internal — history miner seed data"
git push
```

---

## Task 6: PR

**Step 1: Push branch**
```bash
cd ~/meridian-home/projects/Stratavore
git push -u origin feat/time-tracking-estimation
```

**Step 2: Open PR**
```bash
gh pr create \
  --repo Meridian-Lex/Stratavore \
  --head Meridian-Lex:feat/time-tracking-estimation \
  --title "feat(jobs): time tracking — size metadata, estimator, history miner" \
  --assignee LunarLaurus \
  --body "$(cat <<'EOF'
## Summary

- **time_tracker.py**: size/complexity metadata fields on sessions; md-append command writes formatted blocks to TIME-TRACKING.md
- **estimator.py**: task size estimation (XS-XL) with historical variance correction, P50/P80/P95 percentiles, confidence levels
- **history_miner.py**: three-phase pipeline that mines task archives + git commit histories to backfill synthetic calibration sessions; designed for parallel Haiku scout deployment

## Test plan

- [ ] All 3 test files pass: test_time_tracker.py, test_estimator.py, test_history_miner.py
- [ ] history_miner.py mine --dry-run completes without errors
- [ ] estimator.py calibration shows improved confidence after mining run
- [ ] time_tracker.py start with --size and --complexity stores fields correctly
- [ ] time_tracker.py md-append writes correct block to TIME-TRACKING.md
EOF
)"
```

---

## Deferred (Phase 2)

- Scheduled/cron invocation of `history_miner.py mine`
- GitHub API fallback for repos not cloned locally
- Incremental mode (skip commits already processed)
- DB migration 0005 (task_time_logs + sprint_tasks size columns)
