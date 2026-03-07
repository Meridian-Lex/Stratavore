#!/usr/bin/env python3
"""Tests for time_tracker.py — size/complexity metadata and md-append command."""

import os
import json
import tempfile
import pytest
from time_tracker import TimeTracker


def make_tracker(tmp_path):
    t = TimeTracker()
    t.sessions_file = str(tmp_path / "sessions.jsonl")
    t._ensure_file_exists()
    return t


def test_start_with_metadata(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job1", "lex", "fix parser", size="M", complexity="High")
    sessions = t._load_sessions()
    assert sessions[-1]["size"] == "M"
    assert sessions[-1]["complexity"] == "High"


def test_start_without_metadata_defaults_none(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job2", "lex", "misc work")
    sessions = t._load_sessions()
    assert sessions[-1].get("size") is None
    assert sessions[-1].get("complexity") is None


def test_md_append_writes_block(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job3", "lex", "some task", size="S", complexity="Low")
    t.end_session(sid, notes="done")
    md_file = str(tmp_path / "TIME-TRACKING.md")
    with open(md_file, "w") as f:
        f.write("# Time Tracking\n\n---\n")
    t.md_append("job3", estimate_minutes=20, md_file=md_file)
    content = open(md_file).read()
    assert "job3" in content
    assert "Actual" in content
    assert "Variance" in content


def test_md_append_no_estimate_omits_variance(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job4", "lex", "another task")
    t.end_session(sid)
    md_file = str(tmp_path / "TT.md")
    open(md_file, "w").close()
    t.md_append("job4", md_file=md_file)
    content = open(md_file).read()
    assert "job4" in content
    assert "Variance" not in content


def test_md_append_includes_size_complexity(tmp_path):
    t = make_tracker(tmp_path)
    sid = t.start_session("job5", "lex", "sized task", size="L", complexity="High")
    t.end_session(sid, notes="complete")
    md_file = str(tmp_path / "TT2.md")
    open(md_file, "w").close()
    t.md_append("job5", estimate_minutes=90, md_file=md_file)
    content = open(md_file).read()
    assert "Size" in content
    assert "L" in content
    assert "Complexity" in content
    assert "High" in content


def test_md_append_no_completed_sessions(tmp_path, capsys):
    """md_append prints an error when there are no completed sessions."""
    t = make_tracker(tmp_path)
    md_file = str(tmp_path / "TT3.md")
    open(md_file, "w").close()
    t.md_append("nonexistent-job", md_file=md_file)
    captured = capsys.readouterr()
    assert "[ERR]" in captured.out


def test_md_append_negative_variance(tmp_path):
    """md_append renders negative variance (faster than estimated) correctly."""
    t = make_tracker(tmp_path)
    sid = t.start_session("job6", "lex", "fast task")
    # Force short duration by ending immediately then patching duration
    t.end_session(sid)
    sessions = t._load_sessions()
    for s in sessions:
        if s["session_id"] == sid:
            s["duration_seconds"] = 300  # 5 min actual vs 20 min estimate → -75%
    t._save_sessions(sessions)
    md_file = str(tmp_path / "TT4.md")
    open(md_file, "w").close()
    t.md_append("job6", estimate_minutes=20, md_file=md_file)
    content = open(md_file).read()
    assert "Variance" in content
    assert "-" in content  # negative variance present


def test_md_append_persists_estimate_to_sessions(tmp_path):
    """md_append with --estimate writes estimated_minutes back into the JSONL."""
    t = make_tracker(tmp_path)
    sid = t.start_session("job7", "lex", "tracked task")
    t.end_session(sid)
    md_file = str(tmp_path / "TT5.md")
    open(md_file, "w").close()
    t.md_append("job7", estimate_minutes=45, md_file=md_file)
    sessions = t._load_sessions()
    completed = [s for s in sessions if s["job_id"] == "job7" and s["status"] == "completed"]
    assert completed
    assert completed[0]["estimated_minutes"] == 45


def test_start_session_invalid_size(tmp_path):
    """start_session raises ValueError for unknown size."""
    t = make_tracker(tmp_path)
    with pytest.raises(ValueError, match="Invalid size"):
        t.start_session("job8", "lex", size="XXL")


def test_start_session_invalid_complexity(tmp_path):
    """start_session raises ValueError for unknown complexity."""
    t = make_tracker(tmp_path)
    with pytest.raises(ValueError, match="Invalid complexity"):
        t.start_session("job9", "lex", complexity="Extreme")


def test_md_append_multiple_sessions_aggregates(tmp_path):
    """md_append sums durations and aggregates notes across multiple sessions for the same job."""
    t = make_tracker(tmp_path)

    # Inject two pre-built completed sessions directly to avoid same-second session_id collision
    base_ts = 1000000.0
    sessions = [
        {
            "session_id": "jobA_1000000",
            "job_id": "jobA",
            "agent": "lex",
            "description": "first part",
            "status": "completed",
            "start_time": "2026-01-01T10:00:00Z",
            "start_timestamp": base_ts,
            "end_time": "2026-01-01T10:10:00Z",
            "end_timestamp": base_ts + 600,
            "duration_seconds": 600,
            "paused_time": 0,
            "pauses": [],
            "notes": "first note",
            "size": "S",
            "complexity": "Low",
            "estimated_minutes": None,
            "created_at": "2026-01-01T10:00:00Z",
        },
        {
            "session_id": "jobA_1000001",
            "job_id": "jobA",
            "agent": "lex",
            "description": "second part",
            "status": "completed",
            "start_time": "2026-01-01T10:20:00Z",
            "start_timestamp": base_ts + 1200,
            "end_time": "2026-01-01T10:40:00Z",
            "end_timestamp": base_ts + 2400,
            "duration_seconds": 1200,
            "paused_time": 0,
            "pauses": [],
            "notes": "second note",
            "size": "M",
            "complexity": "High",
            "estimated_minutes": None,
            "created_at": "2026-01-01T10:20:00Z",
        },
    ]
    with open(t.sessions_file, "w") as f:
        for s in sessions:
            f.write(json.dumps(s) + "\n")

    md_file = str(tmp_path / "MULTI.md")
    open(md_file, "w").close()
    t.md_append("jobA", estimate_minutes=40, md_file=md_file)
    content = open(md_file).read()

    # Total actual should be 30 minutes (10 + 20)
    assert "30" in content
    # Both notes should be aggregated into the block
    assert "first note" in content
    assert "second note" in content
    # Size and complexity should reflect the last session (M/High)
    assert "M" in content
    assert "High" in content
